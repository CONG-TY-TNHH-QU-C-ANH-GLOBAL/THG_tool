package auth

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	authpkg "github.com/thg/scraper/internal/auth"
	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store"
	"github.com/thg/scraper/internal/store/app"
)

// signupUser handles POST /api/auth/signup (public).
// Creates a user WITHOUT an org â€” org is created in the onboarding step.
func (h *Handler) signupUser(c *fiber.Ctx) error {
	var req struct {
		Name     string `json:"name"`
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}
	if req.Email == "" || req.Password == "" || req.Name == "" {
		return c.Status(400).JSON(fiber.Map{"error": "name, email and password required"})
	}
	if err := authpkg.ValidatePasswordStrength(req.Password); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	existing, _ := h.deps.DB.GetUserByEmail(req.Email)
	if existing != nil {
		return c.Status(409).JSON(fiber.Map{"error": "email already registered"})
	}

	orgID := int64(0)
	role := models.RoleAdmin
	var provisionedClaim *store.ProvisionedOrgClaim
	if claim, err := h.deps.DB.FindProvisionedOrgByEmail(req.Email); err != nil {
		log.Printf("[Signup] Provisioned org lookup error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "workspace assignment failed"})
	} else if claim != nil {
		orgID = claim.OrgID
		role = claim.Role
		provisionedClaim = claim
	}

	hash, err := authpkg.HashPassword(req.Password)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "internal error"})
	}
	user := &models.User{
		OrgID:        orgID,
		Email:        req.Email,
		Name:         req.Name,
		PasswordHash: hash,
		Role:         role,
	}
	userID, err := h.deps.DB.CreateUser(user)
	if err != nil {
		log.Printf("[Signup] Create user error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "could not create user"})
	}
	if err := h.completeProvisionedClaim(userID, provisionedClaim, c.IP()); err != nil {
		log.Printf("[Signup] Complete provisioned claim error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "workspace assignment failed"})
	}

	accessToken, _ := authpkg.GenerateAccessToken(userID, orgID, req.Email, string(role), h.deps.JWTSecret)
	refreshToken, _ := authpkg.GenerateRefreshToken()
	expiresAt := time.Now().Add(authpkg.RefreshTokenTTL)
	_ = h.deps.DB.SaveRefreshToken(userID, refreshToken, expiresAt)
	setRefreshCookie(c, refreshToken)
	setAuthCookies(c, accessToken, time.Now().Add(authpkg.AccessTokenTTL))
	h.deps.DB.InsertAuditLog(userID, "signup", c.IP(), `{}`)

	return c.Status(201).JSON(fiber.Map{
		"access_token": accessToken,
		"user": fiber.Map{
			"id":     userID,
			"org_id": orgID,
			"email":  req.Email,
			"name":   req.Name,
			"role":   role,
		},
	})
}

// onboardingSetup handles POST /api/onboarding/setup (requires auth, user with org_id=0).
// Creates the org and assigns the calling user as admin.
func (h *Handler) onboardingSetup(c *fiber.Ctx) error {
	userID, _ := c.Locals("user_id").(int64)
	user, err := h.deps.DB.GetUserByID(userID)
	if err != nil || user == nil {
		return c.Status(404).JSON(fiber.Map{"error": "user not found"})
	}
	if models.IsPlatformRole(user.Role) {
		return c.Status(403).JSON(fiber.Map{"error": "founder accounts do not use workspace onboarding"})
	}
	if user.OrgID != 0 {
		return c.Status(409).JSON(fiber.Map{"error": "user already belongs to an organization"})
	}
	if claimed, err := h.attachProvisionedOrgIfNeeded(user, c.IP()); err != nil {
		log.Printf("[Onboarding] Provisioned org claim failed: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "workspace assignment failed"})
	} else if claimed {
		newToken, _ := authpkg.GenerateAccessToken(userID, user.OrgID, user.Email, string(user.Role), h.deps.JWTSecret)
		setAuthCookies(c, newToken, time.Now().Add(authpkg.AccessTokenTTL))
		return c.JSON(fiber.Map{
			"access_token": newToken,
			"org_id":       user.OrgID,
			"user": fiber.Map{
				"id":     userID,
				"org_id": user.OrgID,
				"email":  user.Email,
				"name":   user.Name,
				"role":   user.Role,
			},
		})
	}

	var req struct {
		OrgName          string `json:"org_name"`
		Domain           string `json:"domain"`
		Type             string `json:"type"` // "team" | "personal"
		BusinessIndustry string `json:"business_industry"`
		Services         string `json:"services"`
		TargetCustomers  string `json:"target_customers"`
		BusinessProfile  string `json:"business_profile"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}
	if req.OrgName == "" {
		return c.Status(400).JSON(fiber.Map{"error": "org_name required"})
	}

	org := &models.Organization{
		Name:        req.OrgName,
		Domain:      req.Domain,
		PlanTier:    models.PlanFree,
		MaxAccounts: models.PlanFree.MaxAccounts(),
		Active:      true,
	}
	orgID, err := h.deps.DB.CreateOrganization(org)
	if err != nil {
		log.Printf("[Onboarding] Create org error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "could not create organization"})
	}

	if err := h.deps.DB.UpdateUserOrg(userID, orgID, models.RoleAdmin); err != nil {
		log.Printf("[Onboarding] UpdateUserOrg error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "could not assign user to org"})
	}

	// Persist optional business positioning fields right away so the new
	// workspace already has its profile context — used by classifier, gate,
	// outbound generation, etc. Empty fields are skipped.
	profileFields := map[string]string{
		"business_name":     req.OrgName,
		"business_industry": strings.TrimSpace(req.BusinessIndustry),
		"services":          strings.TrimSpace(req.Services),
		"target_customers":  strings.TrimSpace(req.TargetCustomers),
		"business_profile":  strings.TrimSpace(req.BusinessProfile),
	}
	for key, value := range profileFields {
		if value == "" {
			continue
		}
		if err := h.deps.DB.Leads().SetContext(fmt.Sprintf("org:%d:%s", orgID, key), value); err != nil {
			log.Printf("[Onboarding] could not persist profile field %s for org %d: %v", key, orgID, err)
		}
	}

	user, _ = h.deps.DB.GetUserByID(userID)

	// Issue a new token with the correct org_id
	newToken, _ := authpkg.GenerateAccessToken(userID, orgID, user.Email, string(models.RoleAdmin), h.deps.JWTSecret)
	setAuthCookies(c, newToken, time.Now().Add(authpkg.AccessTokenTTL))

	h.deps.DB.InsertAuditLog(userID, "onboarding_complete", c.IP(), `{}`)
	log.Printf("[Onboarding] Org created: %q (id=%d) by user=%d", req.OrgName, orgID, userID)

	return c.JSON(fiber.Map{
		"access_token": newToken,
		"org": fiber.Map{
			"id":     orgID,
			"name":   req.OrgName,
			"domain": req.Domain,
		},
		"user": fiber.Map{
			"id":     userID,
			"org_id": orgID,
			"email":  user.Email,
			"name":   user.Name,
			"role":   models.RoleAdmin,
		},
	})
}

// listMyPendingInvites handles GET /api/auth/me/invites — returns invites
// matching the logged-in user's email that haven't been accepted or expired.
// This is what the "no workspace yet" landing uses to surface invites without
// the user needing the original invite link.
func (h *Handler) listMyPendingInvites(c *fiber.Ctx) error {
	userID, _ := c.Locals("user_id").(int64)
	user, err := h.deps.DB.GetUserByID(userID)
	if err != nil || user == nil {
		return c.Status(404).JSON(fiber.Map{"error": "user not found"})
	}
	rows, err := h.deps.DB.DB().QueryContext(c.Context(),
		`SELECT i.id, i.org_id, COALESCE(o.name, ''), i.email,
		        COALESCE(NULLIF(i.role, ''), 'sales'), i.token, i.expires_at, i.created_at
		 FROM org_invites i
		 LEFT JOIN organizations o ON o.id = i.org_id
		 WHERE lower(trim(i.email)) = lower(trim(?))
		   AND i.used_at IS NULL
		   AND i.revoked_at IS NULL
		   AND i.expires_at > CURRENT_TIMESTAMP
		 ORDER BY i.created_at DESC`, user.Email)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	defer rows.Close()
	type pending struct {
		ID        int64  `json:"id"`
		OrgID     int64  `json:"org_id"`
		OrgName   string `json:"org_name"`
		Email     string `json:"email"`
		Role      string `json:"role"`
		Token     string `json:"token"`
		ExpiresAt string `json:"expires_at"`
		CreatedAt string `json:"created_at"`
	}
	out := []pending{}
	for rows.Next() {
		var p pending
		if err := rows.Scan(&p.ID, &p.OrgID, &p.OrgName, &p.Email, &p.Role, &p.Token, &p.ExpiresAt, &p.CreatedAt); err != nil {
			continue
		}
		out = append(out, p)
	}
	return c.JSON(fiber.Map{"invites": out, "count": len(out)})
}

// searchInviteCandidates handles GET /api/org/invites/search?q=... (admin only).
// Powers the autocomplete on the invite form so admins can see whether the
// email already maps to a registered user, and (if so) which workspace they're
// currently in. Returns small projection (id, email, name, current org_id) —
// no password hashes or sensitive fields.
func (h *Handler) searchInviteCandidates(c *fiber.Ctx) error {
	q := strings.TrimSpace(c.Query("q", ""))
	if len([]rune(q)) < 2 {
		return c.JSON(fiber.Map{"users": []any{}, "count": 0})
	}
	users, err := h.deps.DB.SearchUsersForInvite(q, 8)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	type row struct {
		ID    int64  `json:"id"`
		Email string `json:"email"`
		Name  string `json:"name"`
		OrgID int64  `json:"org_id"`
		Role  string `json:"role"`
	}
	out := make([]row, 0, len(users))
	for _, u := range users {
		out = append(out, row{ID: u.ID, Email: u.Email, Name: u.Name, OrgID: u.OrgID, Role: string(u.Role)})
	}
	return c.JSON(fiber.Map{"users": out, "count": len(out)})
}

// createInvite handles POST /api/org/invites (admin only).
func (h *Handler) createInvite(c *fiber.Ctx) error {
	orgID, _ := c.Locals("org_id").(int64)
	userID, _ := c.Locals("user_id").(int64)
	if orgID == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "workspace context required"})
	}
	var req struct {
		Email string `json:"email"`
		Role  string `json:"role"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	req.Role = strings.ToLower(strings.TrimSpace(req.Role))
	if req.Email == "" {
		return c.Status(400).JSON(fiber.Map{"error": "email is required"})
	}
	if req.Role == "" {
		req.Role = string(models.RoleSales)
	}
	if req.Role != string(models.RoleAdmin) && req.Role != string(models.RoleSales) {
		return c.Status(400).JSON(fiber.Map{"error": "role must be admin or sales"})
	}
	// We allow inviting users who are already a member of a different workspace.
	// On accept, the invitee is moved to this workspace (single-org membership model).
	// Only block re-inviting an existing member of THIS workspace, or platform users.
	if existing, _ := h.deps.DB.GetUserByEmail(req.Email); existing != nil {
		if existing.OrgID == orgID {
			return c.Status(409).JSON(fiber.Map{"error": "email is already a workspace member"})
		}
		if models.IsPlatformRole(existing.Role) {
			return c.Status(409).JSON(fiber.Map{"error": "platform/founder accounts cannot join workspaces"})
		}
	}
	var pendingID int64
	if err := h.deps.DB.DB().QueryRowContext(c.Context(),
		`SELECT id FROM org_invites
		 WHERE org_id = ? AND lower(trim(email)) = ? AND used_at IS NULL AND revoked_at IS NULL AND expires_at > CURRENT_TIMESTAMP
		 LIMIT 1`, orgID, req.Email).Scan(&pendingID); err == nil {
		return c.Status(409).JSON(fiber.Map{"error": "pending invite already exists for this email"})
	}

	token, err := randomHex(20)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "token generation failed"})
	}
	expiresAt := time.Now().Add(7 * 24 * time.Hour)

	res, err := h.deps.DB.DB().ExecContext(c.Context(),
		`INSERT INTO org_invites (org_id, email, role, token, created_by, expires_at) VALUES (?, ?, ?, ?, ?, ?)`,
		orgID, req.Email, req.Role, token, userID, expiresAt)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	inviteID, _ := res.LastInsertId()
	h.deps.DB.InsertAuditLog(userID, "workspace_invite_created", c.IP(),
		fmt.Sprintf(`{"org_id":%d,"email":%q,"role":%q}`, orgID, req.Email, req.Role))
	emailResult := h.sendWorkspaceInviteEmail(c, inviteID, orgID, userID, req.Email, req.Role, token, expiresAt.Format(time.RFC3339))
	// Already-registered invitees also get an in-app bell notification.
	if inviter, _ := h.deps.DB.GetUserByID(userID); inviter != nil {
		h.notifyInviteReceived(orgID, h.orgNameOf(orgID), inviter.Name, req.Email, req.Role, token)
	}

	return c.Status(201).JSON(fiber.Map{
		"id":              inviteID,
		"token":           token,
		"invite_url":      "/join/" + token,
		"invite_full_url": emailResult.URL,
		"email":           req.Email,
		"role":            req.Role,
		"created_by":      userID,
		"expires_at":      expiresAt,
		"created_at":      time.Now(),
		"email_status":    emailResult.Status,
		"email_error":     emailResult.Error,
	})
}

