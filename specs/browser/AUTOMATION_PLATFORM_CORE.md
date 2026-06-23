# Automation Platform Core (P2/P3)

> **STATUS: SCAFFOLDING SHIPPED (additive, code-only).** Channel-neutral automation platform
> core + a Facebook adapter that WRAPS the existing working comment flow. **Zero behavior change
> to the Facebook hot path** — `content/outbound.js`, `comment_button.js`, `comment_composer.js`,
> and the insertion/dedup/submit state machine are untouched. This is the contract layer the
> deferred **P1 Vision fallback** and the **Browser Automation Kit extraction**
> (`specs/BROWSER_AUTOMATION_KIT_REFACTOR.md`) plug into. Track: KnowledgeOS/Platform — separate
> from FB Reliability, Comment Intelligence, and Telegram.

## Why additive (wrap, not extract)

The kit-refactor spec extracts `outbound.js` internals into `core/runtime/platforms`. That is a
larger, riskier move. The founder order after the P0/P0b live fix is: **stand up the channel
contracts first, wrapping the proven flow**, so Vision/Telegram/metrics build on generic seams
immediately — without re-touching a hot path that is finally green. Extraction can later move
`outbound.js` internals BEHIND these same seams, one mechanical PR at a time.

## Loading model (unchanged doctrine)

Classic content scripts + global namespaces, **no ES modules / no bundler**, manifest order =
architecture (same as the kit spec). Each file is an IIFE exposing one global.

- `THGAutomation.*` — platform CORE. Knows **nothing** about any channel's DOM/URLs/labels.
- `THGChannelFacebook.*` — the Facebook adapter. Owns ALL Facebook URL/label/selector specifics.

**Code-only for now:** these files are NOT yet listed in `manifest.json` `content_scripts`, so
they have zero production load (precedent: the kit spec's PR-5 skeletons are code-only). They are
wired into the manifest in the PR that first consumes them (Vision fallback / kit extraction),
loaded in dependency order: `automation/adapter_contract → channel_registry → candidate_schema →
evidence_contract → action_context → diagnostics → metrics`, then `channels/facebook/*`, then the
existing `content/*` (so the adapter can delegate to `THGContentOutbound` etc. at call time).

## Platform CORE — `local-connector-extension/automation/`

| Module | Global | Responsibility |
|---|---|---|
| `adapter_contract.js` | `THGAutomation.AdapterContract` | The 8-method adapter contract + validate/assertValid |
| `channel_registry.js` | `THGAutomation.ChannelRegistry` | `register/get/has/list` channel→adapter; validates on register |
| `candidate_schema.js` | `THGAutomation.Candidate` | Generic DOM candidate (P3) — `create/fromElement/accept/reject` |
| `evidence_contract.js` | `THGAutomation.Evidence` | Bounded evidence envelope; forbidden-key redaction (no secrets/full DOM) |
| `action_context.js` | `THGAutomation.ActionContext` | Neutral carrier; ids injected, never hardcoded |
| `diagnostics.js` | `THGAutomation.Diagnostics` | Channel-aware structured gate-failure object (P6) |
| `metrics.js` | `THGAutomation.Metrics` | `channel.action.event` name + fixed dimension whitelist (P7) |

### Adapter contract (the 8 methods)

`parseTarget` · `locateTarget` · `collectCandidates` · `validateCandidate` · `performAction` ·
`verifyAction` · `buildVisionPromptContext` · `normalizeFailureReason`. The orchestrator resolves
an adapter by `ctx.channel` and calls these without channel knowledge.

### Generic candidate schema (P3)

`{ channel, candidate_id, candidate_kind, tag, role, aria, text, parent_text, rect, visible,
enabled, editable, channel_metadata, accepted, reject_reason }`. CORE fills the generic DOM
fields; the adapter fills `channel_metadata` (FB: `target_post_id`, `composer_reason`).

### Evidence safety (P4)

No `raw_html` field exists. `build()` defensively `sanitize`s forbidden keys
(cookie/localStorage/sessionStorage/token/c_user/access_token/csrf/fb_dtsg/…); `assertSafe()` is
the strict throw-gate. The emitted envelope can never carry secrets or a full DOM dump.

## Facebook ADAPTER — `local-connector-extension/channels/facebook/`

| Module | Global | Wraps |
|---|---|---|
| `target_locator.js` | `THGChannelFacebook.targetLocator` | FB URL → `{channel,kind,id,url}` (mirrors `extractPostIdFromUrl`) |
| `failure_reasons.js` | `THGChannelFacebook.failureReasons` | raw FB reason code → neutral `{phase,reason}` |
| `candidate_collector.js` | `THGChannelFacebook.candidateCollector` | `THGCommentComposer.findComposerEntry` → `Candidate[]` |
| `adapter.js` | `THGChannelFacebook.adapter` | the 8 methods, delegating to the proven globals; self-registers as `facebook` |

`performAction` delegates to `THGContentOutbound.executeOutbound` (the existing executor) — the
insertion/submit path is never reimplemented. `collectCandidates`/`validateCandidate` reuse the
unified `THGCommentComposer` doctrine (host `hostVerdict`, create-post exclusion) shipped in
P0/P0b, so all phases share ONE composer doctrine.

## How Taobao / 1688 plug in (no core rewrite)

Add `channels/taobao/{target_locator,candidate_collector,failure_reasons,adapter}.js` implementing
the same 8 methods; `register('taobao', impl)`. `candidate_kind` becomes `search_box`,
`product_card`, `contact_supplier_button`, `message_box`, `submit_button`; `channel_metadata`
carries `product_id`/`supplier_id`. CORE (`THGAutomation.*`) is untouched. Manifest gains a
per-host block (no taobao/1688 `host_permissions` until real automation — least privilege).

## Tests

- `test/automation_core.test.js` — CORE only; **no channel labels/URLs/selectors** as fixtures
  (uses the generic `demo` channel). Covers registry/contract/candidate/evidence-redaction/
  context/diagnostics/metrics.
- `test/facebook_adapter.test.js` — FB specifics live here: the live `/groups/.../posts/2040…`
  URL parse, failure normalization, candidate wrapping of the "Write an answer…" shape, contract
  conformance + registration, `performAction` executor delegation.

## Deferred (explicitly NOT in this scaffolding)

P1 Vision fallback (`vision_fallback`, backend `/vision/gate1-fallback`, `circuit_breaker`,
`safe_dom_mapping`) — designed to consume `Candidate`/`Evidence`/`buildVisionPromptContext`, built
AFTER this scaffolding. Telegram T1/T2 is a separate parallel track (read-only control-plane).
