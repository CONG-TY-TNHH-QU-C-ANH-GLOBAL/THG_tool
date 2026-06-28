---
id: ARCHCM-R1
status: BLOCKED
lane: RED
risk: RED
depends_on: []
parallel_safe: false
branch: "audit/archcm-r1-account-scope-rbac"
pr_url: ""
boundary_target: blocked-decision
blocked_on: founder-approval-of-decision
audit_status: COMPLETE
---

# ARCHCM-R1 — AUDIT: consolidate duplicated account-control RBAC

## Goal (audit-only — security critical)
"Can requester control this account?" / "pick a safe account" logic is spread across
facebook_account_scope.go, direct_post_account_guard.go, outbound_action_context.go,
and crawl_runtime.go. Decide the single canonical home + API.

## Component / domain
account ownership / RBAC authorization. RED (auth/security).

## Dependencies
Blocks ARCHCM2 and ARCHCM4 (they sit on this RBAC); ARCHCM2 in turn gates ARCHCM3.

---

# DECISION RECORD (audit complete 2026-06-28 — awaiting founder approval)

## 1. Headline finding — this is NOT one duplicated predicate

The premise ("duplicated account-control RBAC, pick one canonical home") is partly
false in a way that matters for safety. The code holds **three deliberately
DIFFERENT authorization gates**, documented as distinct in their headers. Merging
them is a security regression, not a cleanup:

| Gate | Predicate | Role behaviour | Used for | Home |
|---|---|---|---|---|
| **CONTROL** | `canRequesterControlAccount` (+ connector-ownership conjunct `connectorControllableBy`) | **role-BLIND — admin grants NOTHING** | Facebook WRITE side-effects (comment / inbox / post) | `cmd/scraper/facebook_account_scope.go` (confined to 2 cmd files) |
| **OWNER** | `models.IsAccountOwnerAllowed` | **role-AWARE — admin/platform PASS** | account *resolution* / candidate filtering for outbound + crawl | **already canonical** in `internal/models/permissions.go` |
| **VISIBILITY** | `models.CanViewAccountDevice` | admin sees unassigned only, never another member's | read/device visibility | `internal/models/permissions.go` (out of scope — control ≠ visibility) |

The CONTROL header is explicit: *"A member-owned account is NEVER controllable by
an admin … admin ROLE grants nothing here."* That is the **exact opposite** of
`IsAccountOwnerAllowed` (admin passes). A naive `CanControl` that unifies them would
either let an admin comment/post from a member's Facebook identity (write-gate
regression) or make resolution role-blind (breaks admin account management). This
divergence is **intentional and correct**, and is why the item is RED.

## 2. Current coupling map

```
WRITE side-effects (comment/inbox/post)         RESOLUTION / candidate pool
────────────────────────────────────            ──────────────────────────────
facebook_account_scope.go                        outbound_action_context.go
  canRequesterControlAccount  ◄── role-blind       resolveCallerAccountID
  connectorControllableBy                          callerAccountForExplicitID ─► models.IsAccountOwnerAllowed
  controllableConnectors                           ownedAccountCandidates  ◄─┐  (admin passes)
  liveReadyControllableAccountIDs                                            │
  resolveControllablePool / runPooledOutreach     crawl_runtime.go          │ SAME role-aware
direct_post_account_guard.go                       pickReadyFacebookAccountIDForCrawl
  resolveDirectPostAccount  ──► canRequesterControlAccount   inline role-branch (lines 155-166) ┘ ◄── TRUE duplication
  guardFacebookWriteAccount                          (userID<=0 → all; admin/platform → all; sales → GetAccountsForUser)
```

- `models.IsAccountOwnerAllowed` is the **shared spine** of the OWNER gate: **6
  production call sites** across `cmd/scraper`, `internal/readiness`,
  `internal/server/agent` (×2: `account_guard.go`, `local_connector.go`), and
  `internal/store/execution_context.go`. It is already consolidated — moving
  ARCHCM2/CM4 does not disturb it.
- The CONTROL gate (`canRequesterControlAccount` + connector helpers) is **confined
  to the two `cmd/scraper` write-guard files** and is fully test-covered. It is
  already as "canonical" as it needs to be for the moves.

## 3. The ONLY genuine duplication

The role-aware **owned-candidate filter** is copy-pasted in two places:

- `ownedAccountCandidates` — `outbound_action_context.go:84-94` (a named helper).
- inline role-branch — `crawl_runtime.go:155-166` inside
  `pickReadyFacebookAccountIDForCrawl`.

