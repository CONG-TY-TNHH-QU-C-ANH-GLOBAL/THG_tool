# Facebook Comment Automation — ROOT CAUSE REPORT

> **STATUS: ROOT CAUSE ESTABLISHED (2026-06-04).** Populated from a real
> 20-attempt run on org 5 (account 102, group 1312868109620530). Of the 20
> rows, **7 carry a classified Evidence Pack and ALL 7 are identical: Group A —
> Redirect Failure**, with the `nav_events` trace naming the culprit as the
> **Facebook SPA router** (`history.pushState` → home). 13 rows are unclassified
> (empty outcome — see the caveat). **Dominant root cause = A. Redirect → PR8B
> direction = `PR8B-Redirect` (chosen).**

- **Owner of the 20-run gate:** Operator (founder). Claude cannot run the live
  browser; it can only read the evidence the runs produce and classify it.
- **Extension build required:** manifest **0.5.19** (PR8A evidence pack). Reload
  the extension before collecting, or the new fields will be absent.
- **Opened:** 2026-06-04. **Investigation log:** `autocomment-redirect-investigation.md` (same directory).

---

## What PR8A captures per failed attempt

Every comment failure now persists a complete Evidence Pack into
`execution_attempts.evidence_json.nav_diagnostic` (Go struct `models.NavDiagnostic`):

| Field | Meaning | Group it discriminates |
|---|---|---|
| `target_url` (row column) | the post we intended | — |
| `nav_to_url` | URL background asked the tab to load | — |
| `landed_url` | URL the **background** verified the tab reached (≈ target) | — |
| `final_url` | `location.href` when the **content script** evaluated the gate (post-drift) | **A** (`final_url != target`) |
| `doc_title` | page title at gate eval | A / login |
| `redirect_class` | `permalink\|feed\|home\|login\|checkpoint\|unsupported_target\|unknown` | A |
| `phase` | **last execution phase reached**: `navigation\|gate1\|composer\|typing\|submit\|verify` | A–E (primary key) |
| `article_found` / `permalink_found` / `comment_button_found` | pre-comment gate booleans | B / C |
| `article_count` | `[role=article]` on the page | A (0) vs B (>0) |
| `comment_button_count` | visible Comment/Bình luận buttons | C |
| `composer_count` | `[contenteditable][role=textbox]` composers | C / D |
| `textarea_count` / `contenteditable_count` | raw editor census | C / D |
| `nav_events` | full `webNavigation` timeline (before/committed/completed/history) | A (names FB-redirect vs our code) |
| `dom_snapshot` | ≤2KB text excerpt of the landed page | login wall / "Content unavailable" |
| `screenshot_path` | server-written JPEG path `data/evidence/<org>/ob<id>-att<id>-<ts>.jpg` | visual confirm for every group |

---

## Deterministic bucket assignment (evidence → group)

Assign each failed attempt to **exactly one** group using `phase` first, then the
supporting fields. This rule is mechanical — no judgement:

| `phase` | `redirect_class` / counts | Group |
|---|---|---|
| `navigation` | `home` / `feed` (final_url != target) | **A. Redirect Failure** |
| `navigation` | `login` / `checkpoint` | **(human_required — not a delivery bug; tally separately)** |
| `gate1` | `permalink`, `article_count==0` | **B. Gate Failure** (reached post, article never stabilised) |
| `composer` | `article_count>0`, `composer_count==0` | **C. Composer Failure** |
| `typing` | `composer_count>0`, editor never held text | **D. Typing Failure** |
| `submit` | text inserted, composer never cleared | **(submit-stage failure — fold into E or its own line)** |
| `verify` | submit cleared but verifier found no node | **E. Verification Failure** |

---

## Population queries (run against the runtime DB / superadmin endpoint)

Latest failed comment attempt, full pack:

```sql
SELECT
  id, target_url, outcome, failure_reason,
  json_extract(evidence_json,'$.nav_diagnostic.phase')                 AS phase,
  json_extract(evidence_json,'$.nav_diagnostic.redirect_class')        AS redirect_class,
  json_extract(evidence_json,'$.nav_diagnostic.landed_url')            AS landed_url,
  json_extract(evidence_json,'$.nav_diagnostic.final_url')             AS final_url,
  json_extract(evidence_json,'$.nav_diagnostic.article_count')         AS article_count,
  json_extract(evidence_json,'$.nav_diagnostic.comment_button_count')  AS comment_button_count,
  json_extract(evidence_json,'$.nav_diagnostic.composer_count')        AS composer_count,
  json_extract(evidence_json,'$.nav_diagnostic.screenshot_path')       AS screenshot_path,
  json_extract(evidence_json,'$.nav_diagnostic.nav_events')            AS nav_events
FROM execution_attempts
WHERE action_type='comment' AND outcome NOT IN ('dom_verified','optimistic_success','duplicate_blocked')
ORDER BY id DESC
LIMIT 1;
```