// listInvites handles GET /api/org/invites (admin only).
func (h *Handler) listInvites(c *fiber.Ctx) error {
	orgID, _ := c.Locals("org_id").(int64)
	if orgID == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "workspace context required"})
	}
	// All recent invites, with the derived lifecycle status the admin
	// table renders: pending | accepted | expired | revoked (PR-1).
	rows, err := h.deps.DB.DB().QueryContext(c.Context(),
		`SELECT id, email, COALESCE(NULLIF(role, ''), 'sales'), token, created_by, expires_at, used_at,
		        revoked_at, created_at,
		        COALESCE(NULLIF(email_status, ''), 'pending'), COALESCE(email_error, ''),
		        CASE
		          WHEN revoked_at IS NOT NULL THEN 'revoked'
		          WHEN used_at IS NOT NULL THEN 'accepted'
		          WHEN expires_at <= CURRENT_TIMESTAMP THEN 'expired'
		          ELSE 'pending'
		        END
		 FROM org_invites WHERE org_id = ?
		 ORDER BY created_at DESC LIMIT 100`, orgID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	defer rows.Close()

	type inviteRow struct {
		ID          int64   `json:"id"`
		Email       string  `json:"email"`
		Role        string  `json:"role"`
		Token       string  `json:"token"`
		InviteURL   string  `json:"invite_url"`
		CreatedBy   int64   `json:"created_by"`
		ExpiresAt   string  `json:"expires_at"`
		UsedAt      *string `json:"used_at"`
		RevokedAt   *string `json:"revoked_at"`
		CreatedAt   string  `json:"created_at"`
		EmailStatus string  `json:"email_status"`
		EmailError  string  `json:"email_error"`
		Status      string  `json:"status"`
	}
	var invites []inviteRow
	for rows.Next() {
		var inv inviteRow
		var usedAt, revokedAt *string
		if err := rows.Scan(&inv.ID, &inv.Email, &inv.Role, &inv.Token, &inv.CreatedBy, &inv.ExpiresAt, &usedAt, &revokedAt, &inv.CreatedAt, &inv.EmailStatus, &inv.EmailError, &inv.Status); err != nil {
			continue
		}
		inv.UsedAt = usedAt
		inv.RevokedAt = revokedAt
		inv.InviteURL = "/join/" + inv.Token
		invites = append(invites, inv)
	}
	return c.JSON(fiber.Map{"invites": invites, "count": len(invites)})
}

