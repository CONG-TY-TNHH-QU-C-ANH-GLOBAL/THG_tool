// Package telegram marks the target inbound Telegram/webhook driver boundary: it
// translates Telegram input into typed application commands. Telegram is an
// interface, not a separate business-logic path.
//
// Architecture role: DRIVERS/TELEGRAM — see
// docs/architecture/ARCHITECTURE_STANDARD.md §3 and MODULE_BOUNDARIES.md.
//
//   - Allowed imports (conceptual): application command ports (consumer-owned),
//     models, stdlib.
//   - Forbidden imports (conceptual): DB repositories directly, store/outbound
//     internals, connector internals, business workflows. Telegram commands go
//     through the shared ActionContext → PolicyGate → Execution/Ledger spine; the
//     driver owns no workflow or outbound execution logic.
//
// SCAFFOLD ONLY (Phase A): boundary marker; no runtime logic lives here. The
// Telegram driver currently lives under internal/server/telegram and
// internal/telegram (see MODULE_OWNERSHIP.yml). Code migrates here only via a
// reviewed refactor — do not add or move runtime logic casually.
package telegram