Distribution across the last 20 failed attempts (auto-buckets by phase):

```sql
SELECT
  json_extract(evidence_json,'$.nav_diagnostic.phase') AS phase,
  COUNT(*) AS n
FROM (
  SELECT evidence_json FROM execution_attempts
  WHERE action_type='comment'
    AND outcome NOT IN ('dom_verified','optimistic_success','duplicate_blocked')
  ORDER BY id DESC LIMIT 20
)
GROUP BY phase
ORDER BY n DESC;
```

---

## Findings — from the 2026-06-04 run (org 5 / acct 102 / group 1312868109620530)

Query 1 (phase distribution, last 20 failed):

| `phase` | n |
|---|---|
| `(empty)` | 13 |
| `navigation` | 7 |

Of the 7 rows that carry a classified Evidence Pack, **all 7 are identical**:
`outcome=target_not_reached`, `phase=navigation`, `redirect_class=home`,
`landed_url=<the post permalink>` (background verified the post DID load),
`final_url=https://www.facebook.com/` (content script saw home), `article_count=2`
(the 2 home-feed articles, not the target), `composer_count=0`.

| Group | Count | % of classified | Confidence | Evidence |
|---|---|---|---|---|
| **A. Redirect Failure** | **7** | **100%** | **HIGH** | ids 220,217,214,211,208,205,202; screenshot `data/evidence/5/ob102-att211-1780558301.jpg` |
| B. Gate Failure         | 0 | 0% | — | — |
| C. Composer Failure     | 0 | 0% | — | — |
| D. Typing Failure       | 0 | 0% | — | — |
| E. Verification Failure | 0 | 0% | — | — |
| (human_required)        | 0 | 0% | — | redirect_class is `home`, never `login`/`checkpoint` |
| (unclassified — empty)  | 13 | — | — | empty outcome; see caveat below |

**DOMINANT ROOT CAUSE = A. Redirect Failure (7/7 classified, 100%).**

### `nav_events` verdict — WHO pulled the tab to home (id 220)

```
t+17ms    before     .../posts/2031780854395915/    ← our nav to the POST starts
t+374ms   committed  link    .../posts/...          ← committed to the POST
t+670ms   history    link    .../posts/...          ← still on post
t+2865ms  completed          .../posts/...          ← POST fully loaded (we are ON it)
t+8447ms  history    link    https://www.facebook.com/   ← FB SPA ROUTER → home
t+8451ms  history    link    https://www.facebook.com/   ← (again)
```

| `kind` / `qualifiers` of the home event | Culprit | Present here? |
|---|---|---|
| `committed` + `client_redirect`/`server_redirect` | Facebook top-level redirect | ✗ |
| `history` (onHistoryStateUpdated) | **Facebook SPA router (`history.pushState`)** | ✓ **(t+8447, t+8451)** |
| `committed` + `typed`/`auto_toplevel` no qualifier | our `chrome.tabs` code | ✗ |

**Verdict: Facebook's own client-side SPA router resets the tab to the home feed
~8.4 s after the tab opens.** Not an HTTP redirect, not our code.

### The decisive timing finding

The post was present and STABLE from **t+2865 → t+8447 (~5.5 s)** — commenting was
fully possible in that window. But `commands.js navigateAndVerify` runs a FIXED
`delay(5000)` before it verifies the URL and hands off to the content script, so
the comment executor only enters at **~t+8.4 s — exactly when FB resets**. We
arrive to comment at the precise moment the SPA router pulls the tab to home, so
gate-1 polls the home feed, never finds the target article, and reports
`target_not_reached` / `redirect_class=home`. The 5 s settle (tuned for CRAWL
feed-render) wastes the entire stable window for the COMMENT path.

Two live sub-hypotheses remain, which PR8B-Redirect's first move discriminates:
- **A1 (timer):** the reset is a ~8 s timer from tab-open, independent of us.
  Then handing off earlier (inside the stable window) makes the comment land.
