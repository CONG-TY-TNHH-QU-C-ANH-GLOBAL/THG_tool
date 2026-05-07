// Package auth (server/auth) is the HTTP routes layer for authentication and
// onboarding: POST /auth/login, /auth/signup, /auth/refresh, /auth/logout,
// /auth/google/*, /auth/invite/accept, plus org bootstrap and password reset
// flows. It depends on internal/auth (the crypto/middleware library) and
// internal/mailer for invite emails.
//
// Layer boundary: routes and request shaping ONLY. JWT signing, password
// hashing, AES encryption, and the RequireAuth middleware live in
// internal/auth. If you find yourself reaching for jwt.Sign or bcrypt here,
// move it down into internal/auth and call from this package instead.
package auth
