package server

import (
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/thg/scraper/internal/auth"
	"github.com/thg/scraper/internal/models"
)

const (
	refreshCookie = "refresh_token"
	cookiePath    = "/api/auth"
)

// login handles POST /api/auth/login — email + password → access + refresh tokens.
func (s *Server) login(c *fiber.Ctx) error {
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

	user, err := s.db.GetUserByEmail(req.Email)
	if err != nil {
		log.Printf("[Auth] DB error on login for %s: %v", req.Email, err)
		return c.Status(500).JSON(fiber.Map{"error": "internal error"})
	}
	// Constant-time: always respond with same error to prevent user enumeration
	if user == nil {
		s.db.InsertAuditLog(0, "login_failed", ip, fmt.Sprintf(`{"email":%q}`, req.Email))
		return c.Status(401).JSON(fiber.Map{"error": "invalid credentials"})
	}

	// Check account lockout
	if user.FailedLogins >= auth.MaxFailedAttempts && time.Now().Before(user.LockedUntil) {
		s.db.InsertAuditLog(user.ID, "login_blocked", ip, `{}`)
		return c.Status(423).JSON(fiber.Map{
			"error":        "account locked — too many failed attempts",
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
		s.db.IncrementFailedLogins(user.ID, newCount, lockUntil)
		s.db.InsertAuditLog(user.ID, "login_failed", ip, `{}`)
		return c.Status(401).JSON(fiber.Map{"error": "invalid credentials"})
	}

	// Successful login
	s.db.ResetFailedLogins(user.ID)

	accessToken, err := auth.GenerateAccessToken(user.ID, user.Email, string(user.Role), s.cfg.JWTSecret)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "token generation failed"})
	}

	refreshToken, err := auth.GenerateRefreshToken()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "token generation failed"})
	}

	expiresAt := time.Now().Add(auth.RefreshTokenTTL)
	if err := s.db.SaveRefreshToken(user.ID, refreshToken, expiresAt); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "session error"})
	}

	// Refresh token: httpOnly, Secure, SameSite=Strict
	c.Cookie(&fiber.Cookie{
		Name:     refreshCookie,
		Value:    refreshToken,
		Path:     cookiePath,
		Expires:  expiresAt,
		HTTPOnly: true,
		Secure:   false, // set to true only in production with HTTPS
		SameSite: "Lax",
	})

	s.db.InsertAuditLog(user.ID, "login_success", ip, `{}`)
	log.Printf("[Auth] Login: %s (role=%s) from %s", user.Email, user.Role, ip)

	return c.JSON(fiber.Map{
		"access_token": accessToken,
		"expires_in":   int(auth.AccessTokenTTL.Seconds()),
		"user": fiber.Map{
			"id":    user.ID,
			"email": user.Email,
			"name":  user.Name,
			"role":  user.Role,
		},
	})
}

// refresh handles POST /api/auth/refresh — rotates the refresh token, issues new access token.
func (s *Server) refresh(c *fiber.Ctx) error {
	token := c.Cookies(refreshCookie)
	if token == "" {
		return c.Status(401).JSON(fiber.Map{"error": "no refresh token"})
	}

	userID, err := s.db.ValidateRefreshToken(token)
	if err != nil {
		return c.Status(401).JSON(fiber.Map{"error": "invalid or expired refresh token"})
	}

	user, err := s.db.GetUserByID(userID)
	if err != nil || user == nil {
		return c.Status(401).JSON(fiber.Map{"error": "user not found"})
	}

	// Rotate: delete old token, issue new one
	s.db.DeleteRefreshToken(token)

	newRefresh, err := auth.GenerateRefreshToken()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "token generation failed"})
	}
	expiresAt := time.Now().Add(auth.RefreshTokenTTL)
	if err := s.db.SaveRefreshToken(user.ID, newRefresh, expiresAt); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "session error"})
	}

	accessToken, err := auth.GenerateAccessToken(user.ID, user.Email, string(user.Role), s.cfg.JWTSecret)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "token generation failed"})
	}

	c.Cookie(&fiber.Cookie{
		Name:     refreshCookie,
		Value:    newRefresh,
		Path:     cookiePath,
		Expires:  expiresAt,
		HTTPOnly: true,
		Secure:   false, // set to true only in production with HTTPS
		SameSite: "Lax",
	})

	return c.JSON(fiber.Map{
		"access_token": accessToken,
		"expires_in":   int(auth.AccessTokenTTL.Seconds()),
	})
}

// logout handles POST /api/auth/logout — deletes the refresh token and clears the cookie.
func (s *Server) logout(c *fiber.Ctx) error {
	token := c.Cookies(refreshCookie)
	if token != "" {
		s.db.DeleteRefreshToken(token)
	}
	c.Cookie(&fiber.Cookie{
		Name:     refreshCookie,
		Value:    "",
		Path:     cookiePath,
		Expires:  time.Unix(0, 0),
		HTTPOnly: true,
		Secure:   false, // set to true only in production with HTTPS
		SameSite: "Lax",
	})
	userID, _ := c.Locals("user_id").(int64)
	s.db.InsertAuditLog(userID, "logout", c.IP(), `{}`)
	return c.JSON(fiber.Map{"status": "logged out"})
}

