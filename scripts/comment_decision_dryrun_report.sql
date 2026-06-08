-- Comment Decision DRY-RUN evaluation queries (P2c-dry-run).
--
-- Source: rows written by recordReasoningDryRun() into prompt_logs
--   source = 'system', action_taken = 'comment_decision_dryrun',
--   action_args = the CommentDecision JSON, success = (NOT knowledge_gap).
--
-- Run AFTER producing dry-run rows (THG_COMMENT_REASONING_DRYRUN=1, then queue
-- 20–50 comment leads). Default DB: data/scraper.db.
--
--   PowerShell:  sqlite3 data\scraper.db ".read scripts\comment_decision_dryrun_report.sql"
--   (or paste each query into the superadmin SQL endpoint)
--
-- SCOPE: by default these read ALL dry-run rows. To scope to one org or a time
-- window, add e.g.  AND org_id = 5  /  AND created_at >= datetime('now','-2 hours')
-- to the WHERE in each query (and the CTE).

.headers on
.mode column

-- 0. Sample size in scope ----------------------------------------------------
SELECT '0_total_decisions' AS metric, COUNT(*) AS value
FROM prompt_logs
WHERE source='system' AND action_taken='comment_decision_dryrun';

-- 1. knowledge_gap rate ------------------------------------------------------
WITH d AS (
  SELECT action_args AS j FROM prompt_logs
  WHERE source='system' AND action_taken='comment_decision_dryrun'
)
SELECT
  COUNT(*)                                                                   AS total,
  SUM(CASE WHEN json_extract(j,'$.knowledge_gap')=1 THEN 1 ELSE 0 END)       AS knowledge_gap_true,
  ROUND(100.0*SUM(CASE WHEN json_extract(j,'$.knowledge_gap')=1 THEN 1 ELSE 0 END)/COUNT(*),1) AS gap_pct
FROM d;

-- 2. intent distribution -----------------------------------------------------
WITH d AS (
  SELECT action_args AS j FROM prompt_logs
  WHERE source='system' AND action_taken='comment_decision_dryrun'
)
SELECT
  json_extract(j,'$.intent')                       AS intent,
  COUNT(*)                                         AS n,
  ROUND(100.0*COUNT(*)/(SELECT COUNT(*) FROM d),1) AS pct
FROM d
GROUP BY intent
ORDER BY n DESC;

-- 3. average selected counts (capabilities / products / proofs / cta rate) ---
WITH d AS (
  SELECT action_args AS j FROM prompt_logs
  WHERE source='system' AND action_taken='comment_decision_dryrun'
)
SELECT
  ROUND(AVG(COALESCE(json_array_length(j,'$.selected.capabilities'),0)),2) AS avg_capabilities,
  ROUND(AVG(COALESCE(json_array_length(j,'$.selected.products'),0)),2)     AS avg_products,
  ROUND(AVG(COALESCE(json_array_length(j,'$.selected.proofs'),0)),2)       AS avg_proofs,
  ROUND(AVG(CASE WHEN json_extract(j,'$.selected.cta') IS NOT NULL THEN 1 ELSE 0 END),2) AS cta_rate,
  ROUND(AVG(json_extract(j,'$.confidence')),3)                            AS avg_confidence
FROM d;

-- 4. most-selected assets / SKUs (across capabilities+products+proofs) -------
WITH d AS (
  SELECT action_args AS j FROM prompt_logs
  WHERE source='system' AND action_taken='comment_decision_dryrun'
),
items AS (
  SELECT json_extract(e.value,'$.source_asset_id') AS asset_id,
         json_extract(e.value,'$.sku')             AS sku,
         json_extract(e.value,'$.label')           AS label
  FROM d, json_each(COALESCE(json_extract(d.j,'$.selected.capabilities'),'[]')) e
  UNION ALL
  SELECT json_extract(e.value,'$.source_asset_id'), json_extract(e.value,'$.sku'), json_extract(e.value,'$.label')
  FROM d, json_each(COALESCE(json_extract(d.j,'$.selected.products'),'[]')) e
  UNION ALL
  SELECT json_extract(e.value,'$.source_asset_id'), json_extract(e.value,'$.sku'), json_extract(e.value,'$.label')
  FROM d, json_each(COALESCE(json_extract(d.j,'$.selected.proofs'),'[]')) e
)
SELECT asset_id, sku, COUNT(*) AS times_selected, MIN(label) AS example_label
FROM items
GROUP BY asset_id, sku
ORDER BY times_selected DESC
LIMIT 20;

-- 5. 10 representative decisions (most recent) -------------------------------
SELECT
  id, created_at,
  json_extract(action_args,'$.intent')        AS intent,
  json_extract(action_args,'$.confidence')    AS confidence,
  json_extract(action_args,'$.knowledge_gap') AS knowledge_gap,
  action_args                                 AS decision_json
FROM prompt_logs
WHERE source='system' AND action_taken='comment_decision_dryrun'
ORDER BY created_at DESC
LIMIT 10;

-- 6. INVARIANT CHECK: high confidence but knowledge_gap=true -----------------
-- recalibrateConfidence forces confidence=0 when no offer is grounded, so this
-- MUST return 0 rows. Any row here is a bug in the grounding/recalibration.
SELECT
  id,
  json_extract(action_args,'$.confidence')    AS confidence,
  json_extract(action_args,'$.knowledge_gap') AS knowledge_gap
FROM prompt_logs
WHERE source='system' AND action_taken='comment_decision_dryrun'
  AND json_extract(action_args,'$.knowledge_gap')=1
  AND json_extract(action_args,'$.confidence') > 0;

-- 7a. ROLE/SEMANTIC mismatch: a PRODUCT pitched to a service_seeking lead ----
SELECT
  id,
  json_extract(action_args,'$.intent') AS intent,
  json_array_length(COALESCE(json_extract(action_args,'$.selected.products'),'[]')) AS n_products,
  action_args AS decision_json
FROM prompt_logs
WHERE source='system' AND action_taken='comment_decision_dryrun'
  AND json_extract(action_args,'$.intent')='service_seeking'
  AND json_array_length(COALESCE(json_extract(action_args,'$.selected.products'),'[]')) > 0
ORDER BY created_at DESC;

-- 7b. NOTE: "a capability grounded from a POD_product" CANNOT appear in output
-- — the P2a.1 role guard (kindAllowedForRole) drops it before it is recorded.
-- To measure how often the LLM ATTEMPTED a mis-slot (and was blocked), we would
-- need to persist GroundingStats.OfferDropped into the dry-run record; that is a
-- small observation-only addition, not yet wired (see report §7).
