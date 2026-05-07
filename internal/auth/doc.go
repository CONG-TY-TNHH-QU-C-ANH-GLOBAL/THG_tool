// Package auth is the auth/security primitives library: JWT signing and
// validation, AES-256-GCM encryption for at-rest secrets, password hashing
// (bcrypt + complexity rules), refresh-token issuance, and the Fiber
// RequireAuth middleware that injects user_id / user_email / user_role
// into request locals.
//
// Layer boundary: this package owns crypto and middleware ONLY. HTTP routes
// (login, signup, Google SSO, invite accept) live in internal/server/auth.
// Anything that needs to verify a JWT, encrypt a value, or guard a route
// imports from here. Anything that wires those primitives into a URL imports
// internal/server/auth.
package auth
