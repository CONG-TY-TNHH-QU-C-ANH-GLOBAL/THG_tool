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
- **Opened:** 2026-06-04. **Investigation log:** `specs/AUTOCOMMENT_REDIRECT_INVESTIGATION.md`.

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

### Verification gate for step 1
Reload extension (bump pending), run 20 `comment_all_leads`, then re-run
`scripts/rootcause_query.py` / Query 1 + Query 3. Update this section with the
new distribution and the A1-vs-A2 verdict.
