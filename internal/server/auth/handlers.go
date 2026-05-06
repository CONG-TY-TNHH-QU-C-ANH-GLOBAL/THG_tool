package auth

import (
	"fmt"
	"log"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/thg/scraper/internal/auth"
)

const (
	refreshCookie = "refresh_token"
	cookiePath    = "/api/auth"

	// accessCookie holds the short-lived JWT access token. Phase 4b moves
	// the access token out of the localStorage-backed Authorization header
	// (XSS-readable) into an HttpOnly cookie. The middleware
	// (auth.RequireAuth Ã¢â€ â€™ extractToken) already prefers Authorization but
	// falls through to this cookie, so legacy clients keep working until
	// they migrate. Path "/" so the cookie is sent on /api and /ws/* Ã¢â‚¬â€
	// WebSocket upgrades carry cookies, which is also how the VNC /
	// screen proxies authenticate after Phase 4c.
	accessCookie = "access_token"

	// authPresentCookie is a non-HttpOnly companion flag the SPA reads to
	// know "I am logged in" without ever seeing the JWT. It carries no
	// secret, only a presence bit; the real auth is the HttpOnly cookie
	// above. The SPA expires its in-memory user when this flag flips off.
	authPresentCookie = "autoflow_session"
)

// setAuthCookies writes the access-token + presence cookies after a
// successful login or refresh.
//
// The access-token cookie expires with the access-token TTL (short,
// because the JWT is a bearer secret). The presence cookie expires
// with the refresh-token TTL (long) Ã¢â‚¬â€ it carries no secret, only a
// "session might be valid, try refresh" signal. Aligning the presence
// cookie to access TTL caused early logout: after the access cookie
// expired, the SPA on a fresh page load saw no presence cookie and
// short-circuited to login even though the refresh cookie was still
// alive. Now restoreSession() runs, /auth/me returns 401 because the
// access cookie is gone, apiFetch silently refreshes via the refresh
// cookie, retries /auth/me, and the user stays signed in.
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
		Expires:  time.Now().Add(auth.RefreshTokenTTL),
		HTTPOnly: false, // SPA reads this to detect logged-in state
		Secure:   secureCookie(c),
		SameSite: "Strict",
	})
}

// clearAuthCookies expires the access-token + presence cookies on
// logout. Refresh cookie is cleared separately because its Path is
// scoped to /api/auth.
func clearAuthCookies(c *fiber.Ctx) {
	c.Cookie(&fiber.Cookie{
		Name:     accessCookie,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		HTTPOnly: true,
		Secure:   secureCookie(c),
		SameSite: "Strict",
	})
	c.Cookie(&fiber.Cookie{
		Name:     authPresentCookie,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		HTTPOnly: false,
		Secure:   secureCookie(c),
		SameSite: "Strict",
	})
}

// login handles POST /api/auth/login Ã¢â‚¬â€ email + password Ã¢â€ â€™ access + refresh tokens.
func setRefreshCookie(c *fiber.Ctx, token string) {
	c.Cookie(&fiber.Cookie{
		Name:     refreshCookie,
		Value:    token,
		Path:     cookiePath,
		Expires:  time.Now().Add(auth.RefreshTokenTTL),
		HTTPOnly: true,
		Secure:   secureCookie(c),
		SameSite: "Lax",
	})
}

