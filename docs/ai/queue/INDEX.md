# Architecture Queue — Index (human map)

> **Source of truth = the item files** under `docs/ai/queue/items/` (discovered
> recursively). This page is only a navigable map; if it disagrees with an item
> file's frontmatter, the item file wins. Lifecycle/policy lives in
> [`docs/ai/AUTOPILOT_QUEUE.md`](../AUTOPILOT_QUEUE.md).
>
> **Layout:** items are grouped by **stable domain/component**, never by mutable
> status — `items/architecture/{workspace_knowledge,store,server,copilot,cmd_scraper}/`
> and `items/docs/`. Dependency resolution is by the `id:` frontmatter, so an
> item's folder never affects `depends_on`. (Pre-existing Facebook-track items
> PR31D/PR31E/PR32A remain at the `items/` root.)

Covers two epics: the **Self-Feeding Architecture Epic** (code decomposition —
topology scan of `internal/workspace_knowledge`, `internal/store`,
`internal/server`, `internal/drivers/copilot`, `cmd/scraper`) and the
**Self-Feeding Docs Architecture Epic** (docs/specs organization). Both generated
2026-06-26.

**Lane key** — GREEN: package-internal pure / file-responsibility cleanup, no
import-boundary or DB/auth/ledger/connector/queue/runtime change. YELLOW:
behavior-preserving move-only that crosses an import boundary (sequential by
deps — in Go a folder move *is* an import-boundary change). RED: audit-only,
`status: BLOCKED`, human decision required — never auto-implemented.

## At a glance

_Status reflects the queue after the last `/thg-next` auto-reconcile (verified via
GitHub PR `merged_at`). Item-file frontmatter is the source of truth._

- **Done (merged):** ARCHWK1, ARCHCM1, ARCHCP1, DOCS1 (+ legacy PR32A).
- **Active item:** none in progress.
- **Next executable READY:** [ARCHCP2](items/architecture/copilot/ARCHCP2-agent-preflight-split.md) (GREEN).
- **Other executable GREEN (no unmet deps):** [ARCHWK2](items/architecture/workspace_knowledge/ARCHWK2-products-canonical-split.md),
  [ARCHWK3](items/architecture/workspace_knowledge/ARCHWK3-retrieval-helpers-rename.md),
  [ARCHST1](items/architecture/store/ARCHST1-store-test-fallback-migration.md),
  [ARCHSV1](items/architecture/server/ARCHSV1-crawl-direct-post-helper-extract.md),
  [DOCS2](items/docs/DOCS2-architecture-doc-backlinks.md).
- **Executable YELLOW (no unmet deps):** [ARCHSV2](items/architecture/server/ARCHSV2-agent-finalize-subpackage.md),
  [ARCHWK4](items/architecture/workspace_knowledge/ARCHWK4-soak-internal-grouping.md),
  [DOCS-Y1](items/docs/DOCS-Y1-relocate-pr-checklist.md),
  [DOCS-Y2](items/docs/DOCS-Y2-governance-frontmatter-backfill.md).
- **Blocked RED (audit-only):** ARCHST-R1, ARCHST-R2, ARCHST-R3, ARCHSV-R1, ARCHCM-R1, ARCHCM-R2, DOCS-R1, DOCS-R2.

## How to resume

Run **`/thg-next`** — it pulls latest `main`, runs `scripts/ai_preflight.sh`, reads
`docs/ai/AUTOPILOT_QUEUE.md` + the item files, and picks the first **executable
READY** item (all `depends_on` DONE). It will not pick a `BLOCKED` (RED) item.
GREEN sprint mode runs only `parallel_safe: true` items with no unmet deps. RED
items need `/thg-red-audit` and a human decision. One PR per item; never merge.

---

## internal/workspace_knowledge

### GREEN implementation
| Item | Status | Summary |
|---|---|---|
| [ARCHWK1](items/architecture/workspace_knowledge/ARCHWK1-governance-output-validation-split.md) | DONE | Split governance Layer-3 validator into per-check files (banned/guarantee/shipping/pricing). |
| [ARCHWK2](items/architecture/workspace_knowledge/ARCHWK2-products-canonical-split.md) | READY | Split `products/canonical.go` by responsibility (canonical / availability / pricing / normalize). |
| [ARCHWK3](items/architecture/workspace_knowledge/ARCHWK3-retrieval-helpers-rename.md) | READY | Replace vague `retrieval/helpers.go` with responsibility files (tokenize / scoring / query). |