// resendInvite handles POST /api/org/invites/:id/resend (admin only).
func (h *Handler) resendInvite(c *fiber.Ctx) error {
	orgID, _ := c.Locals("org_id").(int64)
	userID, _ := c.Locals("user_id").(int64)
	if orgID == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "workspace context required"})
	}
	id, _ := c.ParamsInt("id")
	row := h.deps.DB.DB().QueryRowContext(c.Context(),
		`SELECT email, COALESCE(NULLIF(role, ''), 'sales'), token, expires_at
		 FROM org_invites
		 WHERE id = ? AND org_id = ? AND used_at IS NULL AND revoked_at IS NULL AND expires_at > CURRENT_TIMESTAMP`,
		id, orgID)
	var email, role, token, expiresAt string
	if err := row.Scan(&email, &role, &token, &expiresAt); err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "invite not found or expired"})
	}
	result := h.sendWorkspaceInviteEmail(c, int64(id), orgID, userID, email, role, token, expiresAt)
	code := fiber.StatusOK
	if result.Status == "failed" {
		code = fiber.StatusBadGateway
	}
	return c.Status(code).JSON(fiber.Map{
		"id":              id,
		"email":           email,
		"role":            role,
		"invite_url":      "/join/" + token,
		"invite_full_url": result.URL,
		"email_status":    result.Status,
		"email_error":     result.Error,
	})
}

