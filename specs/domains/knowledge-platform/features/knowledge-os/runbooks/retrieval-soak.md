# Retrieval Substrate — Production Soak Runbook

**Purpose:** the operator-facing instructions for running the soak
harness against real production (PG + pgvector + OpenAI embeddings)
and interpreting the result.

**Goal:** ANSWER ONE QUESTION — is the retrieval substrate trusted
enough to build orchestration on top? READY = yes. DEGRADED = with
caveats. NOT_READY = block.

**Output:** the soak emits a [Report](../../../../../../internal/workspace_knowledge/soak/report.go)
with seven measurement groups, an operator-trust score, and the
auto-generated `artifacts/retrieval-soak/RETRIEVAL_SOAK_REPORT.md` (gitignored, regenerated each run)
committed alongside this runbook. Re-run on every release.

---

## What the soak validates (vs goal directive)

| Goal § | Requirement | Where the soak proves it |
|---|---|---|
| §1 Real traffic observability | hit rate, fallback rate, zero-hit rate, semantic score, budget drop, compliance blocks | `Report.Quality` + `Report.FallbackBehaviour` + `Report.PromptOutcomes` |
| §2 Replay auditability | every retrieval has a complete trace | `Report.ReplayHealth.CompletenessRate` (target ≥ 0.95) |
| §3 Stale knowledge detection | expired pricing never surfaces | `Report.StaleDetection` + per-prompt `ComplianceLeaks` |
| §4 Embedding drift detection | distinct model versions in catalog | `Report.HarnessConfig.EmbedderModel` + soak-time embedding stats |
| §5 Failure mode validation | six scenarios A–F degrade gracefully | `Report.FailureModes` — all must PASS |
| §6 Cost telemetry | embedding token usage observable | embedded in `knowledge_events` (event_type=embedding_batch) — soak doesn't measure but verifies the substrate is in place |
| §7 Retrieval quality benchmark | hybrid-only vs RRF comparison | TWO soak runs: `SearcherVariant: "hybrid"` vs `"rrf"`; diff the Reports |

---

## Running the soak (test mode — every commit)

This is the default. The mock `ClusteredEmbedder` produces
deterministic vectors; CI re-runs on every PR.

```bash
go test -v -run TestSoak ./internal/workspace_knowledge/soak/...
```

The test writes `artifacts/retrieval-soak/RETRIEVAL_SOAK_REPORT.md`
on every run so reviewers can diff retrieval-quality across PRs
before merge. Failing tests block merge — the soak is treated as
a quality gate.

**Pass criteria** (hard-coded in `harness_test.go`):
- Zero compliance leaks across all prompts.
- Zero hidden-state leaks.
- Replay completeness ≥ 95%.
- All six failure modes PASS.
- Operator trust verdict ≠ NOT_READY.
- ≥ 80% of catalog assets embedded after worker drain.

---

## Running the soak (production mode — every release)

This is the REAL validation. Same harness, real backends.

### 1. Prerequisites

- PostgreSQL 14+ with pgvector extension installed.
- `DATABASE_URL=postgres://...` set in env.
- `OPENAI_API_KEY=sk-...` set in env (for real embeddings).
- pgx driver imported in your binary (see `internal/store/postgres_driver.go`).

### 2. Build the prod-soak runner

Create `cmd/soak-runner/main.go` (one-shot template):

```go
package main

import (
    "context"
    "flag"
    "fmt"
    "os"

    "github.com/thg/scraper/internal/store"
    "github.com/thg/scraper/internal/workspace_knowledge/embedding"
    "github.com/thg/scraper/internal/workspace_knowledge/soak"

    _ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
    var (
        variant = flag.String("variant", "rrf", "searcher: hybrid|rrf")
        orgID   = flag.Int64("org", 7777, "tenant org_id for soak")
        outPath = flag.String("out", "soak-report.md", "where to write markdown")
    )
    flag.Parse()

    db, err := store.New("") // reads DATABASE_URL
    if err != nil { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
    defer db.Close()

    emb := embedding.NewOpenAIEmbedder(os.Getenv("OPENAI_API_KEY"), "")

    h := &soak.Harness{
        Store:           db,
        Embedder:        emb,
        Catalog:         soak.RealisticCatalog(),
        Prompts:         soak.RealisticLeads(),
        SearcherVariant: *variant,
        OrgID:           *orgID,
        TopK:            5,
    }
    report, err := h.Run(context.Background())
    if err != nil { fmt.Fprintln(os.Stderr, err); os.Exit(1) }

    if err := os.WriteFile(*outPath, []byte(report.ToMarkdown()), 0644); err != nil {
        fmt.Fprintln(os.Stderr, err); os.Exit(1)
    }
    fmt.Printf("Trust verdict: %s (%d/100)\n", report.OperatorTrust.Verdict, report.OperatorTrust.Score)
    if report.OperatorTrust.Verdict == "NOT_READY" {
        os.Exit(2) // CI exit code = block release
    }
}
```

