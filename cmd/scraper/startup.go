package main

import (
	"context"
	"log"
	"os"

	authpkg "github.com/thg/scraper/internal/auth"
	"github.com/thg/scraper/internal/browser"
	"github.com/thg/scraper/internal/config"
	"github.com/thg/scraper/internal/store"
	"github.com/thg/scraper/internal/workspace"
)

// This file holds closure-free startup bootstrap helpers extracted from
// main() (2026-07-01) — pure sequential setup logic, no defer/goroutine/
// shutdown involvement, moved out to keep the composition root thin and
// under the cognitive-complexity guard. Behavior is unchanged from the
// inline version: same calls, same log lines, same fatal-on-hash-failure.
// External-service wiring (AI agent, Telegram, HTTP serve) lives in
// startup_services.go.

// validateProductionSecrets refuses to boot when JWT/encryption secrets are
// missing in production — otherwise we silently store Facebook cookies
// unencrypted or run with API auth disabled. Set APP_ENV=production to
// enable the strict check.
func validateProductionSecrets(cfg *config.Config) {
	if err := cfg.MustValidateProductionSecrets(); err != nil {
		log.Fatalf("❌ %v", err)
	}
	if cfg.JWTSecret == "" {
		log.Println("⚠️  JWT_SECRET not set — API authentication is DISABLED. Set it in production (APP_ENV=production blocks startup).")
	}
	if cfg.EncryptionKey == "" {
		log.Println("⚠️  ENCRYPTION_KEY not set — Facebook cookies stored unencrypted. Set it in production (APP_ENV=production blocks startup).")
	}
}

// bootstrapAdminUser creates the first admin user if ADMIN_EMAIL +
// ADMIN_PASSWORD are set and no users exist yet.
func bootstrapAdminUser(cfg *config.Config, db *store.Store) {
	if cfg.AdminEmail == "" || cfg.AdminPassword == "" {
		return
	}
	hash, err := authpkg.HashPassword(cfg.AdminPassword)
	if err != nil {
		log.Fatalf("❌ Admin password hashing failed: %v", err)
	}
	if err := db.EnsureAdminUser(cfg.AdminEmail, hash, cfg.AdminName); err != nil {
		log.Printf("⚠️  Admin bootstrap failed: %v", err)
	} else {
		log.Printf("✅ Admin user ready: %s", cfg.AdminEmail)
	}
}

// bootstrapSuperadmin upserts the superadmin unconditionally — works even
// when the DB already has users. Set SUPERADMIN_EMAIL + SUPERADMIN_PASSWORD
// in .env to activate.
func bootstrapSuperadmin(db *store.Store) {
	saEmail := os.Getenv("SUPERADMIN_EMAIL")
	if saEmail == "" {
		return
	}
	saPass := os.Getenv("SUPERADMIN_PASSWORD")
	if saPass == "" {
		log.Println("⚠️  SUPERADMIN_EMAIL set but SUPERADMIN_PASSWORD is empty — skipping")
		return
	}
	hash, err := authpkg.HashPassword(saPass)
	if err != nil {
		log.Printf("⚠️  Superadmin password hashing failed: %v", err)
		return
	}
	if err := db.EnsureFounder(saEmail, hash, os.Getenv("SUPERADMIN_NAME")); err != nil {
		log.Printf("⚠️  Superadmin upsert failed: %v", err)
		return
	}
	log.Printf("✅ Superadmin ready: %s", saEmail)
}

// initPortRegistry creates the workspace manager and wires its persistent
// PortRegistry so containers get deterministic host ports across restarts.
// Does not call ReconcileRunning or decide the shutdown-stop policy — those
// stay in main() next to the defer that depends on them.
func initPortRegistry(ctx context.Context, cfg *config.Config, appStore *store.AppStore) *workspace.Manager {
	workspaceMgr := workspace.NewManager(cfg.ChromePath, cfg.ProfileDir)

	portRegistry := workspace.NewPortRegistry(appStore.DB())
	if err := portRegistry.LoadFromDB(ctx); err != nil {
		log.Printf("⚠️  PortRegistry DB load failed: %v", err)
	}
	portRegistry.ReconcileFromDocker()
	workspaceMgr.SetPortRegistry(portRegistry)
	return workspaceMgr
}

// startHealthMonitoring wires the circuit breaker + restart controller +
// health checker (restart-storm guard) and starts the health-checker
// goroutine. Called synchronously from main() at the same point the inline
// version started it, so the `go healthChecker.Run(...)` goroutine begins
// at the same moment in the startup sequence — only where the statement is
// written changed, not when it executes.
func startHealthMonitoring(ctx context.Context, workspaceMgr *workspace.Manager, appStore *store.AppStore) {
	cb := workspace.NewCircuitBreaker(appStore.DB(), func(msg string) {
		log.Printf("[CircuitBreaker] ALERT: %s", msg)
	})
	restartCtrl := workspace.NewRestartController(workspaceMgr, cb)
	healthChecker := workspace.NewHealthChecker()
	go healthChecker.Run(ctx, workspaceMgr, func(accountID int64) {
		restartCtrl.OnUnhealthy(ctx, accountID)
	})
	log.Println("✅ Health checker started (15s interval)")
}

// watchdogOutcomeHandler builds the browser.Watchdog outcome callback. Moved
// out of main() as a named function (was an inline closure) — it does not
// reference any of main()'s later-assigned variables (e.g. telegramNotify),
// so extraction changes nothing about behavior, only where the closure is
// constructed. The returned func is passed to browser.NewWatchdog exactly as
// the inline version was; the `go watchdog.Run(ctx)` launch site in main()
// is untouched.
func watchdogOutcomeHandler(ctx context.Context, mgr workspace.ManagerIface) browser.UnhealthyFunc {
	return func(accountID int64, outcome browser.SessionOutcome, reason string) {
		switch outcome {
		case browser.SessionCDPDown:
			if os.Getenv("WORKSPACE_AUTO_RESTART_CDP_DOWN") != "1" {
				log.Printf("[Watchdog] CDP_DOWN account %d - keeping browser alive during login/session flow: %s", accountID, reason)
				return
			}
			log.Printf("[Watchdog] CDP_DOWN account %d — safe restart: %s", accountID, reason)
			if err := browser.SafeRestart(ctx, mgr, accountID, ""); err != nil {
				log.Printf("[Watchdog] SafeRestart failed account %d: %v", accountID, err)
			}
		case browser.SessionCheckpoint:
			log.Printf("[Watchdog] CHECKPOINT account %d — manual login required: %s", accountID, reason)
		case browser.SessionExpired:
			log.Printf("[Watchdog] EXPIRED account %d — session lost: %s", accountID, reason)
		case browser.SessionBlocked:
			log.Printf("[Watchdog] BLOCKED account %d — ban detected: %s", accountID, reason)
		}
	}
}
