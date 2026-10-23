---
doc_type: engineering
status: draft
owner: platform
last_reviewed: 2026-10-22
related_pr_or_issue: Add Taobao sourcing to replies
---

# Need analysis and grounded operator replies

## State

Baseline deployed through PR #295 at `8ccf5b1b`; runtime revision, active
services, context/image flags and THGFulfill analysis scope were verified by
read-only diagnostic `39580706739` on 2026-10-21. Zero suggestion events in its
20-minute window establish readiness only. See [diagnostic evidence](LEAD_SUGGESTION_DIAGNOSTIC.md).
The image-path audit was released in #296 and the expanded-comment fix in #297;
read-only revision checks confirmed both running binaries at `5303dd6f`.
The 92-record authoring pack remains pending/not_loaded; exactly eight reviewed
public projections were separately approved and imported at 09:28 UTC on
2026-10-21. See [release scope and evidence](COMPANY_KNOWLEDGE_RELEASE.md). On 2026-10-19
the task owner deferred Muse Spark and chose the existing configured provider/model
for the next phase. No new Meta account/key or self-hosted model is required.
Muse Spark has not been called. Authorized server-side text-only trials used the
existing configured `deepseek-v4-flash`, observed from actual preview output,
not inferred from the source default. Trial `39578825596` produced three drafts;
the process case had an unclear analysis note and the announcement used an
analysis-unavailable fallback. These are small synthetic observations, not a
broad quality/latency/cost evaluation. Actual photo recognition and real new-lead
reply quality remain **unverified**.

### Image-path audit (2026-10-21)

Unified need analysis adds bounded, optional diagnostic metadata only. Per-lead
logs record `event_images` (0–3 validated photo references in the notification
event), `model_images` (0–3 references passed to the unified analysis attempt),
and an allowlisted `image_analysis` result. They never log photo URLs, signed
queries, image observations or provider bodies. Counts describe references,
not independently verified delivery or recognition. `analysis_failed` includes
provider, timeout and response-validation failures; it does not identify their
root cause. Successful `observations` means validated nonempty observations
were returned, not that the product/SKU was correctly identified.

States are `none`, `references_rejected`, `analysis_failed`, `observations`,
`no_observations`, `conflict`; other/legacy paths render `not_recorded`. Old
logs with absent fields cannot establish that the post had no images. Image
references rejected before the LeadEvent are not recoverable from these counts.
An event with images and no recorded analysis can indicate a disabled/unavailable
or legacy processing path; inspect effective switches/scope before diagnosing.

The read-only lead diagnostic reports only org-scoped aggregate counts, or
counts for the existing numeric target-post input. It does not replay a lead,
read image content, send messages or change configuration. The notification
and worker roots pass event counts to the logger. The service owns the analysis
audit; shared models only carry its optional metadata. Tests compare audit
counts with real requests to a fake HTTP provider, including early conflict,
empty observations, rejected URLs and provider failure, and check redaction.

Upstream intake/scoring still sees text and may reject a short/vague post before
image analysis. This audit deliberately preserves the existing admission policy;
changing discovery or accepted-lead counts requires a separate scoped decision.
A genuine original post link and a current permitted browser session are needed
to verify collection → intake → provider → suggested reply with a real photo.
The company-knowledge model trials used text only; they are not vision evidence.
The existing provider request has a five-second analysis cap; image URL access
and latency still need an actual trial before changing its budget or model.

### Capture dependency and purchase-scope clarification (2026-10-22)

Read-only connector-version probe `39711376425` found recent active versions
`0.5.29.878`, `0.5.56.992`, `0.5.56.1000`, and `0.5.57.1228` in THGFulfill.
Versions are self-reported, not proof which connector produced a target lead.
The published deploy #1228 maps to `25f33d02`, before #286, and that tree lacks
the photo collector. Server deployment does not update an installed extension.
The currently served official beta `0.5.57.1284` was downloaded and inspected:
its manifest loads `platforms/facebook/crawl_images.js`, matching main's source.
Use the Web Store release or CI-built dashboard beta, never a repo folder;
after updating, reload the existing Facebook tab so its scripts are current.
Do this on the actual scanner's Chrome profile. Do not interrupt an active job,
change accounts, reveal pairing tokens, or replay an old lead as a test.

