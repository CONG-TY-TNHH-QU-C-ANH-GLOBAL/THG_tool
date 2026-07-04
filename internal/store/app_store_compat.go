// Domain: app (see internal/store/DOMAINS.md)
package store

import (
	"github.com/thg/scraper/internal/store/app"
)

// AppStore is a deprecated source-compat alias for [app.Store]. The legacy
// *AppStore wrapper was dissolved (PR6, 2026-07-05): app_tasks/task_leads
// CRUD lives in internal/store/app and the bootstrap runs via app.Migrate
// inside store.New. This alias exists ONLY because
// internal/jobhandlers/facebook_crawl/handler.go cannot be edited until its
// 148-complexity Handle function is decomposed (the cognitive-complexity
// guard scans every function in a changed file). Remove the alias in that
// follow-up PR; do not add new callers.
//
// Deprecated: use [app.Store] via [Store.App].
type AppStore = app.Store
