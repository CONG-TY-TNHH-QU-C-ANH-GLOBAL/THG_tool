## ADDED Requirements

### Requirement: Per-item synchronous filter evaluation
The system SHALL implement `FilterEngine.Evaluate(item RawItem, filters TaskFilters) FilterResult` where `FilterResult.Pass` is true only if the item passes ALL six filter stages. The evaluation SHALL be synchronous, inline, and complete before the handler decides to accumulate or discard the item.

#### Scenario: Item passing all filters
- **WHEN** `FilterEngine.Evaluate` is called on an item matching keywords, not spam, above engagement threshold, correct language, above intent match score, with valid author and in-range timestamp
- **THEN** `FilterResult{Pass:true, Score:0.85, Signals:["keyword:ship hàng", "engagement:met"]}` is returned

#### Scenario: Item failing first stage (keyword relevance) short-circuits
- **WHEN** an item's content contains none of `task.crawl_plan.query.keywords` and `filters.keyword_relevance_min=0.3`
- **THEN** `FilterResult{Pass:false, Signals:["keyword:below_threshold"]}` is returned; no further stages are evaluated

### Requirement: Keyword relevance filter
The keyword relevance stage SHALL compute `score = count(keywords found in item.content) / len(crawl_plan.query.keywords)`. FAIL if `score < filters.keyword_relevance_min`. Keyword matching is case-insensitive and Unicode-normalized.

#### Scenario: Partial keyword match above threshold
- **WHEN** task has 4 keywords and item content matches 2, and `keyword_relevance_min=0.3`
- **THEN** score = 0.5 → PASS

#### Scenario: Match below threshold
- **WHEN** task has 4 keywords, item matches 1 (score=0.25), and `keyword_relevance_min=0.3`
- **THEN** FAIL with signal `"keyword:below_threshold(0.25<0.30)"`

### Requirement: Spam detector filter
The spam stage SHALL FAIL an item if: (a) the item's content matches any string in `filters.exclude` (case-insensitive substring match), OR (b) content length < 20 characters, OR (c) more than 95% of alphabetic characters are uppercase (all-caps spam pattern).

#### Scenario: Exclude phrase match fails item
- **WHEN** `filters.exclude = ["xem thêm", "spam"]` and item content contains "spam cảnh báo"
- **THEN** FAIL with signal `"spam:exclude_match:spam"`

#### Scenario: Very short content fails
- **WHEN** item content is "Ok" (2 chars)
- **THEN** FAIL with signal `"spam:too_short(2<20)"`

### Requirement: Engagement gate filter
The engagement stage SHALL FAIL an item if `item.reactions < filters.engagement.min_reactions` OR `item.comments < filters.engagement.min_comments` OR `item.shares < filters.engagement.min_shares`. Zero values in filters mean "no minimum" (always pass).

#### Scenario: Engagement threshold met
- **WHEN** `min_reactions=2`, item has `reactions=5`, `min_comments=0`
- **THEN** PASS

#### Scenario: Below minimum reactions
- **WHEN** `min_reactions=2`, item has `reactions=1`
- **THEN** FAIL with signal `"engagement:reactions_below(1<2)"`

### Requirement: Language filter
The language stage SHALL FAIL an item if the detected language is not in `filters.language`. Language detection uses a fast n-gram heuristic (no external API call). If `filters.language` is empty, all languages pass.

#### Scenario: Vietnamese item passes Vietnamese-only filter
- **WHEN** `filters.language=["vi"]` and item content is Vietnamese text
- **THEN** PASS

#### Scenario: English item fails Vietnamese-only filter
- **WHEN** `filters.language=["vi"]` and item is predominantly English text
- **THEN** FAIL with signal `"language:detected_en_not_in_[vi]"`

### Requirement: Intent match scorer
The intent match stage SHALL compute a TF-IDF overlap score between item content tokens and `crawl_plan.query.keywords`. FAIL if score < `CRAWL_INTENT_MATCH_THRESHOLD` (default 0.2, configurable via env var).

#### Scenario: High-overlap item passes
- **WHEN** keywords are ["ship hàng mỹ", "order mỹ phẩm"] and item content heavily discusses ordering US goods
- **THEN** score > 0.2 → PASS

### Requirement: Quality threshold filter
The quality stage SHALL FAIL an item if: (a) `item.author.profile_url` is empty (unattributed content), OR (b) `item.timestamp` is outside `crawl_plan.time_range` (before `from` or after `to`).

#### Scenario: Missing author URL fails
- **WHEN** item has `author.profile_url = ""`
- **THEN** FAIL with signal `"quality:no_author_url"`

#### Scenario: Out-of-range timestamp fails
- **WHEN** item `timestamp = "2025-12-01"` and `time_range.from = "2026-01-01"`
- **THEN** FAIL with signal `"quality:timestamp_out_of_range"`

### Requirement: Deduplication check (separate from filter)
The system SHALL implement `DuplicateChecker.IsNew(item RawItem, orgID int64) bool` that computes `dedup_hash = sha256(item.content + item.author_profile_url)` and queries `SELECT 1 FROM posts WHERE dedup_hash=? AND org_id=?`. Returns `true` if no row exists (item is new). This is called only on filter-passing items.

#### Scenario: New item passes dedup
- **WHEN** no `posts` row exists with the computed `dedup_hash` for the org
- **THEN** `IsNew` returns `true`; item is accumulated

#### Scenario: Duplicate item discarded
- **WHEN** a `posts` row with the same `dedup_hash` and `org_id` already exists
- **THEN** `IsNew` returns `false`; item is discarded; `stats.dedup_count` is incremented; no second DB write is made

### Requirement: Rejected items are never stored
No item that fails `FilterEngine.Evaluate` or `DuplicateChecker.IsNew` SHALL be written to any database table, log file, or result payload.

#### Scenario: Filtered item leaves no trace
- **WHEN** an item fails the spam filter
- **THEN** the item is not written to `posts`, `leads`, or `tasks.result_json`; only `stats.total_fetched` is incremented (internal observability only, not user-visible)
