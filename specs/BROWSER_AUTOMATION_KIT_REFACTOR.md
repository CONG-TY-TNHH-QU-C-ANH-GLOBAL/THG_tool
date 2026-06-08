# Browser Automation Kit Refactor

> **STATUS: BACKLOG / DESIGN (opened 2026-06-08, founder-directed).** Refactor the
> 1400-line `content/outbound.js` into a layered, multi-platform kit (Facebook,
> Taobao, 1688) so click/type/wait/proof are written ONCE. Separate track from FB
> Automation Reliability and Comment Intelligence. No big-bang; PR-1 changes ZERO
> production behavior.

## Principles (founder)
1. No big-bang. 2. No behavior change in PR-1. 3. Extract layer-by-layer, each PR
small + syntax/test check. 4. Core knows NOTHING about Facebook/Taobao/1688.
5. Platform adapters own selectors + identity. 6. Runtime only routes actions, no
selectors. 7. Actions return result/proof via a shared contract.

---

## Codebase reality that SHAPES the design (verified)

**A. Loading model = classic content scripts + global namespaces, in manifest
order.** `manifest.json` loads `content/shared.js → meta.js → commands.js →
crawl.js → proof.js → navreport.js → forensics.js → outbound.js → bridge.js →
content.js` for `matches: facebook.com`. Each file is an IIFE exposing a global
(`THGContentShared`, `THGContentMeta`, `THGContentOutbound`, …). There is **NO
bundler and NO ES modules** in content scripts. → The `core/runtime/platforms`
files become **global-namespace classic scripts loaded in dependency order**;
`core/*` MUST be listed in the manifest BEFORE the files that use them. (A bundler
(esbuild) is an alternative that enables real `import`, but it adds build infra +
risk — REJECTED for "no big-bang." Revisit only if the global pattern gets
unwieldy.)

**B. Platform separation extends to the MANIFEST, not just folders.** Today one
`content_scripts` block matches facebook.com. Multi-platform = **one block per
host**: a facebook block loads `core/* + runtime/* + platforms/facebook/*`
(matches facebook.com); a future taobao block loads `core/* + runtime/* +
platforms/taobao/*` (matches taobao.com). `core/` + `runtime/` are SHARED (listed
in every block, in order). The platform folder is what differs per block.

**C. Two layers, both real — the refactor targets the EXECUTOR.**
`background.js` `importScripts(src/facebook-state.js, src/api.js, src/outbox.js)`
is the BACKGROUND orchestrator (heartbeat, c_user identity, work polling,
messaging). `content/outbound.js` is the page DOM EXECUTOR. This refactor is about
the **executor** (the 1400-line file). `src/` (orchestrator) is a smaller,
already-modular concern; its platform-identity (facebook-state.js) is addressed by
the FB Reliability Track PR-B, not here.

**D. Isolated vs MAIN world boundary must survive.** `forensics.js` /
`proof.js` inject a MAIN-world `history.pushState` interceptor; everything else
runs in the isolated world. `core/` stays isolated-world; any MAIN-world injection
is explicit + platform-owned.

---

## Cross-cutting concerns (5 lenses)

**Architect.** Map the founder's structure onto loading model A/B. `core/`
exposes one global per file (e.g. `THGKit.core` with `.dom/.input/.wait/.click/
.evidence/.result/.errors`), `runtime/` → `THGKit.runtime` (`action-router`,
`platform-registry`, `execution-context`), platforms → `THGKit.platforms.facebook`
etc. The registry maps `(platform, actionType) → action fn`; the router resolves
by `message.platform` (default `facebook`) then `action`.

**Security (/security-review).** (1) `host_permissions` + `matches` GROW per
platform — least privilege: do NOT add taobao/1688 hosts until real automation
ships (PR-5 skeletons are CODE-ONLY, no manifest host grants, so the update does
not trigger a Chrome permission re-prompt or widen cookie/page access). (2) The
`cookies` permission is scoped by host_permissions; adding a platform host lets the
extension read that platform's cookies — gate per platform. (3) `core/` having no
platform selectors is also a safety boundary: generic DOM ops can't be steered by
a hostile page into a platform action; the (untrusted) selector logic lives in the
platform adapter. (4) Keep the MAIN-world injection minimal + platform-owned (D).

