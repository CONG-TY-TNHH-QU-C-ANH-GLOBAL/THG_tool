---
doc_type: engineering
status: draft
owner: platform
last_reviewed: 2026-10-20
related_pr_or_issue: Add Taobao sourcing to replies
---

# Lead reply audit after production activation

## Scope and evidence

Reviewed source at deployed `4806c0fc` and the text/image analysis, grounding,
source selection, freight input, optional rewriting and notification paths.
Read-only production diagnostic `39379142536` completed at 06:13:43 UTC:
scraper/worker active, suggestions enabled for THGFulfill, Pricing Hub ready,
zero suggestion events in the preceding 20-minute window. This establishes no
live model quality, actual photo delivery or successful Telegram draft.

The new fixes/tests are on `fix/lead-reply-context-audit`. Do not infer deployment
from this document. No production messages, source requests or private data were
used in the local tests; HTTP model responses are simulated.

## Confirmed findings

| Priority | Finding at the reviewed commit | Correction / regression |
|---|---|---|
| P1 | A valid but wrong `no_commercial_need`, shipping or warehouse verdict could discard a direct product-supplier request. Literal text quotations do not prove interpretation. | Direct requests with a concrete product and no explicit service-only intent use the text-grounded path on conflict, with an operator note. `TestAuditModelCannotSilenceExplicitProductSourcing`; real shipping and service advertisements are covered separately. |
| P1 | A vague text-only `winter products` post could trigger a search for model-invented `winter gloves`; the quoted evidence supported a supplier need, not that product. | A hint must match the text or the observed photo category. Clear unsupported product hints before routing and rewriting, with an operator note. `TestAuditTextOnlyVaguePostCannotInventProductCategory` failed before the fix. The old positive `this item` fixture was corrected to include mocked photo evidence. |
| P1 | Rewriting a cup or hand massager into a hoodie passed as long as the original catalog/1688/Taobao URL and price remained correct. | Require selected product wording using existing product matching, excluding URLs from the check; otherwise retain the grounded draft. Wrong-product rows in `TestPODReplyPreservesSelectedCatalogLink` and `TestDropshipReplyPreservesSelectedListingLink` failed before the fix. |
| P1 | `20 CNY` could become an extra fee when quantity 20 was in the post and source price was `120 CNY`; substring matching accepted it while retaining the original price. | Compare complete currency/amount tokens with the source, not substrings. Positive controls preserve genuine amounts. `TestAuditMoneySubstringCannotAuthorizeExtraFee`. |
| P1 | A generic supplier title `Áo hoodie` could select standard cargo even when the post explicitly said `dùng pin`. | Inspect source title and original request before standard-apparel classification. Unknown cargo remains for the rate service to check. `TestAuditPostBatteryWarningPreventsStandardCargo`. |
| P2 | Successful JSON analysis with no photo observations still got a successful-photo-analysis note. | An image without observations is explicitly unconfirmed. `TestAuditUnreadablePhotoCannotBeReportedAsRead`. |
| P1 | Default 12-second total budget conflicted with a possible 5-second analysis and a 9-second source-search start requirement. Optional wording could also consume the outer deadline and discard an already-found product. | At least 24 seconds only for enabled/scoped analysis orgs; same shared slots and request-count limits. Public retrieval/wording capped at 4 seconds with 400 ms reserved before the parent deadline; preserve grounded facts on failure. Tests reproduce starvation after 3.3-second analysis under the old budget, then verify the scoped policy and slow-wording fallback. |
| P1 | A no-knowledge service template named THG and claimed warehouse/fulfillment assistance without approved company facts, including for other tenants. | Templates ask for relevant missing information without asserting company identity or service capability. Approved public knowledge may still ground model wording. `TestAuditServiceFallbackDoesNotInventCompanyCapabilities`. |

Targeted runs against the reviewed code reproduced the intent, unsupported
product hint, wrong-product wording, money, cargo and photo-note defects before
their fixes. The budget test includes an explicit legacy control
that reproduces starvation; it does not simulate a successful live marketplace.

## Boundaries preserved

- Supplier links remain canonical 1688/Taobao listings; POD keeps catalog links.
- POD marketplace alternatives retain source/customization qualifications.
- Source request count, quota-stop behavior and concurrency ceiling are unchanged.
  Longer scoped deadlines can allow more of the existing bounded fallback path
  to finish and can occupy a slot longer under load; saturation still degrades
  to the operator fallback.
- Default and other-org timing remains unchanged. Invalid/empty analysis scope
  stays closed. Larger explicitly configured budgets are preserved.
- CRM uses the same explicit payload fields. No schema, migration, outbound
  automation, credentials or provider selection change.
- The draft knowledge pack remains pending/not_loaded. No private SOP approval
  or runtime import is implied.

## Remaining uncertainty

The direct-request check is deliberately lexical and narrow; it is not a full
semantic classifier. Syntax guards cannot prove product identity, service
eligibility or every natural-language claim. The image capture extension must
be installed/released, upstream filtering is still text-led, and image-only
lead admission, durable media replay and exact-image SKU verification remain
outside this implementation. Review of real new leads is still required.
Product wording checks are conservative: a translated or heavily paraphrased
name can retain the grounded draft instead of accepting a valid rewrite. This
is not exact SKU validation or proof that all descriptive claims are supported.

## Validation commands

```text
go test ./internal/services/facebook ./internal/notifications ./internal/config ./internal/ai ./internal/ai/leadcontext ./internal/server/agent/crawlingest ./internal/telegram/render ./internal/crmleadsync -count=1 -timeout=120s
go test ./internal/services/facebook -count=1 -timeout=120s
go vet ./internal/services/facebook ./internal/notifications ./internal/server/agent/crawlingest ./cmd/scraper ./cmd/worker
go build ./cmd/scraper/ ./cmd/worker/
```

All four commands above passed locally, including the final Facebook rerun
and eight-package rerun after the product-hint and wording-identity changes.
Docs governance, whitespace and
supplementary changed-file size/same-core complexity checks also passed.

Go 1.26.8, workspace caches, `GOTOOLCHAIN=local`, `GOPROXY=off`; local HTTP
fixtures require execution outside the restricted loopback sandbox. Official
CI and Sonar remain the release gates. Local official preflight still requires
Python; supplementary size/complexity checks are not a substitute for CI.

## Release and rollback

After the checks succeed, release through a separate PR and the existing main
CI/deployment workflow. Preserve the active THGFulfill-only scope. A smaller
rollback disables both context/image flags through the canary workflow and
restores the text path/budget; disable them before restoring binaries predating
the scope fix. Record actual deployment and live observations in workspace state.