// revokeInvite handles DELETE /api/org/invites/:id (admin only).
func (h *Handler) revokeInvite(c *fiber.Ctx) error {
	orgID, _ := c.Locals("org_id").(int64)
	if orgID == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "workspace context required"})
	}
	id, _ := c.ParamsInt("id")
	// Soft revoke (PR-1): keep the row so the admin table shows the
	// 'revoked' status; accepted invites cannot be revoked retroactively.
	_, err := h.deps.DB.DB().ExecContext(c.Context(),
		`UPDATE org_invites SET revoked_at = CURRENT_TIMESTAMP
		  WHERE id = ? AND org_id = ? AND used_at IS NULL AND revoked_at IS NULL`, id, orgID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"ok": true})
}

// getInviteInfo handles GET /api/auth/invite/:token (public).
// Returns org name + email hint so the join page can show context.
func (h *Handler) getInviteInfo(c *fiber.Ctx) error {
	token := c.Params("token")
	row := h.deps.DB.DB().QueryRowContext(c.Context(),
		`SELECT i.email, COALESCE(NULLIF(i.role, ''), 'sales'), i.expires_at, o.name
		 FROM org_invites i JOIN organizations o ON o.id = i.org_id
		 WHERE i.token = ? AND i.used_at IS NULL AND i.revoked_at IS NULL AND i.expires_at > CURRENT_TIMESTAMP`, token)

	var email, role, expiresAt, orgName string
	if err := row.Scan(&email, &role, &expiresAt, &orgName); err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "invite not found or expired"})
	}
	return c.JSON(fiber.Map{"org_name": orgName, "email": email, "role": role, "expires_at": expiresAt})
}

