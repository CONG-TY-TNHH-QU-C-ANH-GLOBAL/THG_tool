# §7 NL Direct-Link Comment — Live Smoke Runbook

**Tests against main:** `843f56b857eca4e99604c28c26f6d7ae7e2c071a`
**Scope:** operator-run, deployed/staging, real workspace + real scanned FB post + real ready connector. Docs only — no behavior here.

Linkage note: `outbound_messages` has **no `lead_id` column**. §7 links to the existing lead by **canonical post reference** — the lead's `post_fbid`, or a canonical/same-post `source_url`. The direct-link flow (`comment_single_post`) only queues when the lead already exists (`GetLeadByPostRef`); the synthetic-lead path belongs to a *different* action (`auto_comment`). An outbound row therefore proves it targeted the **existing** lead when **all three** hold:
- the row's `target_url` matches the lead's post/source URL (or the same numeric post id), AND
- the `[queueLeadOutreach]` log shows `scanned=1` (the run was scoped to the one resolved `lead_id`, not a bulk/synthetic run), AND
- no new synthetic lead row was created (the lead pre-existed — see §2(e)).

---

## 1. Smoke checklist (prompts + expected response)

Send each in Copilot/agent for the target workspace. `<scanned>` = a Facebook post URL whose lead already exists in this org; `<unscanned>` = a valid FB post URL with no lead yet.

**Case A precondition:** use a real **scanned** Facebook post URL whose lead exists in this org, AND a lead/post that does **not** already have an active `planned`/`executing` comment row for the same ready actor (run §2(c) first). If a duplicate/coverage block appears instead of a queue, that is **case G** behavior — not a case A failure.

| # | Prompt | Expected user-facing response | Outbound row |
|---|--------|-------------------------------|--------------|
| **A** | `comment bài này <scanned FB post URL>` | "Đã đưa 1 comment vào hàng đợi từ nhóm Cần xử lý sau khi quét 1 lead. Đây CHƯA phải là đã đăng lên Facebook…" | **exactly 1** new row, `type='comment'`, `execution_state='planned'`, `target_url` = the post |
| **B** | `comment bài này <scanned FB post URL>` (workspace with NO ready connector) | "Chưa có tài khoản Facebook sẵn sàng để chạy comment. Hãy kết nối Facebook…" | **0 rows** |
| **C** | `comment bài này <unscanned FB post URL>` | "Bài viết này chưa có trong hệ thống. Hãy quét/import bài viết trước khi comment." | **0 rows** |
| **D** | `comment https://facebook.com.evil.com/posts/1` | ask-for-link ("Bạn gửi giúp tôi link bài viết Facebook cần comment.") — lookalike is not a FB post | **0 rows** |
| **E** | `comment tất cả leads` | the bulk `comment_all_leads` summary ("Đã đưa N comment…" or "Không tìm được lead hợp lệ…") | per bulk policy (N), not the single-post path |
| **F** | `cào comment <FB post URL>` | the `scrape_comments` crawl flow (a crawl/job response), NOT a comment queue | **0 comment rows** from this prompt |
| **G** | `comment bài này <scanned URL>` repeated until coverage/duplicate hits | "Không tìm được lead hợp lệ để comment sau khi quét 1 lead. Bỏ qua 1 lead (lead đã đủ số tài khoản tiếp cận [coverage_full] ×1 / tài khoản này đã comment lead này [already_commented_by_this_actor] ×1)…" | **0 new rows** |

Other URL-layer responses to spot-check: two links → "Bạn chỉ gửi một link bài viết Facebook cho mỗi lần comment."; group/page shell URL → "Link Facebook này chưa được hỗ trợ…".

---

## 2. Safe SQL / log diagnostics (org-scoped, no secrets)

Set `:org_id` and `:post_id` (the numeric Facebook post id from the URL, e.g. `/posts/456/` → `456`). Only non-sensitive columns are selected — `outbound_messages` has no cookie/token/session columns.

**Before starting smoke, record the current UTC timestamp and use it as `:smoke_started_at`** (e.g. `SELECT datetime('now')` → `2026-06-14 09:00:00`). Filtering by `created_at >= :smoke_started_at` prevents rows from previous tests being counted. (`datetime('now','-10 minutes')` is shown as a fallback example only.)

**(a) Confirm the lead exists for the post (pre-condition for A; absence drives C):**
```sql
SELECT id, org_id, COALESCE(NULLIF(source_url,''),'') AS source_url, post_fbid, group_fbid,
       substr(content,1,60) AS content_snippet, archived_at
FROM leads
WHERE org_id = :org_id AND (post_fbid = :post_id OR source_url LIKE '%' || :post_id || '%')
  AND archived_at IS NULL;
```

