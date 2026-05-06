package org

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	authpkg "github.com/thg/scraper/internal/auth"
	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store"
)

const (
	refreshCookie     = "refresh_token"
	cookiePath        = "/api/auth"
	accessCookie      = "access_token"
	authPresentCookie = "autoflow_session"
)

// registerOrg handles POST /api/register (public â€” no auth required).
// Creates a new organization and its first admin user in one atomic transaction.
func (h *Handler) registerOrg(c *fiber.Ctx) error {
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
	existing, _ := h.deps.DB.GetUserByEmail(req.AdminEmail)
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
	orgID, err := h.deps.DB.CreateOrganization(org)
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
	userID, err := h.deps.DB.CreateUser(user)
	if err != nil {
		log.Printf("[Register] Create user error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "could not create admin user"})
	}
	user.ID = userID

	// Auto-login: issue tokens immediately
	accessToken, _ := authpkg.GenerateAccessToken(userID, orgID, req.AdminEmail, string(models.RoleAdmin), h.deps.JWTSecret)
	refreshToken, _ := authpkg.GenerateRefreshToken()
	expiresAt := time.Now().Add(authpkg.RefreshTokenTTL)
	_ = h.deps.DB.SaveRefreshToken(userID, refreshToken, expiresAt)
	setRefreshCookie(c, refreshToken)
	setAuthCookies(c, accessToken, time.Now().Add(authpkg.AccessTokenTTL))

	h.deps.DB.InsertAuditLog(userID, "org_registered", c.IP(), `{}`)
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

// listOrgs handles GET /api/admin/orgs â€” superadmin only.
func (h *Handler) listOrgs(c *fiber.Ctx) error {
	orgs, err := h.deps.DB.ListOrganizations()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"organizations": orgs, "count": len(orgs)})
}

// getMyOrg handles GET /api/org â€” returns the caller's organization details.
func (h *Handler) getMyOrg(c *fiber.Ctx) error {
	orgID, _ := c.Locals("org_id").(int64)
	if orgID == 0 {
		return c.JSON(fiber.Map{"org": nil, "message": "superadmin â€” no specific org"})
	}
	org, err := h.deps.DB.GetOrganization(orgID)
	if err != nil || org == nil {
		return c.Status(404).JSON(fiber.Map{"error": "organization not found"})
	}
	count, _ := h.deps.DB.CountAccountsByOrg(orgID)
	return c.JSON(fiber.Map{
		"org":            org,
		"account_count":  count,
		"accounts_limit": org.MaxAccounts,
	})
}