// me handles GET /api/auth/me — returns the current user's profile.
func (s *Server) me(c *fiber.Ctx) error {
	userID, _ := c.Locals("user_id").(int64)
	user, err := s.db.GetUserByID(userID)
	if err != nil || user == nil {
		return c.Status(404).JSON(fiber.Map{"error": "user not found"})
	}
	return c.JSON(fiber.Map{
		"id":         user.ID,
		"email":      user.Email,
		"name":       user.Name,
		"role":       user.Role,
		"created_at": user.CreatedAt,
	})
}

// updateOwnProfile handles PUT /api/auth/me — user updates their own name.
func (s *Server) updateOwnProfile(c *fiber.Ctx) error {
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
	user, err := s.db.GetUserByID(userID)
	if err != nil || user == nil {
		return c.Status(404).JSON(fiber.Map{"error": "user not found"})
	}
	if err := s.db.UpdateUser(userID, req.Name, user.Role, user.Active); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "update failed"})
	}
	return c.JSON(fiber.Map{"status": "updated", "name": req.Name})
}

// changeOwnPassword handles PUT /api/auth/me/password — user changes their own password.
func (s *Server) changeOwnPassword(c *fiber.Ctx) error {
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
	user, err := s.db.GetUserByID(userID)
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
	if err := s.db.UpdateUserPassword(userID, hash); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "update failed"})
	}
	s.db.InsertAuditLog(userID, "password_changed", c.IP(), `{}`)
	return c.JSON(fiber.Map{"status": "password updated"})
}

// adminUpdateUser handles PUT /api/auth/users/:id — admin edits a user account.
func (s *Server) adminUpdateUser(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid user id"})
	}
	var req struct {
		Name        string `json:"name"`
		Role        string `json:"role"`
		Active      *bool  `json:"active"`
		NewPassword string `json:"new_password"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}
	user, err := s.db.GetUserByID(id)
	if err != nil || user == nil {
		return c.Status(404).JSON(fiber.Map{"error": "user not found"})
	}
	name := user.Name
	if req.Name != "" {
		name = req.Name
	}
	role := user.Role
	if req.Role == "admin" || req.Role == "sales" {
		role = models.UserRole(req.Role)
	}
	active := user.Active
	if req.Active != nil {
		active = *req.Active
	}
	if err := s.db.UpdateUser(id, name, role, active); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "update failed"})
	}
	if req.NewPassword != "" {
		if err := auth.ValidatePasswordStrength(req.NewPassword); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		hash, err := auth.HashPassword(req.NewPassword)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "password hashing failed"})
		}
		s.db.UpdateUserPassword(id, hash)
		s.db.DeleteUserRefreshTokens(id)
	}
	adminID, _ := c.Locals("user_id").(int64)
	s.db.InsertAuditLog(adminID, "user_updated", c.IP(), fmt.Sprintf(`{"target_id":%d}`, id))
	return c.JSON(fiber.Map{"status": "updated"})
}

// adminDeleteUser handles DELETE /api/auth/users/:id — admin removes a user.
func (s *Server) adminDeleteUser(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid user id"})
	}
	adminID, _ := c.Locals("user_id").(int64)
	if id == adminID {
		return c.Status(400).JSON(fiber.Map{"error": "cannot delete your own account"})
	}
	if err := s.db.DeleteUser(id); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "delete failed"})
	}
	s.db.InsertAuditLog(adminID, "user_deleted", c.IP(), fmt.Sprintf(`{"deleted_id":%d}`, id))
	return c.JSON(fiber.Map{"status": "deleted"})
}

// createUser handles POST /api/auth/users — admin creates a new user account.
func (s *Server) createUser(c *fiber.Ctx) error {
	var req struct {
		Email    string `json:"email"`
		Name     string `json:"name"`
		Password string `json:"password"`
		Role     string `json:"role"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}
	if req.Email == "" || req.Name == "" || req.Password == "" {
		return c.Status(400).JSON(fiber.Map{"error": "email, name, and password required"})
	}
	if req.Role != "admin" && req.Role != "sales" {
		req.Role = "sales"
	}
	if err := auth.ValidatePasswordStrength(req.Password); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "password hashing failed"})
	}
	user := &models.User{
		Email:        req.Email,
		Name:         req.Name,
		PasswordHash: hash,
		Role:         models.UserRole(req.Role),
	}
	id, err := s.db.CreateUser(user)
	if err != nil {
		return c.Status(409).JSON(fiber.Map{"error": "email already exists or DB error"})
	}

	adminID, _ := c.Locals("user_id").(int64)
	s.db.InsertAuditLog(adminID, "user_created", c.IP(),
		fmt.Sprintf(`{"new_user_email":%q,"role":%q}`, req.Email, req.Role))
	log.Printf("[Auth] User created: %s (role=%s) by admin %d", req.Email, req.Role, adminID)

	return c.Status(201).JSON(fiber.Map{"user_id": id, "email": req.Email, "role": req.Role})
}

// listUsers handles GET /api/auth/users — admin lists all user accounts.
func (s *Server) listUsers(c *fiber.Ctx) error {
	users, err := s.db.ListUsers()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"users": users, "count": len(users)})
}

// getAuditLogs handles GET /api/auth/audit — admin views the security audit trail.
func (s *Server) getAuditLogs(c *fiber.Ctx) error {
	limit := 100
	logs, err := s.db.GetAuditLogs(limit)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"logs": logs, "count": len(logs)})
}
