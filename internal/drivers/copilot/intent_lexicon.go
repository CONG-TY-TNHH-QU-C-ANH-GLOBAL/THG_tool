package copilot

// Copilot routing lexicon — the keyword sets the deterministic classifier matches
// against (folded, accent-insensitive). Named here so the routing logic reads as
// intent ("comment verb + bulk scope") and so future NLU can extend a list in ONE
// place. Sets are intentionally NOT all identical — they encode existing,
// behavior-preserving differences (e.g. inbox bulk still accepts bare "lead",
// comment bulk requires an explicit plural/scope). Do not unify them without a
// deliberate behavior change + tests.

// Bulk-scope keywords shared verbatim by more than one set below. Extracted so
// each literal is defined once; the SETS themselves stay intentionally distinct
// (see the package note above) — these consts do not unify them.
const (
	lexScopeTatCa     = "tat ca"
	lexScopeTepKhach  = "tep khach"
	lexScopeKhachHang = "khach hang"
)

var (
	lexCommentVerbs = []string{"comment", "binh luan"}
	lexInboxVerbs   = []string{"inbox", "messenger", "nhan tin", "tin nhan", "dm "}
	lexCrawlVerbs   = []string{"cao", "crawl", "scrape", "quet"}
	lexPostingVerbs = []string{"dang bai", "posting", "post len", "tao bai"}

	// Inbox bulk scope (kept as-is: includes bare singular "lead").
	lexInboxBulkScope = []string{"lead", "leads", lexScopeTepKhach, lexScopeKhachHang, lexScopeTatCa, "all"}
	// Comment bulk scope (explicit only — no bare "lead"; "lead này" is single-post).
	lexCommentBulkScope = []string{"leads", "cac lead", lexScopeTepKhach, lexScopeKhachHang, lexScopeTatCa, "all"}

	// Singular phrasing → one specific post/lead (direct-link comment).
	lexSpecificScope = []string{"bai nay", "post nay", "bai viet nay", "lead nay"}
	// Bulk scope that DISqualifies the no-URL single-post branch.
	lexBulkScopeStrict = []string{lexScopeTatCa, "all", lexScopeTepKhach, lexScopeKhachHang}

	// Crawl/scrape routing verb sets (broader than the comment crawl-verb gate).
	lexScrapeVerbs = []string{"cao", "crawl", "scrape", "quet", "tim", "phan tich", "loc lead", "lead"}
	lexSearchVerbs = []string{"cao", "crawl", "scrape", "quet", "tim tep", "tim khach", "lead", "leads"}
)