The read-only diagnostic now reports bounded org-scoped recent connector-version
counts. It selects no token hash, hostname, browser identity or content; invalid
version strings are `unreported`. Missing/unsupported SQLite tables return an
unavailable code, not a cross-plane fallback or an empty DB creation.

An optional `scope_clarification` response contains kind `purchase_scope` and
exactly two short category alternatives. Only sourcing/POD with request evidence
can use it. Each phrase must occur as whole words in the post or in observations
of an actually supplied image. Numbers, URLs, unknown keys, duplicate/blank
options and unsupported photo categories are rejected. Observations remain
tentative model output, not independent visual truth. Legacy responses omitting
the field keep the existing catalog/sourcing paths.

PR #299 originally made this a blocking customer clarification. Subsequent owner
review rejected that sales behavior. The revision in this branch keeps uncertainty
in operator notes (`scope_review`) and tries the existing bounded source/catalog
path for an explicit listing, a supported named text category, or a validated
visible primary category when no text category is named. Text retains precedence;
two explicitly undecided text options cannot become one combined search or a
random choice. No additional parallel source searches or inferred freight.

If no usable offer is found, the draft offers to check sourcing and invites a
message, optionally requesting an unknown quantity. It does not ask material
versus finished item or ask the customer to describe an observed product again.
Lookup failures remain in operator notes; a contact draft is not a stock or
successful-source claim. Actual text/image conflict still prevents product/rate
selection, but its draft is proactive contact and the conflict stays internal.
The same proactive fallback applies to valid observed sourcing/POD references
without an offer; service/noncommercial and missing-image paths retain their
existing behavior. Model rewriting is prompted to lead with a sourced option or
useful action and never imitate competitors' stock, quote deadlines or promises.

Known quantity is not asked again. Explicit material requests stay material
requests even if a photo shows a finished object; visible personalization alone
does not establish POD. Model detection still needs real quality evaluation;
the typed route is not proof that every ambiguity is caught.
The existing text quantity parser recognizes explicit cuộn/roll(s)/skein(s)
counts too; it does not infer weight or convert material units from appearance.

POD still requires a matching company catalog link and
Dropship a verified Taobao/1688 listing under the existing source/claim checks.
No fallback may invent a link to make this requirement appear satisfied.

This additive feature affects operator suggestions. It does not post Facebook
comments, approve/import knowledge or change CRM contracts or crawl eligibility.

#### Historical #299 isolated real-model observation

Run `39713017297` at `08c763a7` used the configured `deepseek-v4-flash` with
six public synthetic text cases, eight approved facts and no live sourcing,
freight, database writes or Telegram messages. Workflow success means the
preview ran, not that all six interpretations passed semantic acceptance.

| Case | Observed result |
|---|---|
| Warehouse question | Relevant service draft; `model_generated` |
| US shipping question | Guard rejected rewrite; grounded service question retained, `model_rejected` |
| Sourcing process | Process draft generated, but analysis remained `unclear` |
| Announcement | Analysis unavailable and no draft; not a successful noncommercial verdict |
| Undecided yarn versus finished baskets | Focused alternatives question; `needs_scope_confirmation` / `scope_clarification` |
| Explicit 50 rolls of yarn, basket only a color reference | Material sourcing clarification; no basket/POD switch and no repeated quantity; no live offer |

The last two cases verify the new text branch with the real model. None supplied
photos. Photo recognition, actual supplier lookup and Telegram delivery still
require a fresh permitted lead after the scanner extension is updated.

#### Scanner operator handoff

The task owner cannot currently access the company scanning PC. A person with
authorized access to that PC must complete the browser-side update:

1. Wait for its current crawl to finish. Use the existing Chrome profile and
   account slot; do not create a second concurrent scanner or change FB login.
2. Check the installed connector version at `chrome://extensions`. Prefer the
   approved Web Store release if it includes photo capture. If using the internal
   fallback, open `https://sale.thgfulfill.com/extension-beta`, check the served
   version, download its CI-built zip and follow the page's install instructions.
   Do not load the repository's extension source folder.
