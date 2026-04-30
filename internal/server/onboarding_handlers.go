package server

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	authpkg "github.com/thg/scraper/internal/auth"
	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store"
)

// signupUser handles POST /api/auth/signup (public).
// Creates a user WITHOUT an org — org is created in the onboarding step.
func (s *Server) signupUser(c *fiber.Ctx) error {
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

	existing, _ := s.db.GetUserByEmail(req.Email)
	if existing != nil {
		return c.Status(409).JSON(fiber.Map{"error": "email already registered"})
	}

	orgID := int64(0)
	role := models.RoleAdmin
	needsOnboarding := true
	var provisionedClaim *store.ProvisionedOrgClaim
	if claim, err := s.db.FindProvisionedOrgByEmail(req.Email); err != nil {
		log.Printf("[Signup] Provisioned org lookup error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "workspace assignment failed"})
	} else if claim != nil {
		orgID = claim.OrgID
		role = claim.Role
		needsOnboarding = false
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
	userID, err := s.db.CreateUser(user)
	if err != nil {
		log.Printf("[Signup] Create user error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "could not create user"})
	}
	if err := s.completeProvisionedClaim(userID, provisionedClaim, c.IP()); err != nil {
		log.Printf("[Signup] Complete provisioned claim error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "workspace assignment failed"})
	}

	accessToken, _ := authpkg.GenerateAccessToken(userID, orgID, req.Email, string(role), s.cfg.JWTSecret)
	refreshToken, _ := authpkg.GenerateRefreshToken()
	expiresAt := time.Now().Add(authpkg.RefreshTokenTTL)
	_ = s.db.SaveRefreshToken(userID, refreshToken, expiresAt)
	setRefreshCookie(c, refreshToken)
	s.db.InsertAuditLog(userID, "signup", c.IP(), `{}`)

	return c.Status(201).JSON(fiber.Map{
		"access_token":     accessToken,
		"needs_onboarding": needsOnboarding,
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
func (s *Server) onboardingSetup(c *fiber.Ctx) error {
	userID, _ := c.Locals("user_id").(int64)
	user, err := s.db.GetUserByID(userID)
	if err != nil || user == nil {
		return c.Status(404).JSON(fiber.Map{"error": "user not found"})
	}
	if models.IsPlatformRole(user.Role) {
		return c.Status(403).JSON(fiber.Map{"error": "founder accounts do not use workspace onboarding"})
	}
	if user.OrgID != 0 {
		return c.Status(409).JSON(fiber.Map{"error": "user already belongs to an organization"})
	}
	if claimed, err := s.attachProvisionedOrgIfNeeded(user, c.IP()); err != nil {
		log.Printf("[Onboarding] Provisioned org claim failed: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "workspace assignment failed"})
	} else if claimed {
		newToken, _ := authpkg.GenerateAccessToken(userID, user.OrgID, user.Email, string(user.Role), s.cfg.JWTSecret)
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
		OrgName string `json:"org_name"`
		Domain  string `json:"domain"`
		Type    string `json:"type"` // "team" | "personal"
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
	orgID, err := s.db.CreateOrganization(org)
	if err != nil {
		log.Printf("[Onboarding] Create org error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "could not create organization"})
	}

	if err := s.db.UpdateUserOrg(userID, orgID, models.RoleAdmin); err != nil {
		log.Printf("[Onboarding] UpdateUserOrg error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "could not assign user to org"})
	}

	user, _ = s.db.GetUserByID(userID)

	// Issue a new token with the correct org_id
	newToken, _ := authpkg.GenerateAccessToken(userID, orgID, user.Email, string(models.RoleAdmin), s.cfg.JWTSecret)

	s.db.InsertAuditLog(userID, "onboarding_complete", c.IP(), `{}`)
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

// createInvite handles POST /api/org/invites (admin only).
func (s *Server) createInvite(c *fiber.Ctx) error {
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
	if existing, _ := s.db.GetUserByEmail(req.Email); existing != nil {
		if existing.OrgID == orgID {
			return c.Status(409).JSON(fiber.Map{"error": "email is already a workspace member"})
		}
		if existing.OrgID != 0 {
			return c.Status(409).JSON(fiber.Map{"error": "email already belongs to another workspace"})
		}
	}
	var pendingID int64
	if err := s.db.DB().QueryRowContext(c.Context(),
		`SELECT id FROM org_invites
		 WHERE org_id = ? AND lower(trim(email)) = ? AND used_at IS NULL AND expires_at > CURRENT_TIMESTAMP
		 LIMIT 1`, orgID, req.Email).Scan(&pendingID); err == nil {
		return c.Status(409).JSON(fiber.Map{"error": "pending invite already exists for this email"})
	}

	token, err := randomHex(20)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "token generation failed"})
	}
	expiresAt := time.Now().Add(7 * 24 * time.Hour)

	res, err := s.db.DB().ExecContext(c.Context(),
		`INSERT INTO org_invites (org_id, email, role, token, created_by, expires_at) VALUES (?, ?, ?, ?, ?, ?)`,
		orgID, req.Email, req.Role, token, userID, expiresAt)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	inviteID, _ := res.LastInsertId()
	s.db.InsertAuditLog(userID, "workspace_invite_created", c.IP(),
		fmt.Sprintf(`{"org_id":%d,"email":%q,"role":%q}`, orgID, req.Email, req.Role))
	emailResult := s.sendWorkspaceInviteEmail(c, inviteID, orgID, userID, req.Email, req.Role, token, expiresAt.Format(time.RFC3339))

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
func (s *Server) listInvites(c *fiber.Ctx) error {
	orgID, _ := c.Locals("org_id").(int64)
	if orgID == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "workspace context required"})
	}
	rows, err := s.db.DB().QueryContext(c.Context(),
		`SELECT id, email, COALESCE(NULLIF(role, ''), 'sales'), token, created_by, expires_at, used_at, created_at,
		        COALESCE(NULLIF(email_status, ''), 'pending'), COALESCE(email_error, '')
		 FROM org_invites WHERE org_id = ? AND used_at IS NULL AND expires_at > CURRENT_TIMESTAMP
		 ORDER BY created_at DESC`, orgID)
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
		CreatedAt   string  `json:"created_at"`
		EmailStatus string  `json:"email_status"`
		EmailError  string  `json:"email_error"`
	}
	var invites []inviteRow
	for rows.Next() {
		var inv inviteRow
		var usedAt *string
		if err := rows.Scan(&inv.ID, &inv.Email, &inv.Role, &inv.Token, &inv.CreatedBy, &inv.ExpiresAt, &usedAt, &inv.CreatedAt, &inv.EmailStatus, &inv.EmailError); err != nil {
			continue
		}
		inv.UsedAt = usedAt
		inv.InviteURL = "/join/" + inv.Token
		invites = append(invites, inv)
	}
	return c.JSON(fiber.Map{"invites": invites, "count": len(invites)})
}

// resendInvite handles POST /api/org/invites/:id/resend (admin only).
func (s *Server) resendInvite(c *fiber.Ctx) error {
	orgID, _ := c.Locals("org_id").(int64)
	userID, _ := c.Locals("user_id").(int64)
	if orgID == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "workspace context required"})
	}
	id, _ := c.ParamsInt("id")
	row := s.db.DB().QueryRowContext(c.Context(),
		`SELECT email, COALESCE(NULLIF(role, ''), 'sales'), token, expires_at
		 FROM org_invites
		 WHERE id = ? AND org_id = ? AND used_at IS NULL AND expires_at > CURRENT_TIMESTAMP`,
		id, orgID)
	var email, role, token, expiresAt string
	if err := row.Scan(&email, &role, &token, &expiresAt); err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "invite not found or expired"})
	}
	result := s.sendWorkspaceInviteEmail(c, int64(id), orgID, userID, email, role, token, expiresAt)
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
func (s *Server) revokeInvite(c *fiber.Ctx) error {
	orgID, _ := c.Locals("org_id").(int64)
	if orgID == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "workspace context required"})
	}
	id, _ := c.ParamsInt("id")
	_, err := s.db.DB().ExecContext(c.Context(),
		`DELETE FROM org_invites WHERE id = ? AND org_id = ?`, id, orgID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"ok": true})
}

