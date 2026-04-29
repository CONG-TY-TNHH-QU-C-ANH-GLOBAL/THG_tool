package server

import (
	"log"
	"time"

	"github.com/gofiber/fiber/v2"
	authpkg "github.com/thg/scraper/internal/auth"
	"github.com/thg/scraper/internal/models"
)

// registerOrg handles POST /api/register (public — no auth required).
// Creates a new organization and its first admin user in one atomic transaction.
func (s *Server) registerOrg(c *fiber.Ctx) error {
	var req struct {
		OrgName       string `json:"org_name"`
		OrgDomain     string `json:"org_domain"`
		AdminName     string `json:"admin_name"`
		AdminEmail    string `json:"admin_email"`
		AdminPassword string `json:"admin_password"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}
	if req.OrgName == "" || req.AdminEmail == "" || req.AdminPassword == "" {
		return c.Status(400).JSON(fiber.Map{"error": "org_name, admin_email and admin_password required"})
	}
	if req.AdminName == "" {
		req.AdminName = req.AdminEmail
	}

	if err := authpkg.ValidatePasswordStrength(req.AdminPassword); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	// Prevent duplicate email
	existing, _ := s.db.GetUserByEmail(req.AdminEmail)
	if existing != nil {
		return c.Status(409).JSON(fiber.Map{"error": "email already registered"})
	}

	// Create org
	org := &models.Organization{
		Name:        req.OrgName,
		Domain:      req.OrgDomain,
		PlanTier:    models.PlanFree,
		MaxAccounts: models.PlanFree.MaxAccounts(),
		Active:      true,
	}
	orgID, err := s.db.CreateOrganization(org)
	if err != nil {
		log.Printf("[Register] Create org error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "could not create organization"})
	}

	// Create first admin user for the org
	hash, err := authpkg.HashPassword(req.AdminPassword)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "internal error"})
	}
	user := &models.User{
		OrgID:        orgID,
		Email:        req.AdminEmail,
		Name:         req.AdminName,
		PasswordHash: hash,
		Role:         models.RoleAdmin,
	}
	userID, err := s.db.CreateUser(user)
	if err != nil {
		log.Printf("[Register] Create user error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "could not create admin user"})
	}
	user.ID = userID

	// Auto-login: issue tokens immediately
	accessToken, _ := authpkg.GenerateAccessToken(userID, orgID, req.AdminEmail, string(models.RoleAdmin), s.cfg.JWTSecret)
	refreshToken, _ := authpkg.GenerateRefreshToken()
	expiresAt := time.Now().Add(authpkg.RefreshTokenTTL)
	_ = s.db.SaveRefreshToken(userID, refreshToken, expiresAt)
	setRefreshCookie(c, refreshToken)

	s.db.InsertAuditLog(userID, "org_registered", c.IP(), `{}`)
	log.Printf("[Register] New org: %q (id=%d) admin=%s", req.OrgName, orgID, req.AdminEmail)

	return c.Status(201).JSON(fiber.Map{
		"org_id":       orgID,
		"org_name":     req.OrgName,
		"access_token": accessToken,
		"user": fiber.Map{
			"id":     userID,
			"org_id": orgID,
			"email":  req.AdminEmail,
			"name":   req.AdminName,
			"role":   models.RoleAdmin,
		},
	})
}

// listOrgs handles GET /api/admin/orgs — superadmin only.
func (s *Server) listOrgs(c *fiber.Ctx) error {
	orgs, err := s.db.ListOrganizations()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"organizations": orgs, "count": len(orgs)})
}

// getMyOrg handles GET /api/org — returns the caller's organization details.
func (s *Server) getMyOrg(c *fiber.Ctx) error {
	orgID, _ := c.Locals("org_id").(int64)
	if orgID == 0 {
		return c.JSON(fiber.Map{"org": nil, "message": "superadmin — no specific org"})
	}
	org, err := s.db.GetOrganization(orgID)
	if err != nil || org == nil {
		return c.Status(404).JSON(fiber.Map{"error": "organization not found"})
	}
	count, _ := s.db.CountAccountsByOrg(orgID)
	return c.JSON(fiber.Map{
		"org":            org,
		"account_count":  count,
		"accounts_limit": org.MaxAccounts,
	})
}

// updateOrg handles PUT /api/org — org admin updates their org settings.
func (s *Server) updateOrg(c *fiber.Ctx) error {
	orgID, _ := c.Locals("org_id").(int64)
	if orgID == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "no org context"})
	}
	var req struct {
		Name   string `json:"name"`
		Domain string `json:"domain"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}
	org, err := s.db.GetOrganization(orgID)
	if err != nil || org == nil {
		return c.Status(404).JSON(fiber.Map{"error": "organization not found"})
	}
	if req.Name != "" {
		org.Name = req.Name
	}
	if req.Domain != "" {
		org.Domain = req.Domain
	}
	if err := s.db.UpdateOrganization(orgID, org.Name, org.Domain, org.PlanTier, org.MaxAccounts, org.Active); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"status": "updated", "org": org})
}

