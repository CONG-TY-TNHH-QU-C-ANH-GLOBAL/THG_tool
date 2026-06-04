# Facebook Comment Automation — ROOT CAUSE REPORT

> **STATUS: AWAITING EVIDENCE.** This report is a SCAFFOLD. It must NOT be
> filled from inference. It is populated ONLY from the PR8A Evidence Pack after
> **≥20 real `comment_all_leads` attempts** have been run and their
> `execution_attempts.evidence_json` collected. Until the buckets below carry
> real counts + screenshots, **no PR8B may be designed** (see the gate at the
> bottom).

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

## Findings — FILL ONLY FROM REAL DATA

> Replace every `?` below from the 20-run query output. Do not estimate.

| Group | Count | % of 20 | Confidence | Representative screenshot | Representative `nav_events` |
|---|---|---|---|---|---|
| A. Redirect Failure       | ? | ? | ? | `?` | `?` |
| B. Gate Failure           | ? | ? | ? | `?` | `?` |
| C. Composer Failure       | ? | ? | ? | `?` | `?` |
| D. Typing Failure         | ? | ? | ? | `?` | `?` |
| E. Verification Failure   | ? | ? | ? | `?` | `?` |
| (human_required: login/checkpoint) | ? | ? | ? | `?` | — |

**Dominant root cause = `?`** (the single highest-count group).

### For group A (Redirect), the `nav_events` verdict (who pulled the tab to home)

Read the event whose `url` is the home/feed URL:

| `kind` / `qualifiers` | Culprit |
|---|---|
| `committed` + `client_redirect` / `server_redirect` | **Facebook** top-level redirect |
| `history` | **Facebook SPA router** reset |
| `committed`, `typed`/`auto_toplevel`, no redirect qualifier | **Our `chrome.tabs` code** |

Fill: `?`

---

## PR8B GATE — DO NOT CROSS WITHOUT THE TABLE ABOVE FILLED

Per the PR8 mandate, exactly **one** PR8B direction is chosen, and **only after**
the dominant root cause is established by the real counts above:

- Dominant = **A. Redirect** → `PR8B-Redirect`
- Dominant = **B. Gate** → `PR8B-Gate`
- Dominant = **C. Composer** → `PR8B-Composer`
- Dominant = **E. Verify** → `PR8B-Verify`

> **No PR8B design appears in this repo until this report's findings table is
> populated from ≥20 real attempts.** Designing it earlier violates the
> "Evidence trước Fix" rule.
