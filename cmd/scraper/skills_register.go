package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/thg/scraper/internal/ai"
	"github.com/thg/scraper/internal/jobs"
	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/skills"
	"github.com/thg/scraper/internal/store"
)

// builtinSkillDeps groups every dependency the registered skill
// closures capture at boot. It mirrors the signature of
// makeAgentActionHandler so a skill's Run can simply re-route into the
// existing action handler — the typed Skill is a thin metadata wrapper
// over the production handler logic, not a re-implementation.
type builtinSkillDeps struct {
	db       *store.Store
	jobStore *jobs.Store
	msgGen   *ai.MessageGenerator
	notify   func(string)

	// handler is the existing makeAgentActionHandler closure. Skills
	// re-route into it so business logic lives in exactly one place.
	handler func(action string, args map[string]any) (string, error)
}

// registerBuiltinSkills populates reg with the canonical Phase 6 skill
// catalog. Called once at boot from cmd/scraper/main.go after the
// action handler has been built. Panics on duplicate registration —
// duplicates indicate a developer copy-paste error, not a runtime
// condition.
func registerBuiltinSkills(reg *skills.Registry, deps builtinSkillDeps) {
	// ---- scrape ----------------------------------------------------------------
	reg.Register(&skills.Skill{
		ID:             "scrape_group",
		Title:          "Cào bài viết Facebook (group/post URL)",
		Description:    "Cào bài từ một group hoặc một post Facebook khi user đã cung cấp URL cụ thể. Không phải dùng để publish comment.",
		Category:       skills.CategoryScrape,
		DefaultEnabled: true,
		Parameters: []skills.SkillParam{
			{Name: "url", Type: "url", Required: true, MaxLen: 1024, Description: "Facebook group hoặc post URL"},
			{Name: "max_items", Type: "int", Description: "Số bài tối đa muốn cào"},
			{Name: "account_id", Type: "int", Description: "Account workspace; trống = auto-pick"},
		},
		Run: skillThroughHandler("scrape_group", deps),
	})

	reg.Register(&skills.Skill{
		ID:             "scrape_comments",
		Title:          "Cào comments của một post",
		Description:    "Đọc comments của 1 post Facebook để phân tích lead. KHÔNG dùng để publish comment mới.",
		Category:       skills.CategoryScrape,
		DefaultEnabled: true,
		Parameters: []skills.SkillParam{
			{Name: "post_url", Type: "url", Required: true, MaxLen: 1024, Description: "Facebook post URL"},
			{Name: "max_items", Type: "int", Description: "Số comments tối đa"},
			{Name: "account_id", Type: "int", Description: "Account workspace"},
		},
		Run: skillThroughHandler("scrape_comments", deps),
	})

	reg.Register(&skills.Skill{
		ID:             "search_groups",
		Title:          "Tìm Facebook source phù hợp",
		Description:    "Khám phá group/page Facebook phù hợp khi user mô tả nhóm khách mục tiêu nhưng không cho URL cụ thể.",
		Category:       skills.CategoryScrape,
		DefaultEnabled: true,
		Parameters: []skills.SkillParam{
			{Name: "query", Type: "string", Required: true, MaxLen: 256, Description: "Từ khoá tìm kiếm suy luận từ prompt + business profile"},
			{Name: "max_items", Type: "int", Description: "Số source tối đa"},
			{Name: "account_id", Type: "int"},
		},
		Run: skillThroughHandler("search_groups", deps),
	})

	// ---- comment ---------------------------------------------------------------
	reg.Register(&skills.Skill{
		ID:             "auto_comment",
		Title:          "Đăng comment cho 1 post cụ thể",
		Description:    "Queue một comment cho duy nhất 1 post Facebook. Mặc định draft trừ khi org đã bật outbound_mode=auto.",
		Category:       skills.CategoryComment,
		Outbound:       true,
		DefaultEnabled: true,
		Parameters: []skills.SkillParam{
			{Name: "post_url", Type: "url", Required: true, MaxLen: 1024, Description: "Target post URL"},
			{Name: "account_id", Type: "int", Description: "Account workspace"},
			{Name: "context", Type: "string", MaxLen: 4000, Description: "Nội dung post nếu có"},
			{Name: "target_name", Type: "string", MaxLen: 200, Description: "Tên author"},
			{Name: "auto", Type: "bool", Description: "Caller xin auto-execute; bị downgrade về draft nếu org chưa bật"},
		},
		Run: skillThroughHandler("auto_comment", deps),
	})

	reg.Register(&skills.Skill{
		ID:             "comment_all_leads",
		Title:          "Comment hàng loạt cho leads đủ tiêu chí",
		Description:    "Queue comment cho mọi lead đã được classify, qua dedup/cooldown/approval guardrails của store.",
		Category:       skills.CategoryComment,
		Outbound:       true,
		DefaultEnabled: true,
		Parameters: []skills.SkillParam{
			{Name: "template", Type: "string", MaxLen: 2000, Description: "Comment template tuỳ chọn"},
			{Name: "score_filter", Type: "enum", Enum: []string{"hot", "warm", "cold", "all"}, Description: "Bộ lọc lead score"},
			{Name: "account_id", Type: "int"},
			{Name: "auto", Type: "bool"},
		},
		Run: skillThroughHandler("comment_all_leads", deps),
	})

	// ---- inbox -----------------------------------------------------------------
	reg.Register(&skills.Skill{
		ID:             "auto_inbox",
		Title:          "Gửi inbox Messenger cho 1 lead cụ thể",
		Description:    "Queue 1 inbox message cho 1 lead. Mặc định draft trừ khi org đã bật outbound_mode=auto.",
		Category:       skills.CategoryInbox,
		Outbound:       true,
		DefaultEnabled: true,
		Parameters: []skills.SkillParam{
			{Name: "target_url", Type: "url", Required: true, MaxLen: 1024, Description: "Profile / Messenger target URL"},
			{Name: "account_id", Type: "int"},
			{Name: "context", Type: "string", MaxLen: 4000, Description: "Lead context"},
			{Name: "target_name", Type: "string", MaxLen: 200},
			{Name: "auto", Type: "bool"},
		},
		Run: skillThroughHandler("auto_inbox", deps),
	})

	reg.Register(&skills.Skill{
		ID:             "inbox_all_leads",
		Title:          "Inbox hàng loạt cho leads đủ tiêu chí (first-touch sales)",
		Description:    "First-touch SALES outreach (không phải customer-service follow-up): queue inbox cho mọi lead đã classify đạt `score_filter` (mặc định `hot`). Áp dụng thread state + cooldown + approval. Customer-service reply trên thread đã có inbound message phải dùng skill khác — `awaiting_reply_cooldown` của conversation gate sẽ chặn lặp lại trên cùng thread.",
		Category:       skills.CategoryInbox,
		Outbound:       true,
		DefaultEnabled: true,
		Parameters: []skills.SkillParam{
			{Name: "score_filter", Type: "enum", Enum: []string{"hot", "warm", "cold", "all"}, Description: "Lead score filter; mặc định hot (first-touch sales). Đặt 'all' chỉ khi explicit sales blast."},
			{Name: "account_id", Type: "int"},
			{Name: "auto", Type: "bool"},
		},
		Run: skillThroughHandler("inbox_all_leads", deps),
	})

	// ---- post ------------------------------------------------------------------
	reg.Register(&skills.Skill{
		ID:             "create_job_post",
		Title:          "Soạn & queue một post lên group",
		Description:    "Queue draft cho 1 post lên group Facebook. AI tự sinh content từ user request + business profile khi cần.",
		Category:       skills.CategoryPost,
		Outbound:       true,
		DefaultEnabled: true,
		Parameters: []skills.SkillParam{
			{Name: "title", Type: "string", MaxLen: 200, Description: "Tiêu đề / chủ đề post"},
			{Name: "description", Type: "string", MaxLen: 4000, Description: "Tóm tắt brief"},
			{Name: "content", Type: "string", MaxLen: 8000, Description: "Full content nếu có sẵn"},
			{Name: "group_url", Type: "url", MaxLen: 1024, Description: "Target group URL nếu user chỉ định"},
			{Name: "account_id", Type: "int"},
			{Name: "auto", Type: "bool"},
		},
		Run: skillThroughHandler("create_job_post", deps),
	})

	// ---- admin -----------------------------------------------------------------
	reg.Register(&skills.Skill{
		ID:             "describe_business",
		Title:          "Lưu mô tả doanh nghiệp",
		Description:    "Lưu org-scoped business context: brand, services, target customers, tone, reject rules, approval policy.",
		Category:       skills.CategoryAdmin,
		DefaultEnabled: true,
		Parameters: []skills.SkillParam{
			{Name: "description", Type: "string", Required: true, MaxLen: 8000, Description: "Mô tả doanh nghiệp / workspace"},
		},
		Run: skillThroughHandler("describe_business", deps),
	})

	reg.Register(&skills.Skill{
		ID:             "set_context",
		Title:          "Lưu context org-scoped (private files / data sources / business profile)",
		Description:    "Lưu org-scoped configuration: business_profile, private_files_summary, data_sources_summary. KHÔNG dùng cho outbound_mode/auto_comment_mode (server reject).",
		Category:       skills.CategoryAdmin,
		DefaultEnabled: true,
		Parameters: []skills.SkillParam{
			{Name: "key", Type: "enum", Enum: []string{"business_profile", "private_files_summary", "data_sources_summary"}, Required: true},
			{Name: "value", Type: "string", Required: true, MaxLen: 16000},
		},
		Run: skillThroughHandler("set_context", deps),
	})

	reg.Register(&skills.Skill{
		ID:             "get_stats",
		Title:          "Đọc stats workspace",
		Description:    "Đọc số lượng posts / leads / hot leads / running jobs.",
		Category:       skills.CategoryAdmin,
		DefaultEnabled: true,
		Parameters:     []skills.SkillParam{},
		Run:            skillThroughHandler("get_stats", deps),
	})

	reg.Register(&skills.Skill{
		ID:             "add_group",
		Title:          "Đăng ký một Facebook source cho org",
		Description:    "Thêm group/page URL vào source catalog của org để dùng cho các lần crawl sau.",
		Category:       skills.CategoryAdmin,
		DefaultEnabled: true,
		Parameters: []skills.SkillParam{
			{Name: "url", Type: "url", Required: true, MaxLen: 1024},
			{Name: "name", Type: "string", Required: true, MaxLen: 200},
		},
		Run: skillThroughHandler("add_group", deps),
	})

	reg.Register(&skills.Skill{
		ID:             "classify_leads",
		Title:          "Confirm classification chạy inline",
		Description:    "Xác nhận classification chạy inline trong mỗi crawl job dùng business context hiện tại.",
		Category:       skills.CategoryAdmin,
		DefaultEnabled: true,
		Run:            skillThroughHandler("classify_leads", deps),
	})

	// ---- new skills (Phase 6.3 — scaffolds) ------------------------------------
	registerScaffoldSkills(reg, deps)
}