- **A2 (activity-triggered):** the reset is triggered by our content-script
  activity (its closeness to our ~8.4 s handoff is suspicious). Then handing off
  earlier moves the reset earlier too — the new `nav_events` would show the
  `history`→home event right after the (now earlier) handoff. THAT would be the
  first evidence that DOM automation on the permalink surface is platform-limited
  → justifies a technology change (m.facebook.com / GraphQL) as PR8C.

### Caveat — 13 unclassified rows (empty outcome)

13 of 20 rows have empty `outcome`/`phase`. They alternate 1-classified : 2-empty,
which suggests ~3 execution_attempts rows per outbound where only one was
finalized with evidence. This is a measurement gap, not a competing root cause
(no row shows gate/composer/typing/submit/verify). Investigate with:

```sql
SELECT id, outbound_id, status, outcome, attempt,
       length(evidence_json) AS ev_len, started_at, finished_at
FROM execution_attempts WHERE action_type='comment' ORDER BY id DESC LIMIT 20;
```

If the empties are `status='verifying'` rows that never reached
`FinishExecutionAttempt`, that is an orphaned-attempt cleanup item — tracked
separately from the delivery root cause.

---

## PR8B — DIRECTION CHOSEN: `PR8B-Redirect`

The dominant root cause is **A. Redirect (100% of classified)**, so exactly one
direction is taken: **PR8B-Redirect**. No other direction is touched (no Gate /
Composer / Verify work).

**Why NOT a technology change yet.** The mandate allows a tech change (Playwright /
GraphQL / m.facebook) ONLY when evidence proves the current technology is
platform-limited. The evidence here shows a **timing collision** (our fixed 5 s
settle hands off at the FB reset edge) — fixable WITHIN the current DOM-automation
stack. A tech change is therefore NOT justified yet. It becomes justified only if
sub-hypothesis **A2** is confirmed by the next run (reset follows our handoff even
when handoff moves earlier).

### PR8B-Redirect — step 1 (shipped 2026-06-04)

Give the COMMENT path a short settle so the content script enters the post while
it is still in the stable window (t≈3–4 s), instead of at the ~8.4 s reset edge:

- `commands.js navigateAndVerify(navigateTo, opts)` — new optional `opts.settleMs`
  (default **5000**, crawler UNCHANGED).
- `outbox.js executeInFacebookTab` — comment nav passes `{ settleMs: 800 }`, so
  URL-verify + handoff happen ~4 s earlier. gate-1 then polls the post during the
  proven-stable window and types before FB's SPA reset.

This is also the **A1/A2 discriminator**: re-run 20 attempts and read `nav_events`.
- Comment lands (or `nav_events` shows NO `history`→home before our comment) →
  **A1 (timer) confirmed, root cause fixed.** Close PR8.
- `nav_events` shows `history`→home again, now right after the earlier handoff →
  **A2 (activity-triggered) confirmed.** Open **PR8C** for the technology change
  (m.facebook.com mbasic surface has no SPA router → no pushState reset; or the
  GraphQL track). Only then is a tech change evidence-backed.

### Verification gate for step 1 — RESULT (2026-06-04, build 0.5.20): A2 CONFIRMED

Step 1 did NOT change the outcome. Query 1 on 0.5.20 is unchanged: **7 navigation
/ 0 in any other group** (+13 empty). The faster settle only moved the reset
earlier — a controlled before/after that settles A1 vs A2:

| | 0.5.19 (settle 5000) id 220 | 0.5.20 (settle 800) id 316 |
|---|---|---|
| `completed` | t+2865 | t+4715 |
| handoff (≈ready+settle) | ≈ t+8365 | ≈ t+6015 |
| **FB reset → home** (`history`) | t+8447 | t+6162 |
| **reset − handoff** | **+82 ms** | **+147 ms** |

Moving the handoff −2350 ms moved the reset −2285 ms; the reset fires ~100 ms
**after** the content-script handoff in BOTH builds. The reset is **triggered by
our content-script activity on the permalink surface**, not a fixed timer (**A2**).
Corroborated by the historical baseline: the operator can open these same post
URLs manually and comment fine — manual works, automation is bounced. ⇒ the www
permalink DOM-automation surface is platform-limited.

**Conclusion: A2 → open PR8C (technology change), evidence-justified.** Step 1
stays in (a short comment settle is harmless and correct) but is NOT the fix.

---

## PR8C-Forensics — name the trigger BEFORE any technology change (shipped 2026-06-04, build 0.5.21)