func (h *Handler) login(c *fiber.Ctx) error {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}
	if req.Email == "" || req.Password == "" {
		return c.Status(400).JSON(fiber.Map{"error": "email and password required"})
	}

	ip := c.IP()

	user, err := h.deps.DB.GetUserByEmail(req.Email)
	if err != nil {
		log.Printf("[Auth] DB error on login for %s: %v", req.Email, err)
		return c.Status(500).JSON(fiber.Map{"error": "internal error"})
	}
	// Constant-time: always respond with same error to prevent user enumeration
	if user == nil {
		h.deps.DB.InsertAuditLog(0, "login_failed", ip, fmt.Sprintf(`{"email":%q}`, req.Email))
		return c.Status(401).JSON(fiber.Map{"error": "invalid credentials"})
	}

	// Check account lockout
	if user.FailedLogins >= auth.MaxFailedAttempts && time.Now().Before(user.LockedUntil) {
		h.deps.DB.InsertAuditLog(user.ID, "login_blocked", ip, `{}`)
		return c.Status(423).JSON(fiber.Map{
			"error":        "account locked Ã¢â‚¬â€ too many failed attempts",
			"locked_until": user.LockedUntil.Format(time.RFC3339),
		})
	}

	if !auth.CheckPassword(user.PasswordHash, req.Password) {
		newCount := user.FailedLogins + 1
		var lockUntil time.Time
		if newCount >= auth.MaxFailedAttempts {
			lockUntil = time.Now().Add(auth.LockoutDuration)
			log.Printf("[Auth] Account locked: %s (failed attempts: %d)", req.Email, newCount)
		}
		h.deps.DB.IncrementFailedLogins(user.ID, newCount, lockUntil)
		h.deps.DB.InsertAuditLog(user.ID, "login_failed", ip, `{}`)
		return c.Status(401).JSON(fiber.Map{"error": "invalid credentials"})
	}

	// Successful login
	h.deps.DB.ResetFailedLogins(user.ID)
	if _, err := h.attachProvisionedOrgIfNeeded(user, ip); err != nil {
		log.Printf("[Auth] Provisioned org claim failed for %s: %v", user.Email, err)
		return c.Status(500).JSON(fiber.Map{"error": "workspace assignment failed"})
	}

	accessToken, err := auth.GenerateAccessToken(user.ID, user.OrgID, user.Email, string(user.Role), h.deps.JWTSecret)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "token generation failed"})
	}

	refreshToken, err := auth.GenerateRefreshToken()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "token generation failed"})
	}

	expiresAt := time.Now().Add(auth.RefreshTokenTTL)
	if err := h.deps.DB.SaveRefreshToken(user.ID, refreshToken, expiresAt); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "session error"})
	}

	// Refresh token: httpOnly, Secure, SameSite=Strict
	c.Cookie(&fiber.Cookie{
		Name:     refreshCookie,
		Value:    refreshToken,
		Path:     cookiePath,
		Expires:  expiresAt,
		HTTPOnly: true,
		Secure:   secureCookie(c),
		SameSite: "Strict",
	})

	// Phase 4b: also issue the access token as an HttpOnly cookie so the
	// SPA never has to read the JWT in JavaScript. The body still carries
	// access_token for backward compatibility with any existing client
	// that sets Authorization: Bearer manually Ã¢â‚¬â€ once all clients migrate
	// the response field can be removed.
	setAuthCookies(c, accessToken, time.Now().Add(auth.AccessTokenTTL))

	h.deps.DB.InsertAuditLog(user.ID, "login_success", ip, `{}`)
	log.Printf("[Auth] Login: %s (role=%s) from %s", user.Email, user.Role, ip)

	return c.JSON(fiber.Map{
		"access_token": accessToken,
		"expires_in":   int(auth.AccessTokenTTL.Seconds()),
		"user": fiber.Map{
			"id":     user.ID,
			"org_id": user.OrgID,
			"email":  user.Email,
			"name":   user.Name,
			"role":   user.Role,
		},
	})
}

// refresh handles POST /api/auth/refresh Ã¢â‚¬â€ rotates the refresh token, issues new access token.
// Token is read from the httpOnly cookie first, then falls back to X-Refresh-Token header
// (for clients behind reverse proxies that strip Cookie headers).
func (h *Handler) refresh(c *fiber.Ctx) error {
	token := c.Cookies(refreshCookie)
	if token == "" {
		token = c.Get("X-Refresh-Token")
	}
	if token == "" {
		return c.Status(401).JSON(fiber.Map{"error": "no refresh token"})
	}

	userID, err := h.deps.DB.ValidateRefreshToken(token)
	if err != nil {
		return c.Status(401).JSON(fiber.Map{"error": "invalid or expired refresh token"})
	}

	user, err := h.deps.DB.GetUserByID(userID)
	if err != nil || user == nil {
		return c.Status(401).JSON(fiber.Map{"error": "user not found"})
	}
	if _, err := h.attachProvisionedOrgIfNeeded(user, c.IP()); err != nil {
		log.Printf("[Auth] Provisioned org claim failed during refresh for user %d: %v", user.ID, err)
		return c.Status(500).JSON(fiber.Map{"error": "workspace assignment failed"})
	}

	// Rotate: delete old token, issue new one
	h.deps.DB.DeleteRefreshToken(token)

	newRefresh, err := auth.GenerateRefreshToken()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "token generation failed"})
	}
	expiresAt := time.Now().Add(auth.RefreshTokenTTL)
	if err := h.deps.DB.SaveRefreshToken(user.ID, newRefresh, expiresAt); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "session error"})
	}

	accessToken, err := auth.GenerateAccessToken(user.ID, user.OrgID, user.Email, string(user.Role), h.deps.JWTSecret)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "token generation failed"})
	}

	c.Cookie(&fiber.Cookie{
		Name:     refreshCookie,
		Value:    newRefresh,
		Path:     cookiePath,
		Expires:  expiresAt,
		HTTPOnly: true,
		Secure:   secureCookie(c),
		SameSite: "Strict",
	})

	// Phase 4b: refresh the access-token cookie alongside the rotated
	// refresh-token cookie so the SPA never needs to handle the JWT.
	setAuthCookies(c, accessToken, time.Now().Add(auth.AccessTokenTTL))

	return c.JSON(fiber.Map{
		"access_token": accessToken,
		"expires_in":   int(auth.AccessTokenTTL.Seconds()),
	})
}