**Backend.** The connector-command + result/proof contract is Facebook-shaped
(`connector_commands.type`, `VerificationEvidence`, `NavDiagnostic`). PR-4/5 need:
a `platform` dimension on the work item / connector command (default `facebook`
for back-compat) and a **platform-agnostic result envelope** (`{ok, action,
platform, evidence_blob, reason_code}`) with platform-specific evidence inside.
This INTERSECTS the FB Reliability Track connector registry (PR-C there): a
connector/account is eventually per-platform with platform-specific identity. Do
NOT duplicate — the connector model gains a `platform` field once, shared.

**Frontend.** Minimal for PR-1..5 (extension-internal). Forward note: the
dashboard (accounts/missions/outbox/readiness) is FB-centric; multi-platform UI is
downstream and should reuse the FB Reliability Readiness Matrix (PR-D) extended
with `platform`. Not in this track's PRs.

**Code-reviewer.** Each extraction PR MUST: (a) move the function to its new file
+ global; (b) update `manifest.json` js order so deps load first; (c) the IIFE in
the consumer references the new global (watch closure-scope captures — nothing may
rely on the old closure-local binding); (d) `node --check` every touched file;
(e) a jsdom/node harness for the PURE-ish functions (extractPostIdFromUrl, version
compare, label/identity filters) that don't need a live browser. DOM-interaction
(click/type) is verified manually on prod. Add a grep gate: `platforms/**` must
NOT redefine core primitive names (no duplicated click/type/wait).

---

## Refined PR plan (maps founder PRs onto loading model)

- **PR-1 Core primitives.** Extract `visible, labelOf, wait, waitFor,
  clickLikeUser, dispatchMouseLike, dispatchPointerLike, setEditableText,
  textOfEditable, editorContainsContent` → `core/*.js` exposing `THGKit.core`.
  Manifest: load `core/*` before `outbound.js`. `outbound.js` calls `THGKit.core.*`.
  ZERO behavior change. Verify: node --check + jsdom harness for the pure ones.
- **PR-2 Facebook locators/identity.** Move `extractPostIdFromUrl,
  extractArticleCanonicalEntityId, findTargetArticle, waitUntilTargetArticleStable,
  onTargetPermalinkPage, findCommentEditor, findComposerForTarget,
  findSubmitButtons` → `platforms/facebook/{selectors,post-locator,composer,
  identity}.js`. ZERO behavior change. (Note: `findComposerForTarget`/permalink
  logic just landed in ext 0.5.29 — keep its semantics byte-for-byte.)
- **PR-3 Facebook actions.** Move comment/inbox/post executors →
  `platforms/facebook/actions/*.js` + `platforms/facebook/proof.js`.
  `content/outbound.js` becomes a thin entrypoint that calls the action.
- **PR-4 Registry + router.** `runtime/platform-registry.js` +
  `runtime/action-router.js` + `runtime/execution-context.js`. `message.platform`
  defaults to `facebook`. Backend: add `platform` (default facebook) to the work
  item / connector command + the platform-agnostic result envelope. content/
  outbound.js → thin bridge → router.
- **PR-5 Taobao/1688 skeletons.** Adapter skeletons only (selectors/identity/
  actions stubs returning `not_implemented`), reusing `core/`. **Code-only — NO
  manifest host_permissions / matches for taobao/1688 yet** (security: no surface
  expansion until real automation). Proves the architecture supports new platforms
  with zero core duplication.

## Definition of Done (+ codebase additions)
Founder DoD (FB comment/inbox/post unchanged · build/syntax pass · no platform
selector in core/ · no duplicated click/type/wait in platforms/ · new adapter
reuses core · outbound.js is a thin bridge) **PLUS**: manifest js-order correct &
per-platform blocks modeled · isolated/MAIN world boundary preserved · backend
work item carries `platform` (default facebook, back-compat) · PR-5 adds NO new
host_permissions · a node/jsdom test harness exists for pure functions · a grep
gate prevents core-primitive duplication in platforms/.

## Open questions
1. Bundler later? (Stay global-namespace for now.)
2. `src/outbox.js` (background orchestrator) vs `content/outbound.js` (executor) —
   confirm this track touches only the executor; orchestrator stays.
3. Connector/account `platform` field — own it here (PR-4) or in FB Reliability
   PR-C connector registry? (Recommend: define once in PR-C, consume here.)
