// Package automation marks the target boundary for cross-vertical automation
// coordination — the vertical-neutral glue shared by service modules (e.g. shared
// scheduling/sequencing helpers), should one be needed.
//
// Architecture role: APPLICATION (cross-vertical) — see
// docs/architecture/ARCHITECTURE_STANDARD.md §3 and MODULE_BOUNDARIES.md.
//
//   - Allowed imports (conceptual): shared primitives via ports, models, stdlib.
//   - Forbidden imports (conceptual): platform-specific logic of any vertical,
//     drivers/*, internal/server transport. This package MUST NOT become a vague
//     "utils"/"common" dumping ground; platform-specific workflows belong under
//     internal/services/<service>.
//
// SCAFFOLD ONLY (Phase A): boundary marker; no runtime logic lives here. Add code
// only via a reviewed refactor that justifies cross-vertical placement — do not add
// runtime logic casually.
package automation