// skillThroughHandler builds a Run closure that funnels into the existing
// action handler. Adds env metadata (org_id, account_id, user_prompt)
// before delegating, then converts the string summary into a
// SkillResult. Errors propagate as-is.
func skillThroughHandler(actionID string, deps builtinSkillDeps) skills.SkillRun {
	return func(ctx context.Context, env skills.Env, args map[string]any) (skills.SkillResult, error) {
		if args == nil {
			args = map[string]any{}
		}
		// Inject env values the handler expects without trusting any
		// matching keys the LLM might have produced. user_id + user_role
		// are SERVER-SIDE truth — even if the LLM tried to set them in
		// args, we overwrite with the authenticated caller's identity.
		// See feedback_shared_battlefield_not_crm.md.
		args["org_id"] = env.OrgID
		args["user_id"] = env.UserID
		args["user_role"] = env.Role
		if env.AccountID > 0 {
			if existing, ok := args["account_id"].(int64); !ok || existing == 0 {
				args["account_id"] = env.AccountID
			}
		}
		if env.Prompt != "" && args["user_prompt"] == nil {
			args["user_prompt"] = env.Prompt
		}
		if deps.handler == nil {
			return skills.SkillResult{}, fmt.Errorf("skill %q: action handler not wired", actionID)
		}
		summary, err := deps.handler(actionID, args)
		return skills.SkillResult{
			Summary: summary,
			Data: map[string]any{
				"action_id": actionID,
			},
		}, err
	}
}