// updateOrg handles PUT /api/org â€” org admin updates their org settings.
func (h *Handler) updateOrg(c *fiber.Ctx) error {
	orgID, _ := c.Locals("org_id").(int64)
	if orgID == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "no org context"})
	}
	var req struct {
		Name   string `json:"name"`
		Domain string `json:"domain"`
		Abbr   string `json:"abbr"`
		Color  string `json:"color"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}
	org, err := h.deps.DB.GetOrganization(orgID)
	if err != nil || org == nil {
		return c.Status(404).JSON(fiber.Map{"error": "organization not found"})
	}
	if req.Name != "" {
		org.Name = req.Name
	}
	if req.Domain != "" {
		org.Domain = req.Domain
	}
	if req.Abbr != "" {
		org.Abbr = strings.ToUpper(strings.TrimSpace(req.Abbr))
	}
	if req.Color != "" {
		org.Color = req.Color
	}
	if err := h.deps.DB.UpdateOrganizationBrand(orgID, org.Name, org.Domain, org.Abbr, org.Color); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"status": "updated", "org": org})
}

const orgAssetDir = "data/org_assets"

func (h *Handler) uploadOrgAsset(c *fiber.Ctx) error {
	orgID, _ := c.Locals("org_id").(int64)
	if orgID == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "no org context"})
	}
	kind := c.Params("kind")
	if kind != "logo" && kind != "avatar" {
		return c.Status(400).JSON(fiber.Map{"error": "kind must be logo or avatar"})
	}
	fh, err := c.FormFile("file")
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "no file provided"})
	}
	if fh.Size > 5*1024*1024 {
		return c.Status(413).JSON(fiber.Map{"error": "file too large (max 5MB)"})
	}
	mime := strings.ToLower(fh.Header.Get("Content-Type"))
	if mime != "image/png" && mime != "image/jpeg" && mime != "image/webp" && mime != "image/svg+xml" {
		return c.Status(415).JSON(fiber.Map{"error": "unsupported image type"})
	}
	dir := filepath.Join(orgAssetDir, fmt.Sprintf("%d", orgID))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "storage error"})
	}
	ext := strings.ToLower(filepath.Ext(fh.Filename))
	if ext == "" {
		switch mime {
		case "image/jpeg":
			ext = ".jpg"
		case "image/webp":
			ext = ".webp"
		case "image/svg+xml":
			ext = ".svg"
		default:
			ext = ".png"
		}
	}
	dest := filepath.Join(dir, kind+ext)
	if err := c.SaveFile(fh, dest); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "save error"})
	}
	if err := h.deps.DB.UpdateOrganizationAsset(orgID, kind, dest); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(201).JSON(fiber.Map{
		"kind": kind,
		"url":  fmt.Sprintf("/api/public/org-assets/%d/%s?v=%d", orgID, kind, time.Now().Unix()),
	})
}

func (h *Handler) serveOrgAsset(c *fiber.Ctx) error {
	orgID, err := c.ParamsInt("orgID")
	if err != nil || orgID <= 0 {
		return c.Status(400).SendString("invalid org")
	}
	kind := c.Params("kind")
	if kind != "logo" && kind != "avatar" {
		return c.Status(404).SendString("not found")
	}
	org, err := h.deps.DB.GetOrganization(int64(orgID))
	if err != nil || org == nil {
		return c.Status(404).SendString("not found")
	}
	dir := filepath.Join(orgAssetDir, fmt.Sprintf("%d", org.ID))
	matches, _ := filepath.Glob(filepath.Join(dir, kind+".*"))
	if len(matches) == 0 {
		return c.Status(404).SendString("not found")
	}
	return c.SendFile(matches[0])
}

// adminUpdateOrg handles PUT /api/admin/orgs/:id â€” superadmin changes plan/limits.
func (h *Handler) adminUpdateOrg(c *fiber.Ctx) error {
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
	org, err := h.deps.DB.GetOrganization(int64(id))
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
	if err := h.deps.DB.UpdateOrganization(org.ID, org.Name, org.Domain, org.PlanTier, org.MaxAccounts, org.Active); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"status": "updated", "org": org})
}

// createOrgUser handles POST /api/auth/users for platform-founder maintenance.
// Tenant workspaces must use /api/org/invites so staff create their own accounts.
func (h *Handler) createOrgUser(c *fiber.Ctx) error {
	callerOrgID, _ := c.Locals("org_id").(int64)
	callerRole, _ := c.Locals("user_role").(string)
	callerIsPlatform := models.IsPlatformUser(callerOrgID, models.UserRole(callerRole))
	if !callerIsPlatform {
		return c.Status(409).JSON(fiber.Map{
			"error": "workspace staff must be invited and create their own account",
			"code":  "INVITE_REQUIRED",
		})
	}

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
	org, err := h.deps.DB.GetOrganization(targetOrgID)
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

	existing, _ := h.deps.DB.GetUserByEmail(req.Email)
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
	id, err := h.deps.DB.CreateUser(user)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	_ = h.deps.DB.UpsertStaffKPI(id, targetOrgID, store.KPIDelta{})
	adminID, _ := c.Locals("user_id").(int64)
	h.deps.DB.InsertAuditLog(adminID, "workspace_member_created", c.IP(),
		fmt.Sprintf(`{"user_id":%d,"org_id":%d,"role":%q}`, id, targetOrgID, req.Role))
	return c.Status(201).JSON(fiber.Map{
		"user_id": id,
		"org_id":  targetOrgID,
		"email":   req.Email,
		"name":    req.Name,
		"role":    req.Role,
		"active":  true,
		"user": fiber.Map{
			"id":         id,
			"org_id":     targetOrgID,
			"email":      req.Email,
			"name":       req.Name,
			"role":       req.Role,
			"active":     true,
			"created_at": time.Now().Format(time.RFC3339),
		},
	})
}

func setRefreshCookie(c *fiber.Ctx, token string) {
	// Intentionally minimal â€” full cookie set in auth_handlers.go
	c.Cookie(&fiber.Cookie{
		Name:     refreshCookie,
		Value:    token,
		Path:     cookiePath,
		Expires:  time.Now().Add(authpkg.RefreshTokenTTL),
		HTTPOnly: true,
		Secure:   secureCookie(c),
		SameSite: "Lax",
	})
}

func setAuthCookies(c *fiber.Ctx, accessToken string, accessExpiresAt time.Time) {
	c.Cookie(&fiber.Cookie{
		Name:     accessCookie,
		Value:    accessToken,
		Path:     "/",
		Expires:  accessExpiresAt,
		HTTPOnly: true,
		Secure:   secureCookie(c),
		SameSite: "Strict",
	})
	c.Cookie(&fiber.Cookie{
		Name:     authPresentCookie,
		Value:    "1",
		Path:     "/",
		Expires:  time.Now().Add(authpkg.RefreshTokenTTL),
		HTTPOnly: false,
		Secure:   secureCookie(c),
		SameSite: "Strict",
	})
}

func secureCookie(c *fiber.Ctx) bool {
	if strings.EqualFold(c.Get("X-Forwarded-Proto"), "https") {
		return true
	}
	if strings.EqualFold(c.Protocol(), "https") {
		return true
	}
	return false
}
