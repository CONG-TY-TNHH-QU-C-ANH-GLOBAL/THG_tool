---
id: ARCHCP3a
status: REVIEW
lane: YELLOW
risk: YELLOW
depends_on: [ARCHCP1, ARCHCP2]
parallel_safe: false
branch: "chore/archcp3-textnorm-leaf"
pr_url: ""
boundary_target: leaf-move
---

# ARCHCP3a — Extract the generic text-norm leaf (ARCHCP3 phase 1, Option B)

## IMPLEMENTED (2026-06-29, branch chore/archcp3-textnorm-leaf)
Phase 1 of ARCHCP3 via **Option B incremental** (textnorm leaf first), self-selected
under Autonomy v2 + a senior-architect feasibility pass. Smallest reversible diff that
makes the eventual `copilot/intent` move clean and resolves the E3 boundary-pollution
question by construction (generic helpers leave `intent.*`).

- **New `internal/drivers/copilot/textnorm`** (pure, `strings`-only neutral leaf):
  `Fold` (Vietnamese diacritic fold + lowercase) + `ContainsAny` (folded-needle match;
  caller passes an already-folded value — historical contract preserved). Verbatim logic.
- **`intent_normalize.go`:** `foldVietnameseForMatch` / `containsAnyFolded` are now thin
  package-local **shims** delegating to `textnorm.*`, so the **42 existing copilot call
  sites across 8 files stay unchanged** (wrapper-first; no big-bang). `stripDashboardContext`
  stays — it strips the copilot "Dashboard context:" marker (copilot-specific, NOT generic
  text-norm), so it is deliberately NOT moved into the generic leaf.
- New `textnorm_test.go` characterizes Fold + ContainsAny (incl. the pre-folded-value
  contract). Existing `intent_router_test.go` covers the call sites via the shims.

Why only 2 of the 3 candidate funcs: cohesion honesty — a "generic text-norm" leaf must
not absorb a domain-specific stripper. textnorm = Vietnamese folding/matching only.

## Feasibility (verified)
No import cycle: `textnorm` imports only `strings`; `copilot → textnorm` one-way; nothing
outside copilot used the helpers. No RED zone (pure classification text-norm; no
queue/RBAC/auth/runtime). 42 refs / 8 files kept unchanged via shims. check_topology +
import-boundary (textnorm unflagged) + cognitive + file-size all green.

## Rollback
Revert the commit: the 2 shims become the original function bodies again and the leaf
is deleted. Pure code relocation; trivially reversible.

## Remaining ARCHCP3 phases (sequential follow-ups)
- **Phase 2:** rewrite the 42 call sites from the shims to `textnorm.*`; delete the shims.
- **Phase 3 (the real ARCHCP3 move):** move the genuinely-intent files (`intent_router`,
  `intent_entities`, `intent_types`, `intent_lexicon`, `intent_normalize` remnant + test)
  into `internal/drivers/copilot/intent/`, exporting only the ~6 real intent symbols.

## Validation
go build/vet/test ./... green; check_topology + go_cognitive_check + check_file_size +
import-boundary (warn-only, no new violation) pass. On merge → DONE.