// ---- Phase 6.3 scaffolds --------------------------------------------------------

// registerScaffoldSkills installs the three new capabilities the user
// asked for (Messenger fanpage scanning, fanpage maintenance, posting
// to one's own profile). Fanpage care/inbox remain scaffolds until the
// Chrome Extension has dedicated page-inbox adapters. post_to_profile
// already uses the shared outbound queue with a distinct profile_post type.
//
// SCAFFOLD STATUS: scan_fanpage_inbox + care_fanpage are non-functional
// until the Chrome Extension ships fanpage-inbox / fanpage-care adapters
// (phase_gate = chrome_extension_fanpage_adapter). They register so the
// dashboard / Telegram skill catalog shows them, but Run() returns a
// "scaffold ready" summary instead of driving Chrome. post_to_profile is
// LIVE — it routes through the shared outbound queue.
func registerScaffoldSkills(reg *skills.Registry, deps builtinSkillDeps) {
	reg.Register(&skills.Skill{
		ID:             "scan_fanpage_inbox",
		Title:          "Quét tin nhắn Messenger của fanpage",
		Description:    "Mở Messenger của fanpage, lấy thread chưa đọc, classify và queue draft trả lời. Ưu tiên thread mà lead vừa rep.",
		Category:       skills.CategoryInbox,
		Outbound:       true,
		NeedsAccount:   true,
		DefaultEnabled: true,
		Parameters: []skills.SkillParam{
			{Name: "page_url", Type: "url", Required: true, MaxLen: 1024, Description: "Fanpage URL (ví dụ https://www.facebook.com/yourpage)"},
			{Name: "since_minutes", Type: "int", Description: "Chỉ lấy thread mới trong N phút (mặc định 60)", Default: int64(60)},
		},
		Run: scaffoldFanpageInboxRun(deps),
	})

	reg.Register(&skills.Skill{
		ID:             "care_fanpage",
		Title:          "Bảo trì fanpage (pin/react/lịch đăng)",
		Description:    "Routine bảo trì cho fanpage: pin post, react bài mới, đăng lịch. KHÔNG outbound tới lead — chỉ housekeeping nội bộ fanpage.",
		Category:       skills.CategoryCare,
		NeedsAccount:   true,
		DefaultEnabled: true,
		Parameters: []skills.SkillParam{
			{Name: "page_url", Type: "url", Required: true, MaxLen: 1024},
			{Name: "action", Type: "enum", Enum: []string{"pin_post", "react_recent", "scheduled_post"}, Required: true, Description: "Hành động muốn thực thi"},
			{Name: "target_url", Type: "url", MaxLen: 1024, Description: "URL post target khi action = pin_post"},
			{Name: "content", Type: "string", MaxLen: 8000, Description: "Nội dung khi action = scheduled_post"},
		},
		Run: scaffoldFanpageCareRun(deps),
	})

	reg.Register(&skills.Skill{
		ID:             "post_to_profile",
		Title:          "Đăng post lên timeline profile cá nhân",
		Description:    "Queue một post lên timeline profile cá nhân (KHÁC group_post). Giống create_job_post nhưng target là profile.",
		Category:       skills.CategoryPost,
		Outbound:       true,
		NeedsAccount:   true,
		DefaultEnabled: true,
		Parameters: []skills.SkillParam{
			{Name: "content", Type: "string", Required: true, MaxLen: 8000, Description: "Nội dung post"},
			{Name: "auto", Type: "bool"},
		},
		Run: profilePostRun(deps),
	})
}

