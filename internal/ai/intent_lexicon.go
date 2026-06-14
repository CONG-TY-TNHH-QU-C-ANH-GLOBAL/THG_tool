package ai

// Copilot routing lexicon — the keyword sets the deterministic classifier matches
// against (folded, accent-insensitive). Named here so the routing logic reads as
// intent ("comment verb + bulk scope") and so future NLU can extend a list in ONE
// place. Sets are intentionally NOT all identical — they encode existing,
// behavior-preserving differences (e.g. inbox bulk still accepts bare "lead",
// comment bulk requires an explicit plural/scope). Do not unify them without a
// deliberate behavior change + tests.

var (
	lexCommentVerbs = []string{"comment", "binh luan"}
	lexInboxVerbs   = []string{"inbox", "messenger", "nhan tin", "tin nhan", "dm "}
	lexCrawlVerbs   = []string{"cao", "crawl", "scrape", "quet"}
	lexPostingVerbs = []string{"dang bai", "posting", "post len", "tao bai"}

	// Inbox bulk scope (kept as-is: includes bare singular "lead").
	lexInboxBulkScope = []string{"lead", "leads", "tep khach", "khach hang", "tat ca", "all"}
	// Comment bulk scope (explicit only — no bare "lead"; "lead này" is single-post).
	lexCommentBulkScope = []string{"leads", "cac lead", "tep khach", "khach hang", "tat ca", "all"}

	// Singular phrasing → one specific post/lead (direct-link comment).
	lexSpecificScope = []string{"bai nay", "post nay", "bai viet nay", "lead nay"}
	// Bulk scope that DISqualifies the no-URL single-post branch.
	lexBulkScopeStrict = []string{"tat ca", "all", "tep khach", "khach hang"}

	// Crawl/scrape routing verb sets (broader than the comment crawl-verb gate).
	lexScrapeVerbs = []string{"cao", "crawl", "scrape", "quet", "tim", "phan tich", "loc lead", "lead"}
	lexSearchVerbs = []string{"cao", "crawl", "scrape", "quet", "tim tep", "tim khach", "lead", "leads"}
)