**(b) Count + inspect the outbound rows queued for this post (A expects 1; B/C/D/G expect 0):**
```sql
SELECT id, org_id, type, account_id, execution_state, verification_outcome,
       target_url, substr(content,1,80) AS content_snippet, created_by, created_at
FROM outbound_messages
WHERE org_id = :org_id AND type = 'comment'
  AND target_url LIKE '%' || :post_id || '%'
  AND created_at >= :smoke_started_at   -- fallback example: datetime('now','-10 minutes')
ORDER BY created_at DESC;
```
- **Exactly one row** for case A. **Zero rows** for B/C/D/G.
- **Links to the existing lead** when row `target_url` matches the lead's `source_url` from (a) (same post id). A bulk/synthetic run would not target this single post with `scanned=1` in the log (see (d)).

**(c) Confirm no DUPLICATE active row for the same account+post (coverage/dedup safety):**
```sql
SELECT account_id, COUNT(*) AS active_rows
FROM outbound_messages
WHERE org_id = :org_id AND type='comment'
  AND target_url LIKE '%' || :post_id || '%'
  AND execution_state IN ('planned','executing')
GROUP BY account_id HAVING COUNT(*) > 1;   -- expect NO rows
```

**(d) The `[queueLeadOutreach]` log line (capture verbatim) — backend stdout/journald:**
```
grep "\[queueLeadOutreach\]" <log source> | tail -5
# A (queued):       [queueLeadOutreach] org=<id> type=comment scanned=1 queued=1 skipped=0 reasons=map[] samples=map[]
# G (coverage):     [queueLeadOutreach] org=<id> type=comment scanned=1 queued=0 skipped=1 reasons=map[coverage_full:1] samples=map[coverage_full:[<leadID>]]
# C/D/B never reach this line (blocked earlier) — see §3.
```
`scanned=1` confirms the run was scoped to the single resolved lead (`lead_id`), and `samples=...[<leadID>]` names the exact lead on a skip.

**(e) Optional — confirm the row is NOT a synthetic shell (lead row predates the outbound row):**
```sql
SELECT (SELECT MIN(created_at) FROM leads          WHERE org_id=:org_id AND post_fbid=:post_id) AS lead_created,
       (SELECT MIN(created_at) FROM outbound_messages WHERE org_id=:org_id AND type='comment'
              AND target_url LIKE '%'||:post_id||'%' AND created_at >= :smoke_started_at) AS row_created;
-- lead_created should be earlier than row_created (the lead was scanned before, not fabricated now).
```

---

## 3. Failure triage matrix

| Symptom | Likely cause | First diagnostic | Likely site |
|---|---|---|---|
| **Routed wrong tool** (e.g. went to `comment_all_leads`/`scrape_comments`/brain) | intent detection | the agent's chosen action in logs; re-run with the exact prompt | `internal/ai/agent_action_router.go` `deterministicFacebookAction` |
| **URL not canonicalized** (scan-required for a real post, or "unsupported" for a valid post) | canonicalizer rejected/missed the shape | `fburl.CanonicalizePostURL` on the URL; check `LooksLikePostURL`/host | `internal/fburl/canonicalize.go` |
| **Lead not found despite existing** | lookup key mismatch (post_fbid empty + non-canonical stored URL — documented limitation) | run SQL (a); compare stored `source_url`/`post_fbid` vs `:post_id` | `internal/store/leads/lookup.go` `GetLeadByPostRef` |
| **No-ready-account not blocking** (queued row when no connector) | readiness gate skipped | SQL (b) shows a row; log lacks the readiness block string | `cmd/scraper/comment_readiness.go` / `queueLeadOutreach` §5 |
| **Outbound row created when it should not** (C/D/B/G) | gate bypass | SQL (b) row count > 0; check `[queueLeadOutreach]` (was it even reached?) | orchestrator `commentSinglePost` → `queueLeadOutreach` |
| **Duplicate/coverage not respected** | coverage/dedup gate | SQL (c) returns rows; log `reasons` lacks `coverage_full`/`already_commented_by_this_actor` | `models.EvaluateCoverage` / outbound dedup |
| **Unsafe lookalike accepted** (D queues a row) | host anchoring bypass | `fburl.IsFacebookURL("https://facebook.com.evil.com/...")` must be false | `internal/fburl/canonicalize.go` `isFacebookHost` |

---

## 4. Evidence to paste back if a case fails

For the failing case, paste:
1. **Main hash tested:** `843f56b8…`
2. **Workspace/org ID** and the **prompt sent** (verbatim, with the exact URL).
3. **Connector/account readiness state:** which FB account is paired, connector online?, stream status, extension version (from the Browser/Connector view or the readiness message).
4. **Response observed** (verbatim Copilot/agent text).
5. **Outbound row observed or not:** output of SQL (b) (the full row(s) or "0 rows").
6. **Lead matched or not:** output of SQL (a) (lead id + source_url + post_fbid, or "0 rows").
7. **Logs captured:** the `[queueLeadOutreach]` line from (d) (or note "line absent" — means blocked earlier), plus any `[reasoning]` / readiness lines around the same timestamp.
8. **Pass/Fail per case** in the table from §1.

Do **not** paste cookies/tokens/session values — the SQL snippets above already exclude them; redact anything sensitive from logs.
