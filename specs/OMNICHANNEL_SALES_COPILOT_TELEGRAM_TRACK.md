# Omnichannel Sales Copilot + Telegram Intelligence Track

> **STATUS: ACTIVE (opened 2026-06-09, founder-directed).** Telegram is a
> PEER interface to the Web FE — not a side bot. Comment-quality prerequisites
> (PR-1, PR-2) first, THEN the Telegram interface. Separate track from FB
> Reliability / Comment Intelligence / Browser Kit.

## Foundational architecture invariant
Telegram is an **interface only**. Every Telegram command goes through the SAME
backend `ActionContext` → `PolicyGate`/Readiness → ledger as the Web FE. NO logic
copied from web to telegram. Business truth stays in `engagement_events` /
ledger. Learning is **append-only**. Telegram media stores **source/provenance**.
Sensitive actions still pass readiness/policy gate. Every error returns a **typed
reason code**. Parser / auth / command-routing have **tests**.

---

## PR-1 — Comment Quality Hotfix ✅ SHIPPED (ec94a77, ext 0.5.35)
Fixed the duplicated-comment bug ("…nhé.Bên em…nhé."). Investigation: GenerateCommentV2
= single LLM call; queue path single-pass; UI single-render — duplication is an LLM
artifact or FB draft re-mount. Two nets: `internal/ai.SanitizeComment` (dedupe
repeated sentences/paragraphs + reject empty/over-length/anonymous-salutation as
`comment_quality_invalid`, applied at the queue boundary) + extension
`editorTextDoubled` guard (re-clear+re-insert once before submit, else abort).

## PR-2 — GenerateCommentV2 Depth Upgrade (NEXT)
Comment must bind to post context using GROUNDED assets: `sales_playbook · cta ·
pricing · catalog SKU · product price`. Product-seeking lead → cite real SKU/price
when available. Service-seeking lead → capability/proof/CTA from service knowledge.
NEVER fabricate price/proof. `knowledge_gap=true` → generic fallback that is still
specific, not bland/repetitive. Quality guard: no duplicate sentence, no duplicate
CTA, max length, no unsupported claim, no anonymous salutation (PR-1 SanitizeComment
covers most; PR-2 adds grounding-aware checks). Log selected assets + reasoning.

---

## Telegram track (designed AFTER PR-1/PR-2)

### T1 — Auth / Binding
`telegram_user_id` ↔ org user; `telegram_chat_id` ↔ workspace; RBAC reuses the
existing role model. Binding via a one-time code (like connector pairing).

### T2 — Command Gateway
`/status · /accounts · /comment_all_leads · /crawl · /inbox · /post` + inline
buttons. EVERY command resolves the shared `ActionContext` (initiator, account,
connector, readiness) — the SAME path the web/agent actions use. No bespoke logic.

### T3 — Notification
Push: new lead · action queued/success/fail · actor mismatch · connector offline ·
crawl finished. Sourced from the ledger/events, not a parallel store.

### T4 — Learning Ingestion (append-only)
Sales sends a sample comment / product image / proof / pricing / FAQ → system
classifies into a `learning_event` → writes a knowledge asset pending/approved per
policy. Media stores **source/provenance** (who, when, telegram_file_id). AI learns
STYLE from real comments but never copies verbatim. Append-only — corrections emit
new events, never mutate.

### T5 — Parallel FE
Operate fully from Telegram without the website; Web FE + Telegram share the SAME
backend, ledger, and ActionContext. Neither is the source of truth — the ledger is.

## Definition of Done (track)
Telegram can receive leads, issue comment/post/inbox/crawl, show account status,
push success/fail, and ingest learning — ALL through the shared backend, with typed
reason codes, append-only learning, media provenance, and parser/auth/routing tests.
No web logic duplicated; the ledger remains the single source of truth.