### YELLOW implementation
| Item | Status | Deps | Summary |
|---|---|---|---|
| [ARCHWK4](items/architecture/workspace_knowledge/ARCHWK4-soak-internal-grouping.md) | READY | — | Decompose the 12-file `soak` package (sibling grouping vs subpackages). |

---

## internal/store

### GREEN implementation
| Item | Status | Summary |
|---|---|---|
| [ARCHST1](items/architecture/store/ARCHST1-store-test-fallback-migration.md) | READY | Move top-level cross-domain `*_test.go` fallbacks into owning subpackage external tests. |

### RED audit-only / blocked
| Item | Status | Summary |
|---|---|---|
| [ARCHST-R1](items/architecture/store/ARCHST-R1-append-only-ledger-audit.md) | BLOCKED | Audit append-only ledger UPDATE violations (action_ledger / engagement_reconcile). |
| [ARCHST-R2](items/architecture/store/ARCHST-R2-connector-lease-cas-audit.md) | BLOCKED | Audit connector pairing lease / CAS consistency vs reverify lease. |
| [ARCHST-R3](items/architecture/store/ARCHST-R3-direct-post-boundary-audit.md) | BLOCKED | Audit direct-post lookup boundary (leads ↔ coordination ownership). |

---

## internal/server

### GREEN implementation
| Item | Status | Summary |
|---|---|---|
| [ARCHSV1](items/architecture/server/ARCHSV1-crawl-direct-post-helper-extract.md) | READY | Extract pure crawl direct-post outcome-classification helpers. |

### YELLOW implementation
| Item | Status | Deps | Summary |
|---|---|---|---|
| [ARCHSV2](items/architecture/server/ARCHSV2-agent-finalize-subpackage.md) | READY | — | Extract `internal/server/agent/finalize` subpackage (move-only, CAS-adjacent). |
| [ARCHSV3](items/architecture/server/ARCHSV3-agent-crawl-ingest-subpackage.md) | READY | ARCHSV2 | Extract `internal/server/agent/crawl_ingest` subpackage. |
| [ARCHSV4](items/architecture/server/ARCHSV4-agent-outbox-subpackage.md) | READY | ARCHSV2 | Extract `internal/server/agent/outbox` subpackage (preserve dashboard JSON). |

### RED audit-only / blocked
| Item | Status | Summary |
|---|---|---|
| [ARCHSV-R1](items/architecture/server/ARCHSV-R1-workspace-orchestration-audit.md) | BLOCKED | Audit workspace browser-orchestration split (runtime state; auth-adjacent). |

---

## internal/drivers/copilot

### GREEN implementation
| Item | Status | Summary |
|---|---|---|
| [ARCHCP1](items/architecture/copilot/ARCHCP1-agent-brain-split.md) | DONE | Split 525-line `agent_brain.go` into client / types / validator / action-prep siblings. |
| [ARCHCP2](items/architecture/copilot/ARCHCP2-agent-preflight-split.md) | READY | Split `agent_preflight.go` (account readiness vs business-context inference). |

### YELLOW implementation
| Item | Status | Deps | Summary |
|---|---|---|---|
| [ARCHCP3](items/architecture/copilot/ARCHCP3-intent-subpackage.md) | READY | ARCHCP1, ARCHCP2 | Extract `copilot/intent` subpackage (no cross-cluster cycle). |
| [ARCHCP4](items/architecture/copilot/ARCHCP4-agent-subpackage.md) | READY | ARCHCP3 | (Optional) extract `copilot/agent` subpackage — only if still tripping the file-count trigger. |

---

## cmd/scraper

### GREEN implementation
| Item | Status | Summary |
|---|---|---|
| [ARCHCM1](items/architecture/cmd_scraper/ARCHCM1-action-args-split.md) | DONE | Split `action_args.go` (pure coercion vs domain-aware helpers). |