// acceptInvite handles POST /api/auth/join/:token (requires auth).
// Assigns the logged-in user to the org associated with the invite.
func (h *Handler) acceptInvite(c *fiber.Ctx) error {
	userID, _ := c.Locals("user_id").(int64)
	token := c.Params("token")
	user, err := h.deps.DB.GetUserByID(userID)
	if err != nil || user == nil {
		return c.Status(404).JSON(fiber.Map{"error": "user not found"})
	}
	if models.IsPlatformRole(user.Role) {
		return c.Status(403).JSON(fiber.Map{"error": "founder accounts cannot join workspaces"})
	}

	var inviteID, orgID int64
	var email, role string
	row := h.deps.DB.DB().QueryRowContext(c.Context(),
		`SELECT id, org_id, email, COALESCE(NULLIF(role, ''), 'sales') FROM org_invites
		 WHERE token = ? AND used_at IS NULL AND revoked_at IS NULL AND expires_at > CURRENT_TIMESTAMP`, token)
	if err := row.Scan(&inviteID, &orgID, &email, &role); err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "invite not found or expired"})
	}
	if !strings.EqualFold(strings.TrimSpace(user.Email), strings.TrimSpace(email)) {
		return c.Status(403).JSON(fiber.Map{"error": "invite email does not match current account"})
	}
	previousOrgID := user.OrgID
	targetRole := models.RoleSales
	if role == string(models.RoleAdmin) {
		targetRole = models.RoleAdmin
	}
	// Single-org membership: accepting an invite from a different workspace
	// transfers the user out of their previous org. Audit log records the move.
	if err := h.deps.DB.UpdateUserOrg(userID, orgID, targetRole); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "could not join org"})
	}
	if err := h.deps.DB.MarkInviteUsed(inviteID, userID); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "could not complete invite"})
	}
	_ = h.deps.DB.App().UpsertStaffKPI(userID, orgID, app.KPIDelta{})

	user, _ = h.deps.DB.GetUserByID(userID)
	newToken, _ := authpkg.GenerateAccessToken(userID, orgID, user.Email, string(targetRole), h.deps.JWTSecret)
	setAuthCookies(c, newToken, time.Now().Add(authpkg.AccessTokenTTL))

	h.deps.DB.InsertAuditLog(userID, "invite_accepted", c.IP(),
		fmt.Sprintf(`{"org_id":%d,"role":%q,"invite_id":%d,"previous_org_id":%d}`, orgID, targetRole, inviteID, previousOrgID))
	h.deps.DB.InsertAuditLog(userID, "membership_granted", c.IP(),
		fmt.Sprintf(`{"org_id":%d,"role":%q,"invite_id":%d}`, orgID, targetRole, inviteID))
	h.notifyInviteAccepted(orgID, inviteID, user.Name, user.Email, string(targetRole))

	return c.JSON(fiber.Map{
		"access_token": newToken,
		"org_id":       orgID,
		"org_name":     h.orgNameOf(orgID),
		"role":         targetRole,
		"user": fiber.Map{
			"id":     userID,
			"org_id": orgID,
			"email":  user.Email,
			"name":   user.Name,
			"role":   targetRole,
		},
	})
}