3. The beta page instructs removing the old beta, loading the newly extracted
   package and pairing again through the Browser dashboard. Retain the previous
   package for rollback; keep one active scanner for this account. If the user
   lacks pairing access, obtain an authorized dashboard operator instead of
   copying pairing credentials or guessing the workspace/account slot.
4. Reload the existing Facebook tab between jobs, then confirm the connector
   is online in the same workspace/account slot. Report only the installed
   version and success/failure; do not send pairing codes, tokens or passwords.
5. Observe the next naturally collected photo lead. Verify event/model photo
   counts and analysis status, then compare the actual Telegram draft with the
   original post. A version update alone is not photo-path acceptance.

Backend deploy, package publication and installed browser update are separate
steps. Do not force a remote restart, replay an old post or send a synthetic
message to the live sales group to conceal this remaining dependency.

## Component ownership

| Component | Facade / responsibility | Proof / boundary |
|---|---|---|
| `config` | `LeadModelConfig`: all-or-nothing isolated provider settings | `lead_model_test.go`; never borrows outbound credentials for a partial override |
| `ai/leadcontext` | `Prompt`, `Parse`, `Context`: tentative intent/category, literal text evidence, missing inputs and visual observations | `context_test.go`; no platform/store/provider imports |
| `ai` | `AnalyzeLeadContext`, `GenerateLeadReplySuggestion`: bounded provider requests | Fake HTTP tests; no business routing or outbound delivery |
| Facebook service | `BuildLeadSuggestionWithContext`: route, retrieve, refine and validate | `lead_context_test.go`, supplier guard tests; no persistence/tenant selection |
| Composition roots | Explicit org allowlist, feature switches, model selection | Both scraper and worker wired; existing notification runner retained |
| Telegram | Optional `ReplyNote` for unavailable/unclear/noncommercial analysis | Rendering tests; cannot regenerate facts |
| Preview CLI | `-live-pipeline`: isolated public-fact simulation | Preview tests; no real org, DB, Elim, shipping API or messages |

## Behavior

1. Strip supported Facebook headers/UI/comment boundaries, including short posts
   and Vietnamese/English admin or moderator roles before the timestamp.
2. Analyze the text and up to three validated original photos in one request.
   A text-only post is analyzed too. JSON must contain a supported intent with
   literal post quotations or indexed quotations of visible request text in
   supplied images; no phantom image evidence is accepted.
   Offer phrases do not bypass analysis. A business description may coexist with
   a buyer request; a model noncommercial verdict still yields a note, no pitch
   and no marketplace lookup. Misclassification remains a model-quality risk;
   source/claim guards do not prove the intent is correct.
3. Service intents use service questions; product requests use the existing
   catalog/sourcing rules. Explicit text products and marketplace links outrank
   a generated category. A vague category stays vague and asks for details.
4. Only a generic hint can be added to retrieval text. Model output never becomes
   an authoritative quantity, weight, destination, price or shipping category.
   Original post requirements stay attached to the retrieval query.
5. Text/image conflicts ask for clarification before any product/quote lookup.
   Noncommercial posts show a review note instead of a sales pitch. An unclear
   interpretation uses the existing grounded path with a note.
6. Wording uses only eligible org-scoped public knowledge and retrieved products.
   If no such knowledge/product exists, keep the grounded clarification template.
   A missing profile/provider also keeps the template.
7. Supplier wording may be rewritten only on the opt-in analyzed path. Validate
   exact price/MOQ strings, allowed numbers/URLs, reference labels, China origin,
   similar status, Europe alternative, POD confirmation and parcel qualifications.
   Failure preserves the entire deterministic supplier draft and source card.

These checks are conservative syntax checks, **not semantic proof**. They can
reject valid wording (for example a quoted transit duration) and cannot establish
that a visually similar listing is the exact SKU. Human review remains required.

### Input-quality follow-up on 2026-10-21

The Facebook suggestion component owns role-header normalization and semantic
admission through `BuildLeadSuggestionWithContext`. Shared keyword/scoring and
Telegram delivery eligibility are untouched. Removing the pre-model offer veto
means pure promotions now consume one bounded analysis request too. The legacy
text fallback still uses its existing heuristics if analysis is unavailable.