// getInviteInfo handles GET /api/auth/invite/:token (public).
// Returns org name + email hint so the join page can show context.
func (s *Server) getInviteInfo(c *fiber.Ctx) error {
	token := c.Params("token")
	row := s.db.DB().QueryRowContext(c.Context(),
		`SELECT i.email, COALESCE(NULLIF(i.role, ''), 'sales'), i.expires_at, o.name
		 FROM org_invites i JOIN organizations o ON o.id = i.org_id
		 WHERE i.token = ? AND i.used_at IS NULL AND i.expires_at > CURRENT_TIMESTAMP`, token)

	var email, role, expiresAt, orgName string
	if err := row.Scan(&email, &role, &expiresAt, &orgName); err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "invite not found or expired"})
	}
	return c.JSON(fiber.Map{"org_name": orgName, "email": email, "role": role, "expires_at": expiresAt})
}

// acceptInvite handles POST /api/auth/join/:token (requires auth).
// Assigns the logged-in user to the org associated with the invite.
func (s *Server) acceptInvite(c *fiber.Ctx) error {
	userID, _ := c.Locals("user_id").(int64)
	token := c.Params("token")
	user, err := s.db.GetUserByID(userID)
	if err != nil || user == nil {
		return c.Status(404).JSON(fiber.Map{"error": "user not found"})
	}
	if models.IsPlatformRole(user.Role) {
		return c.Status(403).JSON(fiber.Map{"error": "founder accounts cannot join workspaces"})
	}

	var inviteID, orgID int64
	var email, role string
	row := s.db.DB().QueryRowContext(c.Context(),
		`SELECT id, org_id, email, COALESCE(NULLIF(role, ''), 'sales') FROM org_invites
		 WHERE token = ? AND used_at IS NULL AND expires_at > CURRENT_TIMESTAMP`, token)
	if err := row.Scan(&inviteID, &orgID, &email, &role); err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "invite not found or expired"})
	}
	if !strings.EqualFold(strings.TrimSpace(user.Email), strings.TrimSpace(email)) {
		return c.Status(403).JSON(fiber.Map{"error": "invite email does not match current account"})
	}
	if user.OrgID != 0 && user.OrgID != orgID {
		return c.Status(409).JSON(fiber.Map{"error": "account already belongs to another workspace"})
	}
	targetRole := models.RoleSales
	if role == string(models.RoleAdmin) {
		targetRole = models.RoleAdmin
	}

	if err := s.db.UpdateUserOrg(userID, orgID, targetRole); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "could not join org"})
	}
	if err := s.db.MarkInviteUsed(inviteID, userID); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "could not complete invite"})
	}
	_ = s.db.UpsertStaffKPI(userID, orgID, store.KPIDelta{})

	user, _ = s.db.GetUserByID(userID)
	newToken, _ := authpkg.GenerateAccessToken(userID, orgID, user.Email, string(targetRole), s.cfg.JWTSecret)

	s.db.InsertAuditLog(userID, "invite_accepted", c.IP(),
		fmt.Sprintf(`{"org_id":%d,"role":%q,"invite_id":%d}`, orgID, targetRole, inviteID))

	return c.JSON(fiber.Map{
		"access_token": newToken,
		"org_id":       orgID,
		"user": fiber.Map{
			"id":     userID,
			"org_id": orgID,
			"email":  user.Email,
			"name":   user.Name,
			"role":   targetRole,
		},
	})
}
