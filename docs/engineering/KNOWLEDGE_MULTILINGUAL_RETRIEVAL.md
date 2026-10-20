---
doc_type: engineering
status: draft
owner: platform
last_reviewed: 2026-10-18
related_pr_or_issue: Add Taobao sourcing to replies
---

# Multilingual lexical retrieval v2

## Ownership and API

- Owner: `internal/workspace_knowledge/retrieval/hybrid`; Unicode normalization
  belongs to the shared retrieval component.
- API: `hybrid.NewMultilingual(lister)` implements the existing Searcher port.
  `hybrid.New` and `retrieval.Tokenize` retain v1 behavior for replay/comparison.
- Adoption: runtime `NewBuilder` and `NewBuilderWithVector` select v2 for their
  lexical path; the pgvector primary retains its existing behavior.
- Boundaries: no database/ingestion schema, provider, Facebook routing, outbound
  action or UI changes. The company draft pack remains pending and not loaded.

This is an additive behavior change on base `b2a3cf6d`, implemented in the local
worktree. This document does not confirm production deployment.

## Text scoring

1. Normalize case, separators and Latin accents, including Vietnamese đ. NFC and
   decomposed Vietnamese produce identical terms. Preserve other scripts' marks.
2. Use whole words of at least two runes and adjacent word pairs. Phrase matching
   uses word boundaries, so `M4` does not match `M40` or `case` match `phonecase`.
3. Compute document frequency on eligible titles/tags in the tenant's bounded
   candidate pool. Prose mentions do not dilute heading frequencies. Each term
   receives `1 + log(1 + N/(1 + df))` weight.
4. Query coverage gives title/tag matches full weight and description matches
   0.35 weight. Full normalized phrases retain the title 1.0 / description 0.5
   bonus. The text signal is the larger of coverage and phrase bonus.
5. Retain the existing final weights: text 0.50, boost 0.20, pin 0.20, recency 0.10.
   Sort query terms before summing for deterministic scores. Metadata/governance
   filters and top-k caps retain their existing contract.

Trace version is `hybrid-v2-multilingual`; phrase reasons say `normalized-phrase`.
The dependency `golang.org/x/text v0.36.0` already existed and is now direct; no
version upgrade or external search/LLM service was added.

## Evidence and limitations

See [evaluation dataset](../business/company-knowledge/retrieval-cases.json) and
[measured results](../business/company-knowledge/RETRIEVAL_EVALUATION.json).
Both versions run on the same pack and queries; hashes identify those inputs.
The 24 synthetic checks pass 17 with v1 and 24 with v2. The two warehouse queries
require rank 1; other checks require an expected record in the stated top-k, or
no result for unrelated terms. This is a development regression set, not a held-out
estimate or a production success rate.

The initial dataset incorrectly expected an internal catalog reference in public
facts mode. That case was corrected to review mode; the evaluator now rejects
ineligible expected IDs instead of counting access filtering as a ranking defect.

26 company records have curated search terms, including English equivalents.
The local preview maps these to existing asset Tags. These are search metadata,
not additional business claims or evidence that the draft has been imported.

This remains lexical retrieval: it does not translate arbitrary language, infer
intent/negation, segment CJK, verify images, prices, service eligibility or SLA.
Accent folding can merge distinct words; limited candidate pools and metadata
quality also affect ranking. Intent checks, business constraints, policy and
appropriate audience filtering remain necessary before generating a reply.

## Verification and rollback

Normalization tests cover Vietnamese forms and other scripts. Hybrid tests cover
ranking, prose/heading precedence, model boundaries, governance and score fields.
Runtime tests cover both builder constructors and propagation of org/state/source
health filters. Preview tests check pending/public behavior and all 24 cases.
Exact command results are in [validation](../business/company-knowledge/VALIDATION.md).
The standard preflight/cognitive guards could not finish locally (missing Python,
dependency download timeout). Supplementary measurement used the cached v1.1.3
CLI and unchanged complexity algorithm with only the x/tools Analyzer plugin
omitted; touched functions are <=15. This is not a claim that the official CI
guards have run. Input validation/indexing/scoring were separated, and the
existing metrics recorder loop was flattened without changing its behavior.

To restore v1 lexical behavior, change the two runtime constructors back to
`hybrid.New`; the preview also accepts `-legacy-lexical`. No data migration or
asset state change is required. Existing v1 traces retain their version.