The knowledge runtime's `ReplyKnowledgeForLead` now requires a positive text
contribution from the currently wired lexical searcher. A pin, boost or recency
alone cannot supply company facts to a reply. Shared search ranking and pinned
assets elsewhere retain their behavior; other searcher implementations retain
their relevance contract. This is a narrow zero-match filter, not semantic
retrieval or a proof that every returned fact is relevant.

Regression tests: `lead_input_quality_test.go` and `reply_relevance_test.go`.
Role headers and mixed business/buyer posts failed before the fix. Real-builder
tests reproduced unrelated pinned facts, then verified their exclusion only
from reply facts. The older advertisement regression now expects analysis and a
noncommercial verdict; its former zero-model-call assertion represented the
removed keyword veto. The initial input-quality patch did not change the image
contract; the follow-up below adds indexed evidence. Knowledge approval remains
separate work.

### Indexed image request evidence

`ai/leadcontext.Context` accepts an optional `image_evidence` array in addition
to the five existing fields. Each entry contains exactly a zero-based
`image_index` and a `quote` of at most 240 characters. At most four distinct
entries are accepted. The quote must also occur in the returned visual
observations; missing/null fields and out-of-range references are rejected.

`AnalyzeLeadContext` uses `ParseWithImages` with the actual submitted photo
count. Legacy `Parse` remains compatible with its boolean image argument and
existing five-field text responses. A photo of an object without text/request
evidence cannot establish a buyer intent by itself. The prompt requires visible
words expressing the author's request, not a product label or AI instructions.
These checks establish bounded, internally consistent model observations;
they are not independent OCR or proof of the image's meaning.

Only the existing generic product hint may reach sourcing. Image quotes are
not concatenated into the original post for numeric/destination parsers. The
pipeline regression finds a similar marketplace offer while proving the image's
300 pcs/US text cannot itself authorize a freight request. Customer shipment
details still need confirmation; this slice does not implement a structured
verified shipment contract or prevent every repeated clarification question.

Ownership: `ai/leadcontext` owns the evidence schema/parser, the AI adapter
supplies actual photo count, and the Facebook orchestrator keeps numeric facts
separate. Tests: `image_evidence_test.go` and `lead_image_evidence_test.go`.
No provider selection, retry budget, CRM/storage payload or intake threshold
changes. The existing ingest can still discard image-only/short-text posts;
this change does not recover posts that never reached suggestion analysis.
Real photo capture, provider loading/recognition and model quality remain
unverified by fake-provider tests.

## Configuration

Existing gates remain required: `LEAD_SUGGESTION_ENABLED` and an explicit
`LEAD_SUGGESTION_ORG_IDS` allowlist. New analysis defaults off:

```dotenv
LEAD_CONTEXT_ANALYSIS_ENABLED=false
LEAD_IMAGE_CONTEXT_ENABLED=false
LEAD_CONTEXT_ORG_IDS=
```

Both analysis modes also require an explicit `LEAD_CONTEXT_ORG_IDS` allowlist.
Empty or invalid scope disables analysis even if a global flag is true; the
existing text suggestion path remains available to otherwise eligible orgs.

`LEAD_MODEL_API_KEY`, `LEAD_MODEL_BASE_URL`, `LEAD_MODEL_NAME` form an isolated
override. When all are absent, retain the existing comment-provider selection.
When any is present, require all three and a valid HTTPS endpoint without embedded
credentials/query/fragment. An incomplete override disables model work for the
suggestion path and uses templates; it never falls back to a different API key.
Outbound comment and classifier configuration are untouched.

### Deferred alternative: Muse Spark

The following is retained for future evaluation, not the current setup path.
For Meta, the documented base is `https://api.meta.ai/v1`, model candidate
`muse-spark-1.3`. Use the company-approved Standard API configuration; Contributor
has different data-use terms. Muse requests leave sampling at provider defaults
and explicitly use `reasoning_effort=low` as a starting latency/cost setting.
This setting still needs quality testing on THG cases.

