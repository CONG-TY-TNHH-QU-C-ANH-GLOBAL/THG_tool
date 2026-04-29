package server

import (
	"log"
	"time"

	"github.com/gofiber/fiber/v2"
	authpkg "github.com/thg/scraper/internal/auth"
	"github.com/thg/scraper/internal/models"
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
	if claim, err := s.db.FindProvisionedOrgByEmail(req.Email); err != nil {
		log.Printf("[Signup] Provisioned org lookup error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "workspace assignment failed"})
	} else if claim != nil {
		orgID = claim.OrgID
		role = claim.Role
		needsOnboarding = false
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
	var req struct {
		Email string `json:"email"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}

	token, err := randomHex(20)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "token generation failed"})
	}
	expiresAt := time.Now().Add(7 * 24 * time.Hour)

	_, err = s.db.DB().ExecContext(c.Context(),
		`INSERT INTO org_invites (org_id, email, token, created_by, expires_at) VALUES (?, ?, ?, ?, ?)`,
		orgID, req.Email, token, userID, expiresAt)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(201).JSON(fiber.Map{
		"token":      token,
		"invite_url": "/join/" + token,
		"email":      req.Email,
		"expires_at": expiresAt,
	})
}

// listInvites handles GET /api/org/invites (admin only).
func (s *Server) listInvites(c *fiber.Ctx) error {
	orgID, _ := c.Locals("org_id").(int64)
	rows, err := s.db.DB().QueryContext(c.Context(),
		`SELECT id, email, token, created_by, expires_at, used_at, created_at
		 FROM org_invites WHERE org_id = ? AND used_at IS NULL AND expires_at > CURRENT_TIMESTAMP
		 ORDER BY created_at DESC`, orgID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	defer rows.Close()

	type inviteRow struct {
		ID        int64   `json:"id"`
		Email     string  `json:"email"`
		Token     string  `json:"token"`
		CreatedBy int64   `json:"created_by"`
		ExpiresAt string  `json:"expires_at"`
		UsedAt    *string `json:"used_at"`
		CreatedAt string  `json:"created_at"`
	}
	var invites []inviteRow
	for rows.Next() {
		var inv inviteRow
		var usedAt *string
		if err := rows.Scan(&inv.ID, &inv.Email, &inv.Token, &inv.CreatedBy, &inv.ExpiresAt, &usedAt, &inv.CreatedAt); err != nil {
			continue
		}
		inv.UsedAt = usedAt
		invites = append(invites, inv)
	}
	return c.JSON(fiber.Map{"invites": invites, "count": len(invites)})
}

// revokeInvite handles DELETE /api/org/invites/:id (admin only).
func (s *Server) revokeInvite(c *fiber.Ctx) error {
	orgID, _ := c.Locals("org_id").(int64)
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
		`SELECT i.email, i.expires_at, o.name
		 FROM org_invites i JOIN organizations o ON o.id = i.org_id
		 WHERE i.token = ? AND i.used_at IS NULL AND i.expires_at > CURRENT_TIMESTAMP`, token)

	var email, expiresAt, orgName string
	if err := row.Scan(&email, &expiresAt, &orgName); err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "invite not found or expired"})
	}
	return c.JSON(fiber.Map{"org_name": orgName, "email": email, "expires_at": expiresAt})
}

// acceptInvite handles POST /api/auth/join/:token (requires auth).
// Assigns the logged-in user to the org associated with the invite.
func (s *Server) acceptInvite(c *fiber.Ctx) error {
	userID, _ := c.Locals("user_id").(int64)
	token := c.Params("token")

	var orgID int64
	var email string
	row := s.db.DB().QueryRowContext(c.Context(),
		`SELECT org_id, email FROM org_invites
		 WHERE token = ? AND used_at IS NULL AND expires_at > CURRENT_TIMESTAMP`, token)
	if err := row.Scan(&orgID, &email); err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "invite not found or expired"})
	}

	if err := s.db.UpdateUserOrg(userID, orgID, models.RoleSales); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "could not join org"})
	}

	// Mark invite as used
	s.db.DB().ExecContext(c.Context(),
		`UPDATE org_invites SET used_at = CURRENT_TIMESTAMP WHERE token = ?`, token)

	user, _ := s.db.GetUserByID(userID)
	newToken, _ := authpkg.GenerateAccessToken(userID, orgID, user.Email, string(models.RoleSales), s.cfg.JWTSecret)

	s.db.InsertAuditLog(userID, "invite_accepted", c.IP(), `{}`)

	return c.JSON(fiber.Map{
		"access_token": newToken,
		"org_id":       orgID,
		"user": fiber.Map{
			"id":     userID,
			"org_id": orgID,
			"email":  user.Email,
			"name":   user.Name,
			"role":   models.RoleSales,
		},
	})
}