### YELLOW implementation
| Item | Status | Deps | Summary |
|---|---|---|---|
| [ARCHCM2](items/architecture/cmd_scraper/ARCHCM2-outbound-pipeline-move.md) | READY | ARCHCM1, ARCHCM-R1 | Move leaked outbound pipeline out of cmd into `internal/outbound`. |
| [ARCHCM3](items/architecture/cmd_scraper/ARCHCM3-direct-post-intake-move.md) | READY | ARCHCM2, ARCHST-R3 | Move direct-post intake service/scheduler into `internal/directpost`. |
| [ARCHCM4](items/architecture/cmd_scraper/ARCHCM4-crawl-runtime-move.md) | READY | ARCHCM-R1, ARCHCM-R2 | Move crawl runtime/plan/scheduler into `internal/crawler` + `internal/jobs`. |

### RED audit-only / blocked
| Item | Status | Summary |
|---|---|---|
| [ARCHCM-R1](items/architecture/cmd_scraper/ARCHCM-R1-account-scope-consolidation-audit.md) | BLOCKED | Audit + consolidate duplicated account-control RBAC (security). |
| [ARCHCM-R2](items/architecture/cmd_scraper/ARCHCM-R2-crawl-runtime-semantics-audit.md) | BLOCKED | Audit crawl runtime / connector dispatch / fallback semantics. |

---

## Dependency order (main chains only)

GREEN items with no `depends_on` are omitted — they are independently executable.
RED audits gate the cmd/scraper move-out chain.

```mermaid
graph LR
  %% copilot: reduce in-package complexity, then move boundaries
  CP1[ARCHCP1 · GREEN] --> CP3[ARCHCP3 · YELLOW]
  CP2[ARCHCP2 · GREEN] --> CP3
  CP3 --> CP4[ARCHCP4 · YELLOW]

  %% server agent: finalize settles before its siblings
  SV2[ARCHSV2 · YELLOW] --> SV3[ARCHSV3 · YELLOW]
  SV2 --> SV4[ARCHSV4 · YELLOW]

  %% cmd/scraper: helper split + RED audits gate the domain move-outs
  CM1[ARCHCM1 · GREEN] --> CM2[ARCHCM2 · YELLOW]
  CMR1[ARCHCM-R1 · RED] --> CM2
  CM2 --> CM3[ARCHCM3 · YELLOW]
  STR3[ARCHST-R3 · RED] --> CM3
  CMR1 --> CM4[ARCHCM4 · YELLOW]
  CMR2[ARCHCM-R2 · RED] --> CM4
```

---

## Docs Architecture Epic

Applies the same discipline to docs/specs: clear ownership, navigation, no orphan
docs, no duplicated source of truth, no AI-generated spec sprawl. See
[`../INDEX.md`](../INDEX.md) (top-level docs map) and
[`../DOCS_GOVERNANCE.md`](../DOCS_GOVERNANCE.md) (rules).

### GREEN implementation
| Item | Status | Deps | Summary |
|---|---|---|---|
| [DOCS1](items/docs/DOCS1-docs-architecture-map.md) | DONE | — | Add docs/ai + docs/architecture subtree indexes; fix the stale `specs/SPEC_INDEX.md` backlink in docs/INDEX.md. |
| [DOCS2](items/docs/DOCS2-architecture-doc-backlinks.md) | READY | DOCS1 | Backlinks + governance frontmatter for docs/architecture/*. |

### YELLOW implementation
| Item | Status | Deps | Summary |
|---|---|---|---|
| [DOCS-Y1](items/docs/DOCS-Y1-relocate-pr-checklist.md) | READY | DOCS1 | Move docs/PR_CHECKLIST.md → docs/engineering/ (update the AGENTS.md referrer). |
| [DOCS-Y2](items/docs/DOCS-Y2-governance-frontmatter-backfill.md) | READY | DOCS1 | Backfill governance frontmatter across docs/ in small staged batches. |

### RED audit-only / blocked
| Item | Status | Summary |
|---|---|---|
| [DOCS-R1](items/docs/DOCS-R1-spec-source-of-truth-audit.md) | BLOCKED | Spec source-of-truth: root `specs/` (tool-referenced) vs governed `docs/specs/`. Recommend keeping root canonical; human decision. |
| [DOCS-R2](items/docs/DOCS-R2-generated-artifact-location-audit.md) | BLOCKED | Generated `RETRIEVAL_SOAK_REPORT.md` location + soak-test write-gating (`-update`); test-code change, human decision. |