Both encode the identical rule ("userID≤0 → all org accounts; admin/platform → all;
sales → GetAccountsForUser"). This is the real, safe-to-remove duplication — and
both live in `package main` (cmd/scraper), so deduping needs **no new package, no
export, no import change**.

## 4. Call-site / export-count report

| Symbol | Non-test refs (incl. def) | Visibility | Notes |
|---|---|---|---|
| `IsAccountOwnerAllowed` | 11 (6 prod call sites) | exported (models) | already canonical OWNER gate |
| `canRequesterControlAccount` | 7 | package-private (main) | CONTROL gate, 2 files |
| `controllableConnectors` | 7 | package-private (main) | connector-ownership conjunct |
| `connectorControllableBy` | 3 | package-private (main) | — |
| `liveReadyControllableAccountIDs` | 4 | package-private (main) | — |
| `resolveDirectPostAccount` | 4 | package-private (main) | single-account write resolve |
| `ownedAccountCandidates` | 3 | package-private (main) | **dedup target** |
| `pickReadyFacebookAccountIDForCrawl` | 2 | package-private (main) | holds the duplicated branch |
| `resolveCallerAccountID` | 6 | package-private (main) | OWNER resolution entry |

Export surface required for the recommended path: **0 new exports** (the dedup is
within `package main`).

## 5. Import-cycle risk
- Recommended path (Option A): dedup within `package main` → **no import change, no
  cycle**.
- Full-unification path (Option B): a new `internal/identity` authority imported by
  `cmd/scraper` + `internal/models` consumers — `models` is a neutral leaf, so the
  control predicate cannot move INTO `models` without dragging `connectors`
  (`AgentToken`) into the leaf → **leaf-pollution / potential cycle**. Another reason
  B is heavy.

## 6. Controlled-zone risk
- **RED (auth/RBAC):** both gates are authorization surfaces. Any predicate-semantics
  edit is RED and out of scope for autopilot.
- **CAS-adjacent:** the CONTROL gate feeds the Facebook write/CAS path
  (`resolveControllablePool` → `runPooledOutreach`); changing it risks the
  idempotency/identity guarantees the write path depends on.
- The recommended path touches **neither predicate** — only the role-aware *candidate
  filter* (which `IsAccountOwnerAllowed` already governs), so it carries no
  predicate-semantics risk. It does edit `crawl_runtime.go` (a runtime file), so the
  impl PR is YELLOW and needs a crawl characterization test.

## 7. Options

- **Option A — Name the gates; dedup only the candidate filter (RECOMMENDED).**
  Ratify in this record that CONTROL / OWNER / VISIBILITY are three distinct
  canonical gates and must NOT be merged. Canonical homes: OWNER =
  `models.IsAccountOwnerAllowed` (already); CONTROL = the `facebook_account_scope.go`
  cluster (stays in cmd/scraper for now; a later leaf-move is a separate item, not
  this one). The only code follow-up is a small PR that makes
  `pickReadyFacebookAccountIDForCrawl` call the existing `ownedAccountCandidates`
  instead of re-implementing its role-branch. 0 new exports, no import change,
  behavior-preserving. **Unblocks ARCHCM2/CM4** because their open question —
  "does moving outbound/crawl disturb account scope?" — is answered: scope is
  already centralized and test-covered.

- **Option B — Full unification under a new `CanControl` / `PickSafe` API
  (`internal/identity`).** Merge control + owner into one parameterised authority.
  HIGH RISK: must preserve role-blind-for-write vs role-aware-for-resolve as an
  explicit parameter; any slip is an auth regression. Needs RBAC-1 spec sign-off,
  full characterization of both gates, staged additive→migrate PRs, a new package +
  export surface, and risks leaf-pollution (§5). Not recommended now.

- **Option C — Defer entirely.** Mark the divergence intentional and stop. Zero risk,
  but leaves the real candidate-filter duplication (§3) and keeps ARCHCM2/CM4 blocked
  on an unanswered scope question.

## 8. Recommended default: **Option A**

It resolves the audit (names canonical homes, distinguishes the three gates,
isolates the one true duplication), unblocks the downstream moves, and scopes the
only code change to a tiny semantics-preserving extraction — explicitly NOT an
authorization-surface change. RBAC-1 conformance is preserved by construction
(no predicate edited); the dedup PR re-runs the existing gate tests
(`facebook_account_ownership_test.go`, `permissions_test.go`,
`direct_post_account_guard_test.go`) plus a new crawl candidate-filter test.

## 9. Exact next implementation slice (if Option A approved)

1. **New item `ARCHCM-R1a` (lane YELLOW, risk YELLOW — touches `crawl_runtime.go`):**
   replace the inline role-branch in `pickReadyFacebookAccountIDForCrawl`
   (`crawl_runtime.go:155-166`) with a call to the existing `ownedAccountCandidates`
   helper. Add a crawl-path characterization test asserting: sales → only owned
   accounts; admin/platform → all; `userID<=0` → all (org-wide, unchanged).
   Behavior-preserving; ~15-line diff; no new exports; no import change.
2. Then mark ARCHCM-R1 DONE → **ARCHCM2 and ARCHCM4 become executable.**

(Optional later, separate item — NOT this decision: promote the CONTROL cluster to
an `internal/identity` or `internal/connectors` leaf once a move is independently
justified. A durable ADR under `docs/architecture/decisions/` can capture the
three-gate model if the founder wants it outside the queue.)

## Validation
- **N/A for this audit** — no production code changed (docs/queue item only).
- The follow-up **ARCHCM-R1a** implementation PR must add RBAC / crawl
  characterization tests covering all three role cases: **sales** (→ only owned
  accounts), **admin/platform** (→ all org accounts), and **userID<=0** (→ org-wide,
  unchanged).

## Done criteria
Decision record naming the canonical implementation(s) + the three-gate distinction,
call-site/export map, and RBAC-1 conformance plan — **approved by a human.** Until
approval, status stays BLOCKED.