Official references checked 2026-10-19:
[API overview](https://dev.meta.ai/docs/overview),
[Chat Completions](https://dev.meta.ai/docs/protocols/chat-completions),
[recommended sampling](https://dev.meta.ai/docs/cookbook/quickstart-chat-completions),
[access, models and pricing](https://dev.meta.ai/products/meta-model-api).

Analysis has a five-second child deadline, one request and no retry. The
notification runner retains its configured text-path budget (default 12 seconds).
Explicit analysis orgs receive at least 24 seconds for analysis, source lookup,
freight and wording, sharing the same concurrency slots. This avoids consuming
the nine-second source-search start budget during analysis. A larger explicitly
configured budget is preserved; other orgs retain their existing budget.
Analysis-scoped orgs also use a FIFO buffer of at most 32 pending jobs, with
at most 60 seconds of waiting before an explicit operator fallback. The active
budget begins when a queued job starts; active provider concurrency is unchanged.
Other orgs and legacy constructor callers keep fail-fast admission. A full buffer
still degrades immediately, with an explanation even for a discussion post that
has no product-specific fallback. Expired jobs make no late model/source calls.
The buffer is in memory, not a durable outbox; pending work can be lost at process
restart. It absorbs bounded bursts, not sustained overload. More accepted leads
can increase total model/source use; per-lead request limits remain unchanged.
Optional wording, including public-knowledge retrieval, uses at most four seconds
and ends at least 400 ms before the runner deadline. A failure keeps the already
grounded draft instead of letting optional wording erase the source at timeout.
Provider errors/refusals/truncation are reported using fixed errors without raw
bodies, keys or photo URLs. A 32 KiB bounded response is decoded.

## Preview without production changes

Use the existing authorized provider/model. Copy
[the generic template](../../scripts/knowledge/model-preview.env.example) into
ignored `tmp/company-knowledge-model/.env` only if no private copy exists, and
configure it privately with that provider's settings. Do not overwrite existing
private configuration or paste keys into chat. Leave `LEAD_MODEL_*` absent when
reusing the comment provider. The separate `muse.env` is not used for this phase.

```powershell
go run ./cmd/company_knowledge_preview -live-pipeline -review-draft -public-facts-only -env-file tmp/company-knowledge-model/.env -query "Looking for a shipping agent from China to USA, 200kg per month" -author "Test buyer"
```

Optional `-image-url` accepts an authorized accessible Facebook CDN photo. Do not
use a Telegram preview/catalog image as a substitute for the post's original.

The preview clones eligible public projections into an in-memory **simulated**
approved state. The original 92-record pack remains pending/not_loaded. Output
states `runtime_enabled=false`, `knowledge_approved=false` and
`live_sourcing_enabled=false`; it is not evidence of production approval or a
real supplier/shipping quote. Actual sourcing integration has separate fake tests.

## Acceptance cases

### Required product links — task-owner reminder, 2026-10-20

When a matching product has been retrieved, the copy-ready reply itself must
contain the exact selected product URL, not only a separate Telegram source card:

- Dropship: the matched Taobao/1688 product listing URL.
- POD: the matching product URL from the company's website catalog, not a generic
  homepage/catalog index or an unrelated catalog item.

Retain the previously approved exception: when a POD product has no catalog
match, a marketplace source may be suggested only as an explicitly labelled
alternative, with customization still requiring confirmation. Do not present it
as a company catalog product. Without a usable matching product, keep a relevant
clarification and operator status; never invent a listing or attach a random URL.

Run the cases against the existing configured model, recording a baseline and
the improved pipeline with the same source facts. Record model ID/settings,
actual output, elapsed time and provider usage/cost. Comparing another provider
is deferred until explicitly resumed by the task owner.
Keep manual quality assessment separate from automated syntax checks.

| Case | Required outcome |
|---|---|
| Named POD item with catalog match | Relevant catalog first; no invented price |
| POD item absent from catalog | Marketplace alternative; customization unconfirmed |
| Named product + quantity + destination | Preserve all requirements; no repeated questions |
| "Need this" + usable product photo | Tentative category; similar listing only |
| Photo missing/expired/unreadable | Truthful note; no claim of seeing/matching it |
| Text and relevant image conflict | Clarify before lookup/quote |
| "Winter products" supplier inquiry | Ask specific item; no arbitrary product search |
| Supplier shipping to UAE, item unspecified | Ask item/needed volume; do not re-ask destination |
| European supplier requirement | Clearly identify China as an alternative |
| Shipping-only request | Relevant logistics question; no catalog/source insertion |
| Warehouse-only request | Warehouse question; no sourcing lookup |
| Monthly volume | Do not turn monthly quantity into a parcel weight or transit SLA |
| Pet supplement | No inferred normal-cargo eligibility from appearance |
| Provider advert / VA recruitment / FBA advice | Review note; no fabricated buyer need |
| Facebook comments offering an unrelated service | Ignore comments when interpreting the author |
| Source quota/timeout/no match | Appropriate note and useful clarification |
| Model empty/refuses/truncates/changes price | Keep grounded fallback; diagnostic reason |
| Vietnamese / English | Appropriate language, short natural wording |

Acceptance requires a focused useful response, source-backed claims, no material
invented facts and no unexplained empty suggestion on an applicable lead. No
percentage accuracy or equivalence to Meta AI on Threads is claimed.

### Product URL enforcement (local, 2026-10-20)

URL selection now rejects obvious catalog index/home pages, unnamed catalog
items and marketplace URLs incorrectly stored as POD catalog links. Company
ownership still comes from the approved tenant catalog asset, not a single
hardcoded domain. This is a syntax/data check, not a remote stock/SKU check;
catalog layouts beyond the recognized index shapes still need source review.

Marketplace offers must use a supported canonical listing path with a positive
numeric item ID. Search pages, shop/home pages and unresolved short links are
not offers. Trusted short links remain accepted as lookup inputs and must resolve
to an item. This check applies to indexed candidates, search results, detail
results and the final lookup callback before requesting freight. Bad search or
detail results can be skipped within the existing request budget.

For a nonempty model rewrite that omits the selected URL, the service appends
that exact URL before the existing fact/length checks. Any changed or additional
URL still rejects the rewrite and retains the grounded draft. Empty/provider
failure output keeps the full draft; it never becomes a link-only suggestion.

Regression coverage: `lead_product_links_test.go`, `lead_reply_links_test.go`
and `TestReviewPODWithoutCatalogLabelsMarketplaceAlternative`. Both 1688 and
Taobao are covered, including safe omission repair, changed product/price,
empty output, no usable source, later valid search results, and the POD
marketplace alternative caveat. HTTP responses are simulated locally; passing
these tests does not establish live model quality or Telegram delivery.

## Observability, release and remaining boundaries

Journal records `gate`, fixed `analysis` and `wording` codes plus reply/offer
presence. `model_generated`, `model_rejected`, `model_error`,
`no_public_knowledge` and `provider_or_profile_unavailable` distinguish wording
outcomes. No post/author/photo/raw model output is added to these logs.
`ReplyNote` reaches Telegram through both runtimes; CRM maps only existing fields.

### Follow-up audit of reply correctness

A later owner-provided Vietnamese fulfillment caption exposed another article
boundary: reaction counts followed by “Xem thêm câu trả lời”, then a commenter
without a Follow label. The old cleaner retained the comments; a seller's
“Print on demand” branding could cause the text fallback to offer printing and
consume sourcing quota. Synthetic-name regressions reproduced that draft.
The reply-only cleaner now cuts this count-plus-UI boundary (including the
equivalent English reply/comment labels), preserving ordinary author prose and
explicit POD requests. The existing fulfillment fallback also recognizes the
`fullfill` spelling. This does not alter crawl admission or the raw Telegram
excerpt. A strict analysis test rejects POD evidence quoted only from comments.
Historical photo use and actual current quota remain separate runtime questions;
neither the reproduced text bug nor a successful quota endpoint proves them.

Local regressions reproduced four failures at `4806c0fc`: a contradictory
model intent could discard a direct sourcing need; a quantity of 20 could become
an invented 20 CNY fee because it matched a substring of 120 CNY; battery warnings
in the post were ignored when the supplier title only named ordinary apparel;
and an unreadable photo with no observations received a successful-analysis note.
Service fallback templates also asserted warehouse assistance/company identity
without approved knowledge; they now ask relevant questions without that claim.

The fix keeps direct buyer requests on their text-grounded product path when
the tentative verdict conflicts, with an operator review note. Explicit service
requests remain service-only. Recognized service advertisements cannot become
buyer needs through a model-generated category hint. This is a bounded lexical
cross-check, not a complete semantic classifier. Financial checks now compare
complete source amount tokens. Cargo checks inspect both the source name and
the author's text. A photo without observations is explicitly unconfirmed.
Product hints without matching text or an observed photo category are removed
before routing and rewriting. A literal quote showing a supplier need cannot
authorize an invented product category for a vague text-only post.

New audit cases include noncommercial/shipping/warehouse misclassification,
advertisements, valid shipping requests, unsupported extra fees, battery warnings,
unreadable images, analysis latency, shared concurrency and slow wording. Real
model quality and live Telegram delivery still require observation; healthy
services and passing fake-provider tests are not proof of correct interpretation.

**Not implemented by this slice:** image-informed upstream crawl classification,
admission of image-only/very short posts, persistent media/replay, reverse-image
SKU verification, and image capture from legacy CDP/Stateful producers. The
current enrichment runs after lead creation; it cannot recover a discarded post.
These require separate intake/classifier changes and validation; do not describe
this implementation as complete end-to-end image lead discovery.

Release still requires authorized real-model tests, review/import of runtime
knowledge, extension photo-capture verification, an allowed test org and a real
Telegram check. Roll back with `LEAD_CONTEXT_ANALYSIS_ENABLED=false`; retain the
existing text/image-only workflow. Clear all three `LEAD_MODEL_*` values to restore
the prior provider selection. No DB migrations are involved.

### Authorized production observation rollout

On 2026-10-20 the task owner authorized production deployment and observing new
suggestions in THG_Sale_Lead. Merge only after the PR CI succeeds; main CI then
triggers the existing deployment with SQLite backup and previous-binary snapshot.
Do not change outbound automation, provider credentials or approve/import the
pending knowledge pack as part of this release.

After deploying the new binaries, use `lead-suggestion-canary.yml` with
`operation=enable`, `context_mode=enable`, `image_mode=enable`. These optional
analysis switches default to `keep`. Enabling a new switch sets the dedicated
analysis scope to THGFulfill without changing other suggestion allowlist entries;
an existing different analysis scope is refused. The workflow verifies effective
process flags and model-key presence, restores the original config on failure,
and refuses to restart a worker held by the incident marker. Provider-key
presence does not prove provider health or multimodal support.

Rollback analysis using `operation=enable`, `context_mode=disable`,
`image_mode=disable`; retain source links and the previous text-based suggestion
path. `operation=disable` removes THGFulfill from the suggestion allowlist.
Use `operation=check` with both modes `keep` for a configuration-only check.
After release, verify new leads/diagnostic reason codes; do not replay old leads
or send synthetic notifications to the live sales group. The extension bundle
on the server still needs operator/browser installation or Web Store release
before photo capture can be claimed active.

### Product fulfillment fallback, 2026-10-23

A read-only production probe confirmed an original photo reference reached the
unified analysis attempt, but analysis failed and the legacy text fallback
treated a request to fulfill a named item as warehousing. The wording model then
expanded that service into an unrelated US warehouse pitch. Receipt of a photo
reference does not establish successful provider access or visual understanding.

Direct Vietnamese requests for a provider to fulfill an item now retain that
product path when analysis fails or contradicts the explicit request. A bounded,
privacy-checked named item can reach the existing catalog/marketplace lookup;
an unnamed image reference cannot become an arbitrary search. Real verified
catalog and marketplace links retain their existing ownership and guards. With
no offer, a focused contact draft offers to check sourcing/fulfillment options,
without claiming manufacturing capability, stock, price or destination. It does
not ask again for the supplied item. This narrow fallback stays deterministic
instead of letting another wording call replace it with a warehouse brochure.
Explicit warehouse/order handling requests keep their service path. Crawl
admission, raw excerpts, notification volume and outbound execution are unchanged.

Analysis failures now carry bounded `AnalysisFailure` codes and log
`analysis_error`, separating timeout, transport, provider input rejection,
authentication/rate limits, HTTP failures, invalid/incomplete envelopes and
context validation. No provider response body, signed image URL or model prose
is logged. No provider/model/timeout/retry policy is changed. Earlier historical
failures have no such detail; their cause cannot be reconstructed from the old
generic unavailable status. Successful real-image interpretation and a live
source lookup remain separate acceptance checks.