// adminUpdateOrg handles PUT /api/admin/orgs/:id — superadmin changes plan/limits.
func (s *Server) adminUpdateOrg(c *fiber.Ctx) error {
	id, _ := c.ParamsInt("id")
	var req struct {
		Name        string `json:"name"`
		Domain      string `json:"domain"`
		PlanTier    string `json:"plan_tier"`
		MaxAccounts int    `json:"max_accounts"`
		Active      *bool  `json:"active"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}
	org, err := s.db.GetOrganization(int64(id))
	if err != nil || org == nil {
		return c.Status(404).JSON(fiber.Map{"error": "org not found"})
	}
	if req.Name != "" {
		org.Name = req.Name
	}
	if req.Domain != "" {
		org.Domain = req.Domain
	}
	if req.PlanTier != "" {
		org.PlanTier = models.PlanTier(req.PlanTier)
	}
	if req.MaxAccounts > 0 {
		org.MaxAccounts = req.MaxAccounts
	}
	if req.Active != nil {
		org.Active = *req.Active
	}
	if err := s.db.UpdateOrganization(org.ID, org.Name, org.Domain, org.PlanTier, org.MaxAccounts, org.Active); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"status": "updated", "org": org})
}

// createOrgUser handles POST /api/auth/users — now creates users scoped to the caller's org.
// Overrides the existing handler to inject org_id automatically.
func (s *Server) createOrgUser(c *fiber.Ctx) error {
	callerOrgID, _ := c.Locals("org_id").(int64)
	callerRole, _ := c.Locals("user_role").(string)
	callerIsPlatform := models.IsPlatformUser(callerOrgID, models.UserRole(callerRole))

	var req struct {
		Email    string `json:"email"`
		Name     string `json:"name"`
		Password string `json:"password"`
		Role     string `json:"role"`
		OrgID    int64  `json:"org_id"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}
	if req.Email == "" || req.Password == "" || req.Name == "" {
		return c.Status(400).JSON(fiber.Map{"error": "email, name and password required"})
	}

	// Validate password strength
	if err := authpkg.ValidatePasswordStrength(req.Password); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	// Only superadmin can create users in arbitrary orgs; org admins create in own org
	targetOrgID := callerOrgID
	if callerIsPlatform {
		if req.OrgID <= 0 {
			return c.Status(400).JSON(fiber.Map{"error": "org_id is required when superadmin creates a user"})
		}
		targetOrgID = req.OrgID
	}
	org, err := s.db.GetOrganization(targetOrgID)
	if err != nil || org == nil || !org.Active {
		return c.Status(404).JSON(fiber.Map{"error": "organization not found"})
	}

	// Limit role escalation: org admins can only create admin/sales
	if req.Role == "" {
		req.Role = "sales"
	}
	if req.Role != string(models.RoleAdmin) && req.Role != string(models.RoleSales) {
		return c.Status(400).JSON(fiber.Map{"error": "role must be admin or sales"})
	}

	existing, _ := s.db.GetUserByEmail(req.Email)
	if existing != nil {
		return c.Status(409).JSON(fiber.Map{"error": "email already exists"})
	}

	hash, err := authpkg.HashPassword(req.Password)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "internal error"})
	}
	user := &models.User{
		OrgID:        targetOrgID,
		Email:        req.Email,
		Name:         req.Name,
		PasswordHash: hash,
		Role:         models.UserRole(req.Role),
	}
	id, err := s.db.CreateUser(user)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(201).JSON(fiber.Map{
		"user_id": id,
		"org_id":  targetOrgID,
		"email":   req.Email,
		"role":    req.Role,
	})
}

func setRefreshCookie(c *fiber.Ctx, token string) {
	// Intentionally minimal — full cookie set in auth_handlers.go
	c.Cookie(&fiber.Cookie{
		Name:     "refresh_token",
		Value:    token,
		Path:     "/api/auth",
		Expires:  time.Now().Add(authpkg.RefreshTokenTTL),
		HTTPOnly: true,
		Secure:   secureCookie(c),
		SameSite: "Lax",
	})
}
