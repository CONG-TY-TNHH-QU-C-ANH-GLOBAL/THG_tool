package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/gofiber/fiber/v2"
	authpkg "github.com/thg/scraper/internal/auth"
	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store"
)

const (
	googleAuthEndpoint     = "https://accounts.google.com/o/oauth2/v2/auth"
	googleTokenEndpoint    = "https://oauth2.googleapis.com/token"
	googleUserInfoEndpoint = "https://www.googleapis.com/oauth2/v2/userinfo"
	oauthStateCookie       = "g_oauth_state"
)

// googleLoginRedirect redirects the browser to the Google OAuth consent screen.
// GET /api/auth/google
func (s *Server) googleLoginRedirect(c *fiber.Ctx) error {
	if s.cfg.GoogleClientID == "" {
		return c.Status(501).JSON(fiber.Map{"error": "Google OAuth not configured — set GOOGLE_CLIENT_ID"})
	}

	state, err := randomHex(16)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "state generation failed"})
	}

	c.Cookie(&fiber.Cookie{
		Name:     oauthStateCookie,
		Value:    state,
		MaxAge:   300,
		HTTPOnly: true,
		Secure:   true,
		SameSite: "Lax",
	})

	q := url.Values{
		"client_id":     {s.cfg.GoogleClientID},
		"redirect_uri":  {s.cfg.GoogleRedirectURI},
		"response_type": {"code"},
		"scope":         {"openid email profile"},
		"state":         {state},
		"access_type":   {"online"},
		"prompt":        {"select_account"},
	}
	return c.Redirect(googleAuthEndpoint+"?"+q.Encode(), fiber.StatusTemporaryRedirect)
}

// googleCallback handles the OAuth consent redirect.
// GET /api/auth/google/callback
func (s *Server) googleCallback(c *fiber.Ctx) error {
	// CSRF guard
	state := c.Query("state")
	if state == "" || state != c.Cookies(oauthStateCookie) {
		return redirectWithError(c, "invalid OAuth state")
	}
	c.Cookie(&fiber.Cookie{Name: oauthStateCookie, MaxAge: -1}) // clear

	code := c.Query("code")
	if code == "" {
		return redirectWithError(c, c.Query("error", "access denied"))
	}

	// Exchange authorization code → access token
	accessToken, err := exchangeGoogleCode(code, s.cfg.GoogleClientID, s.cfg.GoogleClientSecret, s.cfg.GoogleRedirectURI)
	if err != nil {
		log.Printf("[GoogleAuth] Token exchange failed: %v", err)
		return redirectWithError(c, "token exchange failed")
	}

	// Fetch user info from Google
	info, err := fetchGoogleUserInfo(accessToken)
	if err != nil {
		log.Printf("[GoogleAuth] User info failed: %v", err)
		return redirectWithError(c, "failed to get user info")
	}

	// Find or create user — Google Sign-In creates new accounts on first login.
	isNew := false
	user, err := s.db.GetUserByEmail(info.Email)
	if err != nil || user == nil {
		orgID := int64(0)
		role := models.RoleAdmin
		var provisionedClaim *store.ProvisionedOrgClaim
		if claim, claimErr := s.db.FindProvisionedOrgByEmail(info.Email); claimErr != nil {
			log.Printf("[GoogleAuth] Provisioned org lookup failed: %v", claimErr)
			return redirectWithError(c, "workspace assignment failed")
		} else if claim != nil {
			orgID = claim.OrgID
			role = claim.Role
			provisionedClaim = claim
		}
		// Auto-create user with org_id=0; they'll be sent to onboarding.
		newID, createErr := s.db.CreateUser(&models.User{
			OrgID:        orgID,
			Email:        info.Email,
			Name:         info.Name,
			PasswordHash: "", // no password — Google-only account
			Role:         role,
		})
		if createErr != nil {
			log.Printf("[GoogleAuth] Auto-create user failed: %v", createErr)
			return redirectWithError(c, "failed to create account")
		}
		if claimErr := s.completeProvisionedClaim(newID, provisionedClaim, c.IP()); claimErr != nil {
			log.Printf("[GoogleAuth] Complete provisioned claim failed: %v", claimErr)
			return redirectWithError(c, "workspace assignment failed")
		}
		user, err = s.db.GetUserByID(newID)
		if err != nil || user == nil {
			return redirectWithError(c, "account lookup failed")
		}
		isNew = true
		log.Printf("[GoogleAuth] Auto-created user: %s (id=%d)", info.Email, newID)
	} else if _, err := s.attachProvisionedOrgIfNeeded(user, c.IP()); err != nil {
		log.Printf("[GoogleAuth] Provisioned org claim failed for %s: %v", user.Email, err)
		return redirectWithError(c, "workspace assignment failed")
	}

	// Issue JWT pair
	jwtToken, err := authpkg.GenerateAccessToken(user.ID, user.OrgID, user.Email, string(user.Role), s.cfg.JWTSecret)
	if err != nil {
		return redirectWithError(c, "token generation failed")
	}
	refreshToken, err := authpkg.GenerateRefreshToken()
	if err != nil {
		return redirectWithError(c, "refresh token failed")
	}
	expiresAt := time.Now().Add(authpkg.RefreshTokenTTL)
	if err := s.db.SaveRefreshToken(user.ID, refreshToken, expiresAt); err != nil {
		return redirectWithError(c, "session save failed")
	}

	// Set refresh token cookie
	c.Cookie(&fiber.Cookie{
		Name:     refreshCookie,
		Value:    refreshToken,
		Path:     cookiePath,
		Expires:  expiresAt,
		HTTPOnly: true,
		Secure:   true,
		SameSite: "Strict",
	})

	// Phase 4b: also set the HttpOnly access-token cookie so the SPA
	// has authenticated session immediately on Google OAuth callback.
	setAuthCookies(c, jwtToken, time.Now().Add(authpkg.AccessTokenTTL))

	// Pass the JWT to the SPA's bootstrap exchange via a short-lived
	// HttpOnly cookie. The /api/auth/google/token handler reads the
	// cookie server-side, so the SPA never sees the JWT — earlier
	// versions set HttpOnly:false because the SPA was supposed to
	// read the value, but Phase 4b hands the SPA a real session via
	// setAuthCookies() above; g_at is now an internal handoff and
	// must stay invisible to JS to keep the "JWT never reaches
	// document.cookie" guarantee.
	c.Cookie(&fiber.Cookie{
		Name:     "g_at",
		Value:    jwtToken,
		Path:     cookiePath, // /api/auth — only sent to the exchange endpoint
		MaxAge:   60,         // 60s — server reads and clears it immediately
		HTTPOnly: true,
		Secure:   secureCookie(c),
		SameSite: "Strict",
	})

	action := "google_login"
	if isNew {
		action = "google_signup"
	}
	s.db.InsertAuditLog(user.ID, action, c.IP(), fmt.Sprintf(`{"email":%q}`, user.Email))
	log.Printf("[GoogleAuth] %s: %s (role=%s)", action, user.Email, user.Role)

	redirectURL := "/?google_auth=1"
	if isNew {
		redirectURL = "/?google_auth=1&new=1"
	}
	return c.Redirect(redirectURL, fiber.StatusTemporaryRedirect)
}