// scaffoldFanpageInboxRun records an audit row and surfaces a
// human-readable note that this skill is gated on a dedicated Chrome
// Extension fanpage-inbox adapter. It does NOT yet drive Chrome.
func scaffoldFanpageInboxRun(deps builtinSkillDeps) skills.SkillRun {
	return func(ctx context.Context, env skills.Env, args map[string]any) (skills.SkillResult, error) {
		pageURL, _ := args["page_url"].(string)
		if strings.TrimSpace(pageURL) == "" {
			return skills.SkillResult{}, fmt.Errorf("scan_fanpage_inbox requires page_url")
		}
		// Smoke test: confirm the org has at least one logged-in account so
		// the scaffold message reflects whether the live skill could run.
		if env.OrgID > 0 && deps.db != nil {
			if accounts, err := deps.db.GetAllAccounts(env.OrgID); err == nil {
				ready := 0
				for _, a := range accounts {
					if a.Platform == models.PlatformFacebook && a.BrowserLoggedIn && a.Status == models.AccountActive {
						ready++
					}
				}
				if ready == 0 {
					return skills.SkillResult{
						Summary: fmt.Sprintf("scan_fanpage_inbox: chưa có account Facebook logged-in cho org %d. Ghép THG Chrome Extension và mở tab Facebook đã đăng nhập trước đã.", env.OrgID),
					}, nil
				}
			}
		}
		return skills.SkillResult{
			Summary: "scan_fanpage_inbox: scaffold ready. Live execution sẽ được bật sau khi có Chrome Extension fanpage-inbox adapter. Page=" + pageURL,
			Data: map[string]any{
				"page_url":      pageURL,
				"phase_gate":    "chrome_extension_fanpage_adapter",
				"since_minutes": args["since_minutes"],
			},
		}, nil
	}
}

