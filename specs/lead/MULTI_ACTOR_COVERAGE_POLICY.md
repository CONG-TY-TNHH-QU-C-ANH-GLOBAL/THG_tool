# Multi-Actor Brand Coverage Policy

## Direction

A lead is SHARED; execution is OWNED. Multiple staff/accounts reaching the same lead
with **different roles, tone, angle, CTA** is **brand coverage, not spam**. We do NOT
default to `max_one_org_comment_per_lead`. Instead a **Coverage Policy** caps and
shapes how much, how often, and how varied the org's touches on one lead may be.

This supersedes the per-account dedup as the *planner-level* eligibility rule. The
per-account dedup (already-commented-by-this-account) remains the floor.

## Contracts (PR-0 — `internal/models/coverage.go`)

### CoveragePolicy (per-org config; `DefaultCoveragePolicy()`)
| Field | Default | Meaning |
|---|---|---|
| `AllowMultiActorCoverage` | `true` | multiple accounts may comment one lead |
| `MaxAccountsPerLead` | `2` | distinct accounts that may touch a lead |
| `MaxURLCommentsPerLead` | `1` | only one comment may carry the website URL |
| `MaxDirectCTACommentsPerLead` | `1` | only one comment may use a hard inbox CTA |
| `MinGapBetweenActorComments` | `30m` | spacing between two actors' comments |
| `RequireUniqueAngle` | `true` | a later actor must not repeat an earlier angle |
| `StopIfLeadReplies` | `true` | stop touching a lead that has replied |

### LeadCoverageState (DERIVED from the engagement ledger — verified truth, not UI text)
`ActorsTouched []int64`, `OrgTouchCount int`, `WebsiteAlreadyUsed bool`,
`DirectCTAAlreadyUsed bool`, `UsedAngles []string`, `LastTouchAt time.Time`,
`LeadReplied bool`.

### ActorPersona (input to generation — varies each actor's comment)
`Role`, `Tone`, `AllowedCTAStyle` (`direct_inbox` | `experience_share`),
`ForbiddenRepeatedPhrases []string`, `LinkPolicy` (`may_include_website` | `no_link`).

## Pure rules (PR-0)

`EvaluateCoverage(state, policy, actorAccountID, now) (eligible bool, reason string)`:
- same actor already touched → `already_commented_by_this_actor`
- `StopIfLeadReplies && LeadReplied` → `lead_replied`
- `!AllowMultiActorCoverage && OrgTouchCount > 0` → `single_actor_policy`
- `OrgTouchCount >= MaxAccountsPerLead` → `coverage_full`
- `now - LastTouchAt < MinGapBetweenActorComments` → `coverage_gap_too_soon`
- else `ok`

`DeriveActorPersona(state, policy, role, tone) ActorPersona`:
- `WebsiteAlreadyUsed` (and policy cap reached) → `LinkPolicy=no_link`, else `may_include_website`
- `DirectCTAAlreadyUsed` (and policy cap reached) → `AllowedCTAStyle=experience_share`, else `direct_inbox`
- `ForbiddenRepeatedPhrases = UsedAngles` (don't repeat an earlier angle)

## Staged PRs

- **PR-0 (this)**: contracts + pure `EvaluateCoverage` / `DeriveActorPersona`, fully
  unit-tested. No wiring, no behavior change.
- **PR-1**: project `LeadCoverageState` from the engagement ledger (per-lead, per-org)
  — `ActorsTouched`, `WebsiteAlreadyUsed`, `DirectCTAAlreadyUsed`, `LastTouchAt`,
  `LeadReplied` from verified `action_ledger` / `conversation_threads`. Store-layer,
  contract-shaped, `// tenant-ok` cross-domain read.
- **PR-2**: planner uses `EvaluateCoverage` per candidate (replacing the blunt
  per-account skip) inside the eligible-fill loop; skip reasons mapped to plain
  Vietnamese; load `CoveragePolicy` (default until a settings UI exists).
- **PR-3**: thread `DeriveActorPersona` into the comment prompt (role/tone/CTA-style/
  link-policy/forbidden-phrases) so each actor's comment is genuinely varied.

## Guardrails
No `Engine/Manager/Service/Dispatcher/Coordinator` naming (coordination domain rule).
Branch on explicit fields (deterministic boundaries). Coverage state is the verified
ledger projection, never UI text. New files < 200 lines.