// logout handles POST /api/auth/logout Ã¢â‚¬â€ deletes the refresh token and clears the cookie.
func (h *Handler) logout(c *fiber.Ctx) error {
	token := c.Cookies(refreshCookie)
	if token == "" {
		token = c.Get("X-Refresh-Token")
	}
	if token != "" {
		h.deps.DB.DeleteRefreshToken(token)
	}
	c.Cookie(&fiber.Cookie{
		Name:     refreshCookie,
		Value:    "",
		Path:     cookiePath,
		Expires:  time.Unix(0, 0),
		HTTPOnly: true,
		Secure:   secureCookie(c),
		SameSite: "Strict",
	})
	clearAuthCookies(c)
	userID, _ := c.Locals("user_id").(int64)
	h.deps.DB.InsertAuditLog(userID, "logout", c.IP(), `{}`)
	return c.JSON(fiber.Map{"status": "logged out"})
}

// me handles GET /api/auth/me Ã¢â‚¬â€ returns the current user's profile.
func (h *Handler) me(c *fiber.Ctx) error {
	userID, _ := c.Locals("user_id").(int64)
	user, err := h.deps.DB.GetUserByID(userID)
	if err != nil || user == nil {
		return c.Status(404).JSON(fiber.Map{"error": "user not found"})
	}
	if _, err := h.attachProvisionedOrgIfNeeded(user, c.IP()); err != nil {
		log.Printf("[Auth] Provisioned org claim failed during /me for user %d: %v", user.ID, err)
		return c.Status(500).JSON(fiber.Map{"error": "workspace assignment failed"})
	}
	return c.JSON(fiber.Map{
		"id":         user.ID,
		"org_id":     user.OrgID,
		"email":      user.Email,
		"name":       user.Name,
		"role":       user.Role,
		"created_at": user.CreatedAt,
	})
}

// updateOwnProfile handles PUT /api/auth/me Ã¢â‚¬â€ user updates their own name.
func (h *Handler) updateOwnProfile(c *fiber.Ctx) error {
	var req struct {
		Name string `json:"name"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}
	if req.Name == "" {
		return c.Status(400).JSON(fiber.Map{"error": "name is required"})
	}
	userID, _ := c.Locals("user_id").(int64)
	user, err := h.deps.DB.GetUserByID(userID)
	if err != nil || user == nil {
		return c.Status(404).JSON(fiber.Map{"error": "user not found"})
	}
	if err := h.deps.DB.UpdateUser(userID, req.Name, user.Role, user.Active); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "update failed"})
	}
	return c.JSON(fiber.Map{"status": "updated", "name": req.Name})
}

// changeOwnPassword handles PUT /api/auth/me/password Ã¢â‚¬â€ user changes their own password.
func (h *Handler) changeOwnPassword(c *fiber.Ctx) error {
	var req struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
		ConfirmPassword string `json:"confirm_password"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}
	if req.NewPassword != req.ConfirmPassword {
		return c.Status(400).JSON(fiber.Map{"error": "passwords do not match"})
	}
	userID, _ := c.Locals("user_id").(int64)
	user, err := h.deps.DB.GetUserByID(userID)
	if err != nil || user == nil {
		return c.Status(404).JSON(fiber.Map{"error": "user not found"})
	}
	if !auth.CheckPassword(user.PasswordHash, req.CurrentPassword) {
		return c.Status(401).JSON(fiber.Map{"error": "current password is incorrect"})
	}
	if err := auth.ValidatePasswordStrength(req.NewPassword); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	hash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "password hashing failed"})
	}
	if err := h.deps.DB.UpdateUserPassword(userID, hash); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "update failed"})
	}
	h.deps.DB.InsertAuditLog(userID, "password_changed", c.IP(), `{}`)
	return c.JSON(fiber.Map{"status": "password updated"})
}

// adminUpdateUser handles PUT /api/auth/users/:id Ã¢â‚¬â€ admin edits a user account.