### 3. Run

```bash
go run ./cmd/soak-runner \
    -variant rrf \
    -org 7777 \
    -out artifacts/retrieval-soak/RETRIEVAL_SOAK_PROD_$(date +%Y-%m-%d).md
```

Cost: ~17 embedding calls × text-embedding-3-small = ~$0.0003. Negligible.

### 4. Compare hybrid vs RRF

```bash
go run ./cmd/soak-runner -variant hybrid -out hybrid.md
go run ./cmd/soak-runner -variant rrf    -out rrf.md
diff hybrid.md rrf.md
```

If RRF's `Mean Precision@K` is < hybrid's by more than 0.05, do NOT
roll out the vector path. The pgvector index, embedding model, or
RRF tuning needs work first. This is the §7 benchmark gate.

---

## Reading the report

### Top of report — TRUST VERDICT

```
## Operator Trust: **READY** (score: 85/100)
```

- **READY (≥ 80)** — substrate trusted; orchestration can land on top.
- **DEGRADED (60–79)** — ship with caveats; instrument & monitor.
- **NOT_READY (< 60 OR blocking issues)** — fix issues; don't proceed.

### Per-prompt table

```
| Lang | Verdict | Score | P@K | Lat ms | Prompt |
| en   | ✅ PASS | 0.03  | 1.00 | 1     | Looking for custom cat tee POD … |
```

- **P@K** is the precision-against-intent proxy. Higher = retrieval
  actually surfaced relevant assets.
- **Score** is the raw RRF score (or hybrid score, depending on
  variant). Different scales — don't compare across variants.
- **Lang** distinguishes VI from EN prompts so you can spot
  language-specific regressions.

### Failure modes

Every scenario should be `✅ PASS`. A `❌ FAIL` is a HARD STOP:
- **A** Embedder API down → fallback must invoke
- **B** pgvector unavailable → hybrid serves transparently
- **C** Partial embeddings → hybrid covers the gap
- **D** Slow query → 1.5s timeout fires, fallback engages
- **E** Zero-asset tenant → empty result without error
- **F** Stale catalog → observability surfaces stale count

### Compliance section

```
## Compliance / Governance
✅ No banned claims surfaced. No hidden-state assets surfaced.
```

`🛑 COMPLIANCE VIOLATIONS` = the substrate is **broken**. Do not
ship. Investigate immediately — a banned-claim leak is a legal
incident in waiting.

---

## What to do when the soak fails

### NOT_READY verdict

Read `OperatorTrust.BlockingIssues` first. Each item points at one
of:
1. Compliance/hidden leak → bug in the governance filter at the
   lex/hybrid layer or the assembly drop path.
2. Replay completeness < 95% → bug in the trace-recording call site
   (someone added a Searcher that forgot to populate `SearcherImpl`).
3. Failure mode FAIL → the named scenario broke. Re-read its
   description in `failure_modes.go`.

### DEGRADED verdict

Read `OperatorTrust.WarningIssues`. Common ones:
- **Elevated fallback rate** — semantic searcher is unreliable.
  Check OpenAI API health, PG load, network between app and DB.
- **Low mean Precision@K** — the catalog is poorly tagged, or the
  prompt fixtures don't match the catalog. In production this
  signals the operator should tag assets more aggressively.

### Stale-time validation

The default soak doesn't wait 30 days for staleness to develop. To
validate stale-detection in CI, manually backdate:

```sql
UPDATE knowledge_assets SET last_retrieved_at = NOW() - INTERVAL '60 days'
 WHERE org_id = 7777;
```

Re-run the soak; `Report.StaleDetection.StalePast30d` should reflect
the change.

---

## When to re-run

- Every PR that touches `internal/store/knowledge_*` or
  `internal/workspace_knowledge/retrieval/*` (CI runs the test soak
  automatically).
- Every production release (operator runs the prod-mode soak before
  cutting the deploy).
- After any embedding model change (`embedding_model_version` flip).
- After any HNSW index rebuild or pgvector extension upgrade.

---

## What this runbook does NOT cover

- Real-traffic A/B testing (production observation of live leads).
  The soak validates the SUBSTRATE; A/B validates the OUTCOMES.
- LLM output quality. The soak stops at retrieval — the assembly
  block is rendered and counted, but no actual LLM call is made.
- DOM execution downstream. That belongs to the browser-automation
  test suite.

These remain manual validation steps the team performs in
controlled rollouts AFTER the soak verdict is READY.