**Technology-change proposals (mbasic / GraphQL / Playwright) are HALTED.** The
evidence proves FB fires `history.pushState`→home ~100 ms after our content
script attaches; it does NOT prove WHAT we did triggers it. Changing the surface
(www→mbasic) while keeping the SAME content-script interaction model (DOM scan,
synthetic clicks, large innerHTML reads, observers) risks reproducing the bounce
on the new surface — weeks of refactor for the same failure. Architect ruling:
identify the exact last operation before the reset, or exhaustively rule the
content-script interaction model out, FIRST.

### What PR8C-Forensics instruments
`content/forensics.js` monkey-patches the **isolated world** so EVERY DOM op our
content script performs in the comment window is timestamped (it does NOT touch
the page's own MAIN-world calls), and `src/outbox.js` injects a **MAIN-world**
`history.pushState`/`replaceState` interceptor that captures FB's reset with a
**stack trace**. Persisted into `evidence_json.nav_diagnostic.forensics`:

| Field | Answers |
|---|---|
| `counts` | how many `querySelectorAll` / `querySelector` / `click` / `focus` / `dispatchEvent` / `MutationObserver.observe`; `innerHTML_bytes`, `innerText_bytes` totals |
| `timeline[]` | the last ~80 ops, each `{t (ms), op, detail}` (selector+result count, target tag) |
| `push_states[]` | every history mutation with FB's `stack` at the call site |
| `reset_t_ms` / `reset_stack` | ms to the first home/feed pushState, and FB's stack there |
| `last_op_before_reset` | **the prime suspect** — our final op at/before the reset |

### Read it (superadmin SQL)
```sql
SELECT id,
  json_extract(evidence_json,'$.nav_diagnostic.forensics.reset_t_ms')            AS reset_ms,
  json_extract(evidence_json,'$.nav_diagnostic.forensics.last_op_before_reset')  AS last_op,
  json_extract(evidence_json,'$.nav_diagnostic.forensics.counts')                AS counts,
  json_extract(evidence_json,'$.nav_diagnostic.forensics.reset_stack')           AS fb_reset_stack,
  json_extract(evidence_json,'$.nav_diagnostic.forensics.push_states')           AS push_states,
  json_extract(evidence_json,'$.nav_diagnostic.forensics.timeline')              AS timeline
FROM execution_attempts
WHERE action_type='comment' AND outcome='target_not_reached'
ORDER BY id DESC LIMIT 1;
```

### How the forensics verdict routes the (still un-chosen) PR8C fix
- `last_op_before_reset` is a **specific noisy op** (e.g. `dispatchEvent pointerdown`
  from dismissBlockingOverlays, or a giant `innerHTML.get` from currentFBUserID),
  and `reset_stack` points at an FB visibility/integrity handler → the trigger is
  OUR behaviour, not the surface → **quiet-entry fix within the current stack**
  (strip that op) is the right move, NO tech change.
- The reset fires with **no preceding content-script op** (`last_op_before_reset`
  null but reset present), or after only passive reads → the surface bounces
  automated deep-links regardless of what we do → THAT is the evidence that the
  www permalink surface is platform-limited → only THEN propose a tech change
  (mbasic server-rendered surface / GraphQL), with success probabilities.

> **No technology change is designed until the forensics output above is read
> from a real run.** PR8B-Redirect step 1 (short settle) stays in (harmless), but
> is confirmed NOT the fix.

### Verification gate
Reload extension to **0.5.21**, run `comment_all_leads`, then run the forensics
SQL above and paste `last_op_before_reset` + `reset_stack` + `counts`. That names
the trigger and picks the fix deterministically.

---

## PR8C-Forensics — RESULT (2026-06-04): TRIGGER NAMED. It was OUR code. (fix in 0.5.22)

The forensics SQL returned, unambiguously:

```
last_op_before_reset : {"t":7,"op":"dispatchEvent","detail":"click a[role=link][al=Facebook]"}
counts               : {"click":1,"dispatchEvent":7,"innerHTML.get":1,"innerHTML_bytes":5566912,
                        "innerText.get":61,"innerText_bytes":4879,"querySelectorAll":131}
push_states          : [ {"t":11,"url":"/","method":"pushState",   stack:<FB router Object.a>},
                         {"t":15,"url":"/","method":"replaceState", stack:<FB router Object.l>} ]
reset_stack          : Error at post → history.pushState → Object.a [as pushState]  (FB SPA router)
```

Read literally:
- **t+7 ms — OUR content script `clickLikeUser`d `a[role=link][aria-label="Facebook"]`** — the
  top-nav Facebook **logo** (a link to home). `counts` confirms exactly ONE
  clickLikeUser (1 `click` + its 7 synthetic `dispatchEvent`s).
- **t+11 ms — FB's SPA router reacts to that click and `pushState("/")`** — i.e.
  FB navigated to home **because we clicked the home logo**, exactly as it would
  for a real user. The `reset_stack` is FB's ordinary history wrapper, NOT an
  anti-bot / integrity handler.

### Root cause (definitive)
`dismissBlockingOverlays()` (runs first in `executeComment` to close popups)
matched dismiss keywords with a raw substring `label.includes(key)`. The key
**`'ok'` is a substring of `"facebook"`** (faceb-**OO**-K). So the Facebook logo
(`aria-label="Facebook"`) was classified as an "OK" dismiss button and clicked,
navigating us to the feed. This is **NOT** A1/A2, **NOT** anti-automation, **NOT**
a platform limit — it is a two-character substring false-match in our own code.

This is precisely why the technology change was halted: www→mbasic/GraphQL would
have kept `dismissBlockingOverlays` and **reproduced the bounce**, burning weeks.

### Fix (0.5.22, within-stack — no tech change)
`content/outbound.js dismissBlockingOverlays`:
1. **Whole-word matching** (`labelMatchesDismiss`, `(^|\W)key($|\W)`) instead of
   substring — `"ok"` no longer matches `"facebook"`.
2. **Button-only candidate set** (`div[role=button], button, a[role=button],
   span[role=button]`) — drops the bare `[aria-label]` that caught the
   `a[role=link]` logo.
3. **Defense-in-depth:** skip any candidate that is a navigation link
   (`role="link"`, or an `<a href>` without `role="button"`).

### Verification gate (0.5.22)
Reload to **0.5.22**, run `comment_all_leads`. Expected: the forensic
`last_op_before_reset` no longer shows the logo click; the tab stays on the post;
gate-1 finds the article and proceeds past `navigation` into
`composer`/`typing`/`submit`. Re-run Query 1 — `navigation` should collapse. If a
real `dom_verified` lands, **PR8 is closed.**

---

## PR8D — submit-phase cleanup (0.5.23). Logo bug GONE; comments now post.

0.5.22 result: comments post. Latest attempt forensics returned **`phase=verify`**
(composer cleared → submitted) with `click:4, dispatchEvent:30`. Two residual
issues surfaced, both in the submit/typing phase (NOT navigation):

**1. "Bị mò" — the avatar/sticker picker opened.** `findSubmitButtons` clicked a
composer-toolbar icon before the real send button. Root cause: `rejectActionLabel`
listed only ENGLISH icon labels (emoji/sticker/gif) but FB renders **Vietnamese**
aria-labels (nhãn dán / biểu tượng cảm xúc / máy ảnh / avatar), which passed as
spatial submit candidates and were clicked (opening "Avatar của bạn"). The comment
still posted via a later click, so the attempt reached `verify` — but it groped.
Fix: (a) `rejectActionLabel` extended with the Vietnamese toolbar labels;
(b) `findSubmitButtons` now tries **labeled** submit buttons ("Bình luận"/"Gửi")
FIRST, spatial-only icons only as fallback — so the real send is clicked first and
the editor clears before any icon is reached.

**2. Duplicated comment text.** FB persists an unsent comment **draft per post**;
on a retry it re-mounts the draft and `insertText` appended to it. Fix:
`setEditableText` now clears the editor (`selectNodeContents` + `execCommand
('delete')`) before inserting, so a restored draft can't double the text.

**3. Forensics flooding (tooling fix).** The proof phase fired 356 `innerText`
reads that flushed the click ops out of the 80-event ring. `content/forensics.js`
now keeps mutating ops (click/focus/dispatchEvent/observe) in a separate
`actions` buffer (`nav_diagnostic.forensics.actions`) that read-volume can't
flood, so the triggering click is always visible.

**Separate / upstream — not in 0.5.23:** the comment text begins with the literal
placeholder **"Anonymous participant"** because the lead's author name was never
resolved (crawler) and the AI prompt used the placeholder. This is a content /
name-resolution fix in the Go AI layer (e.g. omit the name salutation when the
author is "Anonymous participant"/empty), tracked separately from the executor.

### Verification gate (0.5.23)
Reload to **0.5.23**, run `comment_all_leads`. Expected: no avatar/sticker picker
opens (forensic `actions` shows the labeled send clicked directly, no toolbar-icon
click), the comment text is single (no duplication), and the attempt reaches
`dom_verified`. Read `forensics.actions` to confirm the click sequence.
