---
doc_type: engineering
status: active
owner: platform
last_reviewed: 2026-10-21
related_pr_or_issue: Add Taobao sourcing to replies
---

# Release the eight approved public company facts

## Approval and scope

On 2026-10-21 the task owner answered **“Duyệt 8 mục này để nạp”** to the review
of [the eight-record manifest](../business/company-knowledge/release-review-2026-10-21.json).
That review was explicit: POD, sourcing, express, US warehouse, the three pricing
pages' purposes and the sourcing order process. It excluded fixed prices, service
acceptance, guarantees, compensation, basecost, tracking SLA and the whole 92-item
pack. Website verification alone does not authorize approval.

The review file is preserved byte-for-byte (except checkout line endings),
including its historical `pending_owner_approval` / `not_loaded` metadata. The
later approval is recorded here, in the maintenance command's approval reference
and in each database approval audit event. Do not interpret the old review
metadata as a current production status. No other pending pack entries are approved.

Reviewed manifest SHA-256 after CRLF→LF normalization:
`ed882debb485e5e55eeb3829ad9aaf05c548d1cfb4fc318bd92ae966c2aa4410`.
The command rejects any different file, including added records. New facts need
another review/release. Search aliases are the existing pack's English search
metadata; they add no service promises. Raw SOP/guidance is never imported.

## Ownership

- `cmd/company_knowledge_release` owns this bounded operator operation and the
  frozen content identity; it is not an HTTP endpoint or a general ingestor.
- `knowledge.Store.PublishReviewedAssets` owns all transactional writes to
  knowledge assets, source health and append-only approval feedback.
- Existing retrieval and suggestion builders keep their public contracts.
- Crawl/intake/scoring/Telegram eligibility, catalog sync, Training sync and
  customer records must not know about or be modified by this operation.

## Production operation

Use the **Release reviewed company facts** manual GitHub workflow at a reviewed
commit. `check` is the default. Run `apply` only for the approved release above.
`withdraw` removes this release from retrieval if its content needs correction.

The workflow tests the importer/store, builds for the configured server CPU,
copies a credential-free maintenance package, and runs the operation under a
server lock. It proves that both active scraper/worker processes use the target
SQLite file. The command requires an existing database, performs no migrations,
and requires exactly one workspace whose space-normalized name is THGFulfill.
No org ID or process environment is printed. This is a deliberately scoped
SQLite operation; PostgreSQL execution has not been verified.

Before either write operation, an online SQLite backup is created and checked
on the server under `/opt/thg-scraper/backups/company-knowledge/`, with restricted
permissions. It is not downloaded or printed. There is no backup retention cleanup
in this operation. Check/import/withdraw do not restart services or send messages.

`apply` creates one manual website source identified by the full manifest hash.
It starts syncing/unpublished. A transaction inserts exactly eight approved
public projections, one approval audit row per asset (maintenance actor 0 plus
the task-owner approval reference), and healthy source status. Failure rolls
back assets, audit and publication together; an empty syncing source may remain
and can be retried. It does not update existing source assets.

Repeated apply checks the same manifest/provenance, asset count, titles, types,
public payloads, search aliases and approval state. It never overwrites edits,
re-approves hidden assets, or approves other source content. The server workflow
and `flock` serialize these operations; the CLI must not be run concurrently by
other means. No new source type or database schema is introduced.

`check` opens SQLite in `mode=ro`. After import it verifies all eight projections
can appear through `Builder.ReplyKnowledgeForLead` using title/alias queries
against the actual full store. This is an exact-content retrieval check, not a
measurement of recall for all customer language or model response quality.
An absent source reports `not_loaded`; a changed/hidden/unhealthy source fails.

## Withdrawal and recovery

Run the same workflow with `withdraw` at the same reviewed commit. It verifies
the exact source/content and marks this source unhealthy before hiding its eight
assets. Thus interruption during hiding cannot leave remaining assets eligible
for reply retrieval. Existing source-health filters enforce exclusion. Each
newly hidden asset gets a reject audit event. No unrelated source is changed.
Do not restore the whole runtime database merely to withdraw these facts.

Withdrawal is repeatable and apply refuses to re-enable a withdrawn release.
If a failure interrupts hiding/audit, inspect the maintenance result and finish
withdrawal; the source stays excluded. A content correction requires a new
approved release, not an in-place re-sync. The command does not automatically
hide older unrelated public sources, which must be reviewed separately if found
to conflict with these facts.

## Verification and limits

Tests cover transaction rollback on a real unique-constraint failure (including
audit/source publication), org isolation, unchanged unrelated pending data,
read-only database access, no creation of a missing DB, ambiguous org rejection,
changed manifest rejection, exact eight-fact retrieval, idempotence, operator
hide protection and withdrawal. Fixture data is synthetic in temporary DBs.

Successful import proves the approved facts exist and are retrievable. It does
not prove all Telegram leads will have replies: noncommercial/unclear intent,
provider errors, model guards and missing source data still produce notes or
fallbacks. Model output quality, real photo interpretation, recall and language
naturalness need separate evaluation. The 36 research scenarios remain proposed
coverage, not 36 completed production evaluations. Do not replay old leads or
send synthetic Telegram messages to obtain verification.

### Optional real-model preview

The workflow's explicit `model-preview` operation first checks the actual DB
release and retrieval. It then calls the existing production suggestion facade
on four fixed synthetic posts (US warehouse, CN→US shipping, sourcing process,
noncommercial announcement), with only these eight approved public projections
in memory. The prepared preview pack omits every other draft, SOP and guidance.
The existing preview's draft/simulated-approval metadata is retained by contract;
it does not assert that model generation reads the whole production database.

Model credentials are read in-process from the existing private server env file;
they are never copied into the maintenance package or printed. This operation
makes provider requests using the existing model config, with no Elim/shipping
lookup, live photo, production lead read, DB write or outbound message. Output is
unreviewed synthetic model replies plus fixed status codes; review their wording
and factual grounding separately. A successful workflow does not grade quality
or prove the live org profile, crawl, Telegram and real-image paths end to end.

### First model trial, 2026-10-21

Apply `39576976900` confirmed eight actual DB projections retrievable at
09:28:04 UTC. Independent read-only check inside model trial `39577283674`
confirmed the same at 09:30:28 UTC. The existing configured preview model was
`deepseek-v4-flash`; this is observed configuration for that trial, not a new
provider/model selection. Warehouse/shipping/process wording returned
`model_generated` in about 2 seconds each. The noncommercial announcement had
no reply and an analysis-unavailable fallback note, not a verified model
noncommercial classification. No outbound messages or sourcing requests.

Manual review found a real gap: the process question got an item/quantity
clarification instead of the available process explanation. Offline retrieval
with precisely the eight approved facts ranks `commercial-order-process`
first for that exact synthetic question. The prompt told the model to preserve
the fallback's missing-information question. It now allows replacing that
generic question with a fact-grounded answer to the author's actual question,
explicitly covering process explanations. Product/quote details, source/link/
price qualifications and all downstream safety guards stay required. This is
a narrow wording change that needs another actual-model trial; it is not proof
of broad intent accuracy or semantic grounding.