func scaffoldFanpageCareRun(_ builtinSkillDeps) skills.SkillRun {
	return func(ctx context.Context, env skills.Env, args map[string]any) (skills.SkillResult, error) {
		pageURL, _ := args["page_url"].(string)
		action, _ := args["action"].(string)
		if strings.TrimSpace(pageURL) == "" || strings.TrimSpace(action) == "" {
			return skills.SkillResult{}, fmt.Errorf("care_fanpage requires page_url + action")
		}
		return skills.SkillResult{
			Summary: fmt.Sprintf("care_fanpage: scaffold ready. action=%s page=%s — live execution sau khi có Chrome Extension fanpage-care adapter.", action, pageURL),
			Data: map[string]any{
				"page_url":   pageURL,
				"action":     action,
				"phase_gate": "chrome_extension_fanpage_adapter",
			},
		}, nil
	}
}

// profilePostRun reuses the shared outbound guardrails, but stores a
// distinct profile_post type so profile automation can be audited and
// executed without being confused with group posting.
func profilePostRun(deps builtinSkillDeps) skills.SkillRun {
	return func(ctx context.Context, env skills.Env, args map[string]any) (skills.SkillResult, error) {
		content, _ := args["content"].(string)
		if strings.TrimSpace(content) == "" {
			return skills.SkillResult{}, fmt.Errorf("post_to_profile requires content")
		}
		args["org_id"] = env.OrgID
		args["user_id"] = env.UserID
		args["user_role"] = env.Role
		if env.AccountID > 0 {
			args["account_id"] = env.AccountID
		}
		summary, err := deps.handler("post_to_profile", args)
		return skills.SkillResult{
			Summary: summary,
			Data: map[string]any{
				"action_id":  "post_to_profile",
				"as_skill":   "post_to_profile",
				"phase_note": "Uses the shared outbound queue with type=profile_post.",
			},
		}, err
	}
}
