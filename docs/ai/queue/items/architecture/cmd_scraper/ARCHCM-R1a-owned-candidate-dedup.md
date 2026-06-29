---
id: ARCHCM-R1a
status: DONE
lane: YELLOW
risk: YELLOW
depends_on: [ARCHCM-R1]
parallel_safe: false
branch: "chore/archcm-r1a-owned-candidate-dedup"
pr_url: https://github.com/CONG-TY-TNHH-QU-C-ANH-GLOBAL/THG_tool/pull/154
boundary_target: prep-extraction
---

# ARCHCM-R1a — Canonicalize the OWNER-scope role decision (ARCHCM-R1 Option A)

## Goal
Implement the code follow-up the founder approved in ARCHCM-R1 Option A: give the
role-aware "is this caller restricted to accounts they own?" decision a single
canonical home, and apply it to the outbound candidate pool. Behavior-preserving;
no gate semantics change; the three gates (CONTROL / OWNER / VISIBILITY) stay distinct.

## Component / domain
account-scope OWNER role classification (cmd/scraper, package main).

## What was duplicated
Both `ownedAccountCandidates` (outbound_action_context.go) and the inline gate in
`pickReadyFacebookAccountIDForCrawl` (crawl_runtime.go) independently computed the
same boolean: `restricted = userID > 0 && !IsPlatformRole(r) && r != RoleAdmin`.

## Why the audit's literal mechanism was NOT used
ARCHCM-R1 §9 suggested "make crawl call the existing `ownedAccountCandidates`
helper." On inspection that is NOT behavior-preserving: `ownedAccountCandidates`
returns an enumerated LIST, while crawl uses a permissive GATE
(`allow(id) = !ownedOnly || owned[id]`) that, for admin/platform and userID<=0,
intentionally allows ANY id — including a connector's `AssignedAccountID` not present
in `GetAllAccounts`. Reusing the list helper would narrow that to "only enumerated
org accounts" — an edge-case OWNER-gate semantics change (a hard stop). So only the
shared KERNEL (the role decision) was extracted.

## Scope delivered + crawl deferral (why)
Lane/risk YELLOW: behavior-preserving and package-internal, but it touches the
account-scope / RBAC role classification, so it is deliberately NOT classified GREEN.

- **Delivered:** extracted `callerRestrictedToOwnedAccounts(userID, role) bool` and
  applied it in `ownedAccountCandidates`. Pure, package-internal, behavior-identical.
- **Deferred to ARCHCM4:** applying the helper inside
  `pickReadyFacebookAccountIDForCrawl`. That function is RED-adjacent crawl runtime,
  measures cognitive complexity 23 (> 15), and is gated behind ARCHCM-R2 (crawl
  runtime semantics audit). Editing it makes it New Code and would force a
  complexity-reduction refactor of audit-gated dispatch/fallback code with no
  stage-level tests — outside this PR's safe boundary. The helper is ready; ARCHCM4
  adopts it when that file is refactored under ARCHCM-R2.

## Files touched
- `cmd/scraper/account_scope_role.go` (NEW, ~33 lines) — `callerRestrictedToOwnedAccounts`.
- `cmd/scraper/account_scope_role_test.go` (NEW) — characterization: sales restricted;
  admin/platform/founder/superadmin unrestricted; userID<=0 unrestricted; case + spaces.
- `cmd/scraper/outbound_action_context.go` — `ownedAccountCandidates` uses the helper
  (drops the now-unused `strings` import).
- `crawl_runtime.go` — **intentionally untouched** (see deferral).

## RBAC semantics confirmation
- CONTROL (`canRequesterControlAccount`, role-blind) — UNCHANGED, untouched.
- OWNER (`models.IsAccountOwnerAllowed`) — UNCHANGED, untouched.
- VISIBILITY (`models.CanViewAccountDevice`) — UNCHANGED, untouched.
- Only the shared OWNER-restriction *role classification* was centralized; no
  predicate logic changed for any caller / role / userID combination.

## Validation
go build/vet/test ./cmd/scraper/... (green) ; go_cognitive_check ; check_file_size ;
ai_validate.sh ; git diff --check. Existing store-fixture test
`TestResolveCallerAccountID` guards `ownedAccountCandidates`; new
`TestCallerRestrictedToOwnedAccounts` pins the shared decision.

## Done criteria
Role decision lives in one helper; outbound uses it; behavior identical for all
role/userID cases; new + existing tests green; crawl adoption tracked under ARCHCM4.
On merge → ARCHCM-R1a DONE.