// ------- helpers -------

type googleUserInfo struct {
	Email   string `json:"email"`
	Name    string `json:"name"`
	Picture string `json:"picture"`
}

func exchangeGoogleCode(code, clientID, clientSecret, redirectURI string) (string, error) {
	resp, err := http.PostForm(googleTokenEndpoint, url.Values{
		"code":          {code},
		"client_id":     {clientID},
		"client_secret": {clientSecret},
		"redirect_uri":  {redirectURI},
		"grant_type":    {"authorization_code"},
	})
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var result struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("parse token response: %w", err)
	}
	if result.Error != "" {
		return "", fmt.Errorf("google: %s", result.Error)
	}
	return result.AccessToken, nil
}

func fetchGoogleUserInfo(accessToken string) (*googleUserInfo, error) {
	req, _ := http.NewRequest(http.MethodGet, googleUserInfoEndpoint, nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var info googleUserInfo
	if err := json.Unmarshal(body, &info); err != nil {
		return nil, err
	}
	if info.Email == "" {
		return nil, fmt.Errorf("empty email from Google")
	}
	return &info, nil
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func redirectWithError(c *fiber.Ctx, msg string) error {
	return c.Redirect("/?auth_error="+url.QueryEscape(msg), fiber.StatusTemporaryRedirect)
}

// googleStatus returns whether Google OAuth is configured.
// GET /api/auth/google/status  (used by frontend to decide whether to show the button)
func (s *Server) googleStatus(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"enabled": s.cfg.GoogleClientID != "",
	})
}

// googleToken lets the SPA exchange the short-lived g_at cookie for a proper auth response.
// POST /api/auth/google/token  (called immediately after redirect back from Google)
func (s *Server) googleToken(c *fiber.Ctx) error {
	token := c.Cookies("g_at")
	if token == "" {
		return c.Status(400).JSON(fiber.Map{"error": "no pending Google auth"})
	}
	// Clear the cookie — match Path/HTTPOnly/SameSite so the browser
	// actually overwrites the entry. A bare clear without these
	// attributes would leave the original cookie alive.
	c.Cookie(&fiber.Cookie{
		Name:     "g_at",
		Value:    "",
		Path:     cookiePath,
		MaxAge:   -1,
		HTTPOnly: true,
		Secure:   secureCookie(c),
		SameSite: "Strict",
	})

	// Validate the token and return user info
	claims, err := authpkg.ValidateAccessToken(token, s.cfg.JWTSecret)
	if err != nil {
		return c.Status(401).JSON(fiber.Map{"error": "invalid token"})
	}

	user, err := s.db.GetUserByEmail(claims.Email)
	if err != nil || user == nil {
		return c.Status(401).JSON(fiber.Map{"error": "user not found"})
	}
	if claimed, err := s.attachProvisionedOrgIfNeeded(user, c.IP()); err != nil {
		log.Printf("[GoogleAuth] Provisioned org claim failed during token exchange for %s: %v", user.Email, err)
		return c.Status(500).JSON(fiber.Map{"error": "workspace assignment failed"})
	} else if claimed {
		token, err = authpkg.GenerateAccessToken(user.ID, user.OrgID, user.Email, string(user.Role), s.cfg.JWTSecret)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "token generation failed"})
		}
	}

	// Phase 4b: SPA-friendly auth cookies for the OAuth token-exchange path.
	setAuthCookies(c, token, time.Now().Add(authpkg.AccessTokenTTL))

	return c.JSON(fiber.Map{
		"access_token":     token,
		"expires_in":       int(authpkg.AccessTokenTTL.Seconds()),
		"needs_onboarding": user.OrgID == 0 && !models.IsPlatformRole(user.Role),
		"user": fiber.Map{
			"id":     user.ID,
			"org_id": user.OrgID,
			"email":  user.Email,
			"name":   user.Name,
			"role":   user.Role,
		},
	})
}
