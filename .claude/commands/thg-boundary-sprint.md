# THG Boundary Architecture Sprint

You are running an accelerated architecture boundary sprint for THG AutoFlow.

Primary goal:
Move real production boundaries toward the approved THG architecture:

* modular monolith;
* clear domain packages;
* composition root in `cmd/scraper`;
* no abstraction theater;
* consumer-owned ports only when needed;
* explicit ownership of store/session/outbound/workspace domains;
* behavior-preserving boundary moves before behavior changes.

This command is NOT for:

* docs-only survey PRs;
* cosmetic cleanup;
* random Sonar hunting;
* tiny one-wrapper deletion PRs unless they unlock a larger boundary move;
* broad formatting;
* speculative abstractions;
* microservice extraction.

## Sprint mode

Think in sprint batches, not isolated one-line fixes.

In one sprint, search for 2–4 related architecture moves that can be safely
done together.

Prefer high-leverage targets:

1. finish an already-started boundary migration;
2. dissolve legacy bridge/alias surfaces after callers are migrated;
3. move cohesive store/domain logic into an owned subpackage;
4. reduce composition-root complexity only when it unlocks a real boundary
   migration;
5. remove proven-dead compatibility surfaces only when zero callers are proven;
6. migrate callers from legacy root/AppStore surfaces to domain accessors;
7. collapse stale bridge layers after migration.

Avoid low-leverage targets:

* comment-only changes;
* docs-only corrections unless they unblock code work;
* renaming without boundary improvement;
* cleanup that does not reduce coupling;
* broad lint/format churn;
* unrelated Sonar cleanup.

## Required starting protocol

Before editing:

1. Sync:

   * `git checkout main`
   * `git pull origin main`
   * verify clean working tree.

2. Survey boundaries:

   * inspect `internal/store`;
   * inspect `internal/server`;
   * inspect `internal/session`;
   * inspect `internal/workspace`;
   * inspect `cmd/scraper`;
   * inspect existing queue/docs only as guidance, not as authority.

3. Identify candidates:
   For each candidate, report:

   * target boundary;
   * current coupling;
   * exact files likely touched;
   * expected behavior risk;
   * whether it is GREEN/YELLOW/RED;
   * whether it can be batched with another candidate;
   * why it is higher leverage than small cleanup.

4. Select sprint batch:
   Choose the largest safe batch that satisfies:

   * one coherent architecture theme;
   * no schema/migration unless explicitly approved;
   * no auth/session/security behavior change unless explicitly approved;
   * no queue/CAS/lease/outbox/action-ledger semantic change unless explicitly
     approved;
   * no public route/wire contract change unless explicitly approved;
   * no VNC/VNCPort/readiness work unless explicitly requested;
   * no unrelated docs-only cleanup.

5. Report the sprint plan before coding:

   * selected batch name;
   * exact files expected to change;
   * exact old → new boundary;
   * expected caller migration list;
   * compatibility bridges to keep/remove;
   * stop conditions;
   * rollback plan.

## Allowed acceleration

You MAY batch multiple tasks when they are related and safe, for example:

* migrate all remaining callers of one domain bridge;
* remove bridge methods and aliases after zero callers are proven;
* move a cohesive store cluster into a subpackage;
* extract helper functions from `cmd/scraper/main.go` only when required to
  support a boundary migration;
* update tests in the same PR when behavior is preserved but compile/test
  coverage needs adjustment.

You MAY touch RED-zone files only for safe migration phases:

* pure caller migration;
* additive facade/accessor;
* compatibility wrapper;
* characterization tests;
* no semantic cutover.

You MUST stop and ask before:

* schema migration/drop/rename;
* auth/RBAC/security semantic change;
* queue/CAS/lease/outbox/action-ledger semantic change;
* connector TTL/idempotency change;
* public API/DTO/wire behavior change;
* product-visible behavior change;
* irreversible deletion without zero-caller proof;
* changing workspace readiness/VNC/CDP behavior unless explicitly requested.

## Boundary standards

Composition root:

* `cmd/scraper` may wire dependencies.
* Helper extraction is allowed only to reduce complexity or improve
  composition clarity.
* Do not move defers into helpers if it changes lifetime.
* Do not change goroutine start/shutdown order.
* Do not change signal/shutdown semantics.

Store:

* Prefer concrete domain subpackages such as `internal/store/sessions`,
  `internal/store/outbound`, `internal/store/knowledge`, etc.
* Do not introduce repository interfaces unless a real consumer-owned boundary
  needs one.
* Do not create abstraction theater.
* Root `store.Store` may expose domain accessors.
* Legacy `AppStore` can remain until each domain is dissolved safely.
* Remove bridge methods only after zero callers are proven by grep and build.

Server:

* Do not move HTTP handlers across packages unless the target ownership is
  clear.
* Preserve route paths, auth middleware, response shape, and error status
  unless explicitly approved.

Runtime/workspace:

* Do not change browser readiness, CDP/VNC behavior, Docker startup, port
  allocation, or session semantics unless explicitly approved.

## Diff discipline

* No broad formatting.
* Format touched files only if required.
* Do not use `git add -A`.
* Revert test artifacts such as `RETRIEVAL_SOAK_REPORT.md`.
* Expected diff must map to the selected boundary.
* If unrelated churn appears, revert it before committing.

## Validation

Run the strongest practical validation for touched areas:

Required baseline:

* `go test ./...`
* `go vet ./...`
* `scripts/ai_validate.sh`
* `scripts/check_import_boundaries.sh`
* `scripts/go_cognitive_check.sh`
* `scripts/check_file_size.py`
* `git diff --check`

Add targeted tests for touched areas, for example:

* `go test ./internal/store/...`
* `go test ./internal/session/...`
* `go test ./internal/server/...`
* `go test ./internal/workspace/...`
* `go test ./cmd/scraper/...`
* frontend build/typecheck if frontend is touched.

Sonar expectation:

* New Code issues must be 0.
* No suppressions.
* No Sonar config changes.

## Commit and PR policy

* Create one branch for the sprint batch.
* Commit once unless multiple commits are necessary for clarity.
* Push branch.
* Never merge.
* Final report must include:

  * branch and commit;
  * selected sprint batch and why;
  * candidates considered and rejected;
  * exact old → new boundaries;
  * exact files changed;
  * caller migration proof;
  * bridges kept/removed;
  * behavior-preservation proof;
  * tests/guards run;
  * Sonar expectation;
  * rollback plan;
  * next recommended sprint batch.

## Strong default target selection

If no higher-leverage target is found, prefer this order:

1. Finish the current `sessions` domain boundary:

   * migrate remaining top-level `store.BrowserSession`,
     `store.LocalSessionStatus`, and `store.Session*` alias users to
     `internal/store/sessions`;
   * remove unused top-level session aliases/constants after zero callers are
     proven;
   * do not remove `AppStore` itself.

2. Continue `*AppStore` dissolution:

   * survey `identities.go` and `learning.go`;
   * choose the more cohesive/lower-risk cluster;
   * move implementation to a domain subpackage with accessor;
   * keep compatibility bridge if callers are not fully migrated;
   * migrate as many callers as safely possible in the same sprint.

3. Reduce `cmd/scraper` composition-root complexity only when it unlocks a
   real boundary migration.

Do not choose VNC/VNCPort/readiness work unless explicitly requested.
