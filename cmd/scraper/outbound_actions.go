package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/thg/scraper/internal/ai"
	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store"
	"github.com/thg/scraper/internal/textutil"
	knowledgeRuntime "github.com/thg/scraper/internal/workspace_knowledge/runtime"
)

// commentReasoningMode reads the P2c knowledge-reasoning switch (env, hot
// kill-switch — no redeploy to flip):
//
//	off (default) — comment generation unchanged.
//	dryrun        — compute + persist the grounded decision for observation;
//	                does NOT change the comment text.
//	live          — when the decision has a GROUNDED offer, it drives the comment
//	                text (GenerateCommentV2); knowledge_gap falls back to the
//	                existing generic generation (no regression).
//
// THG_COMMENT_REASONING_DRYRUN=1 is kept as an alias for dryrun.
func commentReasoningMode() string {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("THG_COMMENT_REASONING"))) {
	case "dryrun":
		return "dryrun"
	case "live":
		return "live"
	}
	if os.Getenv("THG_COMMENT_REASONING_DRYRUN") == "1" {
		return "dryrun"
	}
	return "off"
}

// applyCommentReasoning runs the Knowledge Intelligence reasoning for one comment
// lead. dryrun only observes; live lets a GROUNDED decision drive the comment
// text, falling back to `fallback` on knowledge_gap or any error. The decision is
// persisted for observation in BOTH modes. Best-effort: it can never break the
// queue path — every failure returns `fallback`. See
// specs/COMMENT_INTELLIGENCE_PIPELINE.md §9 (P2c).
func applyCommentReasoning(ctx context.Context, db *store.Store, kb *knowledgeRuntime.Builder, msgGen *ai.MessageGenerator, mode string, profile *ai.BusinessProfile, orgID, accountID int64, leadContent, author, fallback string) string {
	rctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	candidates, retrievalID, err := kb.CandidatesForLead(rctx, orgID, leadContent)
	if err != nil {
		log.Printf("[reasoning] candidates org=%d: %v", orgID, err)
		return fallback
	}
	decision, err := msgGen.DecideComment(rctx, leadContent, author, profile, candidates, retrievalID)
	if err != nil || decision == nil {
		log.Printf("[reasoning] decide org=%d: %v", orgID, err)
		return fallback
	}
	log.Printf("[reasoning:%s] org=%d account=%d intent=%s conf=%.2f knowledge_gap=%v caps=%d products=%d proofs=%d",
		mode, orgID, accountID, decision.Intent, decision.Confidence, decision.KnowledgeGap,
		len(decision.Selected.Capabilities), len(decision.Selected.Products), len(decision.Selected.Proofs))
	if payload, perr := json.Marshal(decision); perr == nil {
		_ = db.Prompts().InsertSystemPromptLog(orgID, accountID,
			"agent comment decision ("+mode+")", "comment_decision_"+mode, string(payload), !decision.KnowledgeGap)
	}
	if mode == "live" && !decision.KnowledgeGap {
		text, gerr := msgGen.GenerateCommentV2(rctx, leadContent, author, profile, decision)
		if gerr != nil {
			log.Printf("[reasoning:live] GenerateCommentV2 org=%d: %v — falling back", orgID, gerr)
			return fallback
		}
		if t := strings.TrimSpace(text); t != "" {
			return t // grounded decision drives the live comment text
		}
	}
	return fallback
}

func queueLeadOutreach(ctx context.Context, db *store.Store, msgGen *ai.MessageGenerator, msgType string, args map[string]any, notify func(string)) (string, error) {
	orgID := argInt64(args, "org_id")
	if orgID <= 0 {
		return "", fmt.Errorf("org_id is required for outbound automation")
	}
	userID := argInt64(args, "user_id")
	role := argString(args, "user_role")
	// Resolve the campaign-ready ActionContext (Source=manual). The queue path
	// below consumes only the context, so a future ResolveCampaignActionContext
	// drops in without touching this code.
	actx, err := resolveUserActionContext(db, orgID, userID, role, argInt64(args, "account_id"), true)
	if err != nil {
		return "", err
	}
	accountID := actx.AccountID

	// requestedAuto carries the AI/agent's preference. The store layer
	// (QueueOutboundForOrg -> IsAutoOutboundEnabledForOrg) is the final
	// gatekeeper: it downgrades to draft if the org has not opted in.
	requestedAuto := argBool(args, "auto")

	leads, err := leadsFromActionArgs(ctx, db, orgID, msgType, args)
	if err != nil {
		return "", err
	}
	if len(leads) == 0 {
		return "khong co lead phu hop de queue outbound", nil
	}

	businessContext := businessContextForOrg(db, orgID)
	// Knowledge OS runtime builder. Per-lead retrieval augments the
	// org-wide freeform business profile with top-K matched assets
	// (PRODUCTS, POLICIES, CTAs). When the org has not configured a
	// Knowledge OS source, BuildForLeadWithTrace returns businessContext
	// unchanged — backward compatible by construction. See
	// specs/WORKSPACE_KNOWLEDGE_OS.md §11 (Migration path).
	//
	// TraceRec wires the Operator Replay surface: each retrieval gets
	// a full Trace + Budget recorded under a retrievalID we thread into
	// the outcome event when the message is queued. Replay UI joins on
	// retrievalID to show "this lead → these assets → this outcome".
	knowledgeBuilder := knowledgeRuntime.NewBuilder(db.Knowledge())
	knowledgeBuilder.Recorder = db.Knowledge()
	knowledgeBuilder.TraceRec = db.Knowledge()
	// P2c Knowledge Intelligence reasoning (off | dryrun | live). off is the
	// default → comment generation unchanged. live lets a grounded decision drive
	// the comment text, with a safe fallback to the existing generation on
	// knowledge_gap. Hot env kill-switch — see commentReasoningMode.
	reasoningMode := commentReasoningMode()
	var reasoningProfile *ai.BusinessProfile
	if reasoningMode != "off" {
		reasoningProfile = ai.LoadProfileForOrg(db, orgID)
	}
	// PR-3 brand trust: resolve the org's grounded company identity once so the
	// per-comment contact policy (ScreenCommentContacts) can reject any
	// fabricated / non-grounded website / email / phone.
	var commentIdentity models.CompanyIdentity
	if msgType == "comment" {
		idProfile := reasoningProfile
		if idProfile == nil {
			idProfile = ai.LoadProfileForOrg(db, orgID)
		}
		commentIdentity = ai.ResolveCompanyIdentity(idProfile, nil)
	}
	template := argString(args, "template")
	queued, skipped := 0, 0
	approvedCount := 0
	skipReasons := map[string]int{}
	var lastGenErr error
	// riskBlockDetail captures the operator-actionable inputs of the
	// LAST risk_ceiling_exceeded deny so the response/notification can
	// surface "account=N risk=X ceiling=Y" inline. Without this the
	// operator sees only the reason tag and must run the superadmin
	// diagnostic separately to find out which account + how far over.
	// All deny iterations in a single run share the resolved accountID
	// (resolved once above), so capturing the latest value is sufficient.
	var riskBlockSeen bool
	var riskBlockRisk, riskBlockCeiling float64
	// Eligible-fill (PR-2): "comment thử 5 lead" means QUEUE 5 eligible comments —
	// keep scanning the candidate pool past skipped leads until `requested` are
	// queued or the pool is exhausted, instead of inspecting exactly N raw leads.
	requested := requestedOutreachCount(args)
	scanned := 0
	// Coverage policy: brand-coverage-friendly default until a per-org settings
	// surface exists (allow multi-actor, cap accounts/url/cta, gap, stop-on-reply).
	coveragePolicy := models.DefaultCoveragePolicy()
	for _, lead := range leads {
		if queued >= requested {
			break
		}
		scanned++
		targetURL, skipReason := resolveOutboundTargetURL(lead, msgType)
		if skipReason != "" {
			skipped++
			skipReasons[skipReason]++
			continue
		}

		// Multi-actor coverage gate (spec: MULTI_ACTOR_COVERAGE_POLICY). A SHARED lead
		// may be covered by SEVERAL accounts — this is brand reach, not spam — but
		// capped: skip (and keep scanning) when THIS actor already covered it, the lead
		// replied, coverage is full, or it is too soon behind the previous actor.
		var persona models.ActorPersona
		if msgType == "comment" && lead.ID > 0 {
			if cov, cerr := db.Leads().GetLeadCoverageState(ctx, orgID, lead.ID, commentIdentity.Website); cerr == nil {
				if ok, reason := models.EvaluateCoverage(*cov, coveragePolicy, accountID, time.Now().UTC()); !ok {
					skipped++
					skipReasons[reason]++
					continue
				}
				// Eligible: shape this actor's comment from the CONTENT-ACCURATE coverage
				// state — no_link only if a prior comment actually cited the website,
				// experience_share only if one actually used a hard CTA, and avoid the
				// angles already present in earlier comments.
				persona = models.DeriveActorPersona(*cov, coveragePolicy, "", "")
			}
		}

		content := template
		var retrievalID string
		if msgGen != nil && msgGen.Available() {
			genCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
			// Per-lead Knowledge OS retrieval with full trace.
			// 1.5s timeout because the LIKE-based naive searcher is fast;
			// pgvector replacement should still fit comfortably.
			retrievalCtx, retrievalCancel := context.WithTimeout(ctx, 1500*time.Millisecond)
			generatedAction := msgType + "_drafted"
			var leadContext string
			leadContext, retrievalID = knowledgeBuilder.BuildForLeadWithTrace(retrievalCtx, orgID, lead.Content, businessContext, generatedAction)
			retrievalCancel()
			var genErr error
			if template != "" && msgType == "comment" {
				content, genErr = msgGen.GenerateCommentFromTemplate(genCtx, template, lead.Content, lead.Author)
			} else if msgType == "comment" {
				content, genErr = msgGen.GenerateCommentWithService(genCtx, lead.Content, lead.Author, leadContext, lead.ServiceMatch, commentIdentity, persona)
			} else {
				content, genErr = msgGen.GenerateInboxMessage(genCtx, lead.Content, lead.Author, leadContext, "")
			}
			cancel()
			if genErr != nil {
				log.Printf("[queueLeadOutreach] AI generation failed for lead %s: %v", targetURL, genErr)
				lastGenErr = genErr
				skipped++
				skipReasons["generation_failed"]++
				continue
			}
		}
		content = strings.TrimSpace(content)
		if content == "" {
			skipped++
			skipReasons["empty_content"]++
			continue
		}

		// P2c reasoning: in dryrun observe the grounded decision; in live let a
		// grounded decision drive the comment text (fallback to `content` on
		// knowledge_gap or error). off → no-op. Comment-type only.
		if reasoningMode != "off" && msgType == "comment" {
			content = applyCommentReasoning(ctx, db, knowledgeBuilder, msgGen, reasoningMode, reasoningProfile, orgID, accountID, lead.Content, lead.Author, content)
		}

		// PR-1 Comment Quality Hotfix: dedupe repeated sentences/paragraphs and
		// validate quality at the queue boundary — a doubled "X.X" generation (from
		// any source) must NEVER reach Facebook. Reject with a typed reason instead
		// of posting garbage.
		if msgType == "comment" {
			cleaned, ok, qreason := ai.SanitizeComment(content)
			if !ok {
				skipped++
				skipReasons[qreason]++
				continue
			}
			content = cleaned
			// Duplicate guard (incident PR-1): an A+A repeated block must never enter
			// the outbox, even if it survived sentence-level dedup. Typed reason so the
			// operator sees "Comment bị lặp" instead of garbage on Facebook.
			if ai.DetectRepeatedText(content) {
				skipped++
				skipReasons["comment_quality_duplicate_text"]++
				continue
			}
			// Brand-trust contact policy: ≤1 URL, grounded website / official contact,
			// no fabricated email/phone. On a violation, REPAIR toward the Company
			// Identity (t.me link → @handle, drop non-grounded URLs) and re-screen;
			// only drop the lead if the repaired comment still fails.
			if cok, creason := ai.ScreenCommentContacts(content, commentIdentity); !cok {
				repaired, changed := ai.RepairCommentContacts(content, commentIdentity)
				rok, rreason := ai.ScreenCommentContacts(repaired, commentIdentity)
				if changed && rok {
					content = repaired
				} else {
					skipped++
					if changed {
						skipReasons[rreason]++
					} else {
						skipReasons[creason]++
					}
					continue
				}
			}
		}

		result, err := db.QueueOutboundForOrg(&models.OutboundMessage{
			OrgID:      orgID,
			Type:       msgType,
			Platform:   models.PlatformFacebook,
			AccountID:  accountID,
			TargetURL:  targetURL,
			TargetName: lead.Author,
			Content:    content,
			Context:    lead.Content,
			AIModel:    "agent",
			CreatedBy:  actx.InitiatorUserID, // immutable execution ownership (from ActionContext)
		}, 24*time.Hour)
		if err != nil {
			return "", err
		}
		if !result.Decision.Allowed {
			skipped++
			skipReasons[result.Decision.Reason]++
			if result.Decision.Reason == "risk_ceiling_exceeded" && result.Decision.RiskCeiling > 0 {
				riskBlockSeen = true
				riskBlockRisk = result.Decision.RiskScore
				riskBlockCeiling = result.Decision.RiskCeiling
			}
			// Record the rejection outcome so the Operator Replay UI
			// shows "retrieved → drafted → rejected (reason)" instead
			// of leaving the retrieval event dangling.
			if retrievalID != "" {
				db.Knowledge().RecordOutcome(ctx, orgID, retrievalID, "rejected")
			}
			continue
		}
		queued++
		if result.ExecutionState == models.ExecPlanned {
			approvedCount++
		}
		// Stage outcome: queue success. The downstream browser-execution
		// layer is responsible for the FINAL "sent" / "failed" outcome
		// against the same retrievalID — that's where image attachments
		// (Phase E) and DOM verification land.
		if retrievalID != "" {
			db.Knowledge().RecordOutcome(ctx, orgID, retrievalID, "queued")
		}
	}

	mode := "draft"
	switch {
	case approvedCount > 0 && approvedCount == queued:
		mode = "approved_auto"
	case approvedCount > 0:
		mode = "mixed"
	case requestedAuto:
		// Caller asked for auto but the org is not opted in; make this
		// visible in the response so the operator knows why it queued as draft.
		mode = "draft_org_not_auto"
	}
	if notify != nil && queued > 0 {
		notify(formatOutboundNotification(orgID, accountID, msgType, queued, skipped, mode))
	}

	errDetails := ""
	if lastGenErr != nil {
		errDetails = fmt.Sprintf(" | Last Error: %v", lastGenErr)
	}
	riskDetails := ""
	if riskBlockSeen {
		riskDetails = fmt.Sprintf(" risk_block=account=%d,risk_score=%.3f,effective_ceiling=%.3f", accountID, riskBlockRisk, riskBlockCeiling)
	}

	if msgType == "comment" {
		// Business-friendly: queued ≠ posted. Make clear the system will run on
		// ready Facebook accounts and report each result; surface a status summary.
		// (Submit/verify happens per-account; success is reported only when verified.)
		skipNote := ""
		if skipped > 0 {
			skipNote = fmt.Sprintf(" Bỏ qua %d lead (%s).", skipped, friendlySkipReasons(skipReasons))
		}
		// Eligible-fill semantics: report leads SCANNED vs comments QUEUED so the
		// operator sees "queued 5 after scanning 32", not "checked exactly 5".
		if queued == 0 {
			// Lead Lifecycle PR-5: degrade honestly — report what the org DOES have
			// (waiting/follow-up/archived) and a next step, not a dead-end "0 queued".
			return noEligibleCommentMessage(ctx, db, orgID, scanned, skipNote) + errDetails, nil
		}
		// PR-5: name the source group ("Cần xử lý") so the operator knows selection came
		// from the act-now work queue, not the raw lead list.
		return fmt.Sprintf(
			"Đã đưa %d comment vào hàng đợi từ nhóm Cần xử lý sau khi quét %d lead. Đây CHƯA phải là đã đăng lên Facebook — hệ thống sẽ chạy bằng các tài khoản Facebook sẵn sàng và báo lại từng kết quả. Tóm tắt: %d đang chờ · 0 đang chạy · 0 đã đăng · 0 thất bại.%s%s",
			queued, scanned, queued, skipNote, errDetails,
		), nil
	}
	return fmt.Sprintf("queued_%s=%d skipped=%d mode=%s reasons=%v%s%s", msgType, queued, skipped, mode, skipReasons, riskDetails, errDetails), nil
}

// friendlySkipReasons summarizes skip reason codes for a customer-facing message.
func friendlySkipReasons(reasons map[string]int) string {
	if len(reasons) == 0 {
		return "không đủ điều kiện"
	}
	label := map[string]string{
		"no_target_url":                  "thiếu link bài viết",
		"missing_target_url":             "thiếu link bài viết",
		"empty_content":                  "không soạn được nội dung",
		"generation_failed":              "lỗi soạn nội dung",
		"comment_quality_invalid":        "không đạt kiểm tra chất lượng",
		"comment_quality_duplicate_text": "nội dung bị lặp",
		"comment_multiple_urls":          "comment có nhiều liên kết",
		"comment_unsupported_contact":    "comment có liên hệ chưa xác minh",
		"account_cooldown_active":        "tài khoản đang nghỉ an toàn",
		"daily_limit_exceeded":           "đã đạt giới hạn hôm nay",
		"risk_ceiling_exceeded":          "tài khoản đang ở chế độ bảo vệ",
		"actor_mismatch_blocked":         "tài khoản đăng nhập nhầm Facebook",
		// Coordination / dedup guards — the common "0 queued" causes.
		"outbound_cooldown_active":       "đã gửi tới lead này gần đây (chờ hết 24h)",
		"duplicate_outbound_target_race": "đã có comment đang xếp hàng cho bài này",
		"awaiting_reply_cooldown":        "đang chờ lead phản hồi lần trước",
		"lead_replied":                   "lead đã trả lời — không gửi thêm",
		"conversation_closed":            "hội thoại với lead đã đóng",
		// Multi-actor coverage gate (brand coverage, not spam).
		"already_commented_by_this_actor": "tài khoản này đã comment lead này",
		"single_actor_policy":             "chính sách chỉ 1 tài khoản/lead",
		"coverage_full":                   "lead đã đủ số tài khoản tiếp cận",
		"coverage_gap_too_soon":           "chưa đủ giãn cách giữa các lượt comment",
		"action_policy_missing":          "workspace chưa bật chính sách hành động",
		// Target-URL resolution (resolveOutboundTargetURL) — the common skip for a
		// fresh lead whose crawled URL is not a direct commentable post permalink.
		"missing_post_permalink": "lead chưa có link bài viết comment được (URL không phải permalink bài post)",
		"missing_target":         "lead thiếu link nguồn",
		"unrouted_source_type":   "loại nguồn của lead không hỗ trợ comment",
	}
	parts := make([]string, 0, len(reasons))
	for code, n := range reasons {
		name := label[code]
		if name == "" {
			name = "cần kiểm tra"
		}
		// Incident forensics: keep the raw code in brackets so the exact skip gate is
		// unambiguous in the copilot message (no guessing which guard fired).
		parts = append(parts, fmt.Sprintf("%s [%s] ×%d", name, code, n))
	}
	return strings.Join(parts, ", ")
}

func leadsFromActionArgs(ctx context.Context, db *store.Store, orgID int64, msgType string, args map[string]any) ([]models.Lead, error) {
	if msgType == "comment" {
		if target := textutil.FirstNonEmpty(argString(args, "post_url"), argString(args, "target_url")); target != "" {
			return []models.Lead{{
				OrgID:      orgID,
				SourceURL:  target,
				Author:     argString(args, "target_name"),
				AuthorURL:  argString(args, "author_url"),
				Content:    argString(args, "context"),
				Score:      models.LeadHot,
				Platform:   models.PlatformFacebook,
				SourceType: "prompt_target",
			}}, nil
		}
	} else if target := argString(args, "target_url"); target != "" {
		return []models.Lead{{
			OrgID:      orgID,
			AuthorURL:  target,
			Author:     argString(args, "target_name"),
			Content:    argString(args, "context"),
			Score:      models.LeadHot,
			Platform:   models.PlatformFacebook,
			SourceType: "prompt_target",
		}}, nil
	}
	score := argString(args, "score_filter")
	if score == "" && msgType == "inbox" {
		score = "hot"
	}
	// Lead Lifecycle PR-2: select from the WORK QUEUE, not the raw lead list —
	// lifecycle-filtered to act-now leads (active/followup_due; archived + stale
	// excluded) and ordered by score → freshness → next_action_at. Still a LARGER pool
	// than the requested count for eligible-fill: the planner keeps scanning past
	// coverage-skipped leads until it has queued `requested`. See
	// specs/LEAD_LIFECYCLE_WORK_QUEUE.md.
	return db.Leads().WorkQueueLeads(ctx, orgID, score, scanPoolFor(requestedOutreachCount(args)))
}

// requestedOutreachCount is how many ELIGIBLE comments/messages the caller asked to
// queue ("comment thử 5 lead" → 5). Reads limit, then the agent's max_items
// fallback; defaults to 25.
func requestedOutreachCount(args map[string]any) int {
	n := int(argInt64(args, "limit"))
	if n <= 0 {
		n = int(argInt64(args, "max_items"))
	}
	if n <= 0 {
		n = 25
	}
	return n
}

// scanPoolFor sizes the candidate pool so the planner can keep scanning past skipped
// leads until it has queued `requested` eligible comments — max(50, requested*10).
func scanPoolFor(requested int) int {
	if n := requested * 10; n > 50 {
		return n
	}
	return 50
}

// resolveOutboundTargetURL maps a lead + msgType to the canonical target URL
// the outbound queue should hit. Returns ("", skipReason) when the lead is
// not actionable. Branches on SourceType explicitly so unknown values cannot
// silently fall through to SourceURL (per feedback_deterministic_boundaries).
//
// Routing contract (see models.Lead):
//   - SourceURL is ALWAYS the parent post URL for SourceType in
//     {post, comment, prompt_target}.
//   - SecondaryURL is the comment URL (reply-to-comment, future feature).
//   - For inbox msgType, target is the participant's AuthorURL — SourceURL
//     is not consulted.
//
// PostFBID fallback: if SourceURL is a transient form (photo viewer, share
// shim) that isCommentableFacebookPostURL rejects but post_fbid is present
// with a known group, reconstruct the canonical /groups/<g>/posts/<p>/ URL.
func resolveOutboundTargetURL(lead models.Lead, msgType string) (string, string) {
	if msgType == "inbox" {
		if t := strings.TrimSpace(lead.AuthorURL); t != "" {
			return t, ""
		}
		return "", "missing_target"
	}
	switch strings.ToLower(strings.TrimSpace(lead.SourceType)) {
	case "", "post", "comment", "prompt_target":
		target := strings.TrimSpace(lead.SourceURL)
		if msgType == "comment" && !isCommentableFacebookPostURL(target) {
			if rebuilt := canonicalGroupPostURLFromFBIDs(lead.GroupFBID, lead.PostFBID); rebuilt != "" {
				return rebuilt, ""
			}
			return "", "missing_post_permalink"
		}
		if target == "" {
			return "", "missing_target"
		}
		return target, ""
	default:
		return "", "unrouted_source_type"
	}
}

// canonicalGroupPostURLFromFBIDs reconstructs the canonical group-post URL
// from the routing contract's group_fbid + post_fbid. Returns "" if either
// is missing. Used only as a fallback when SourceURL fails the commentable
// check — photo viewer / share shim / story redirect forms still carry the
// real post_fbid via the crawler's URL repair path.
func canonicalGroupPostURLFromFBIDs(groupFBID, postFBID string) string {
	g := strings.TrimSpace(groupFBID)
	p := strings.TrimSpace(postFBID)
	if g == "" || p == "" {
		return ""
	}
	return fmt.Sprintf("https://www.facebook.com/groups/%s/posts/%s/", g, p)
}

func isCommentableFacebookPostURL(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return false
	}
	host := strings.ToLower(u.Host)
	if host != "fb.watch" && !strings.HasSuffix(host, ".fb.watch") &&
		host != "facebook.com" && !strings.HasSuffix(host, ".facebook.com") {
		return false
	}
	path := strings.ToLower(strings.TrimSpace(u.EscapedPath()))
	if (host == "fb.watch" || strings.HasSuffix(host, ".fb.watch")) && strings.Trim(path, "/") != "" {
		return true
	}
	query := u.Query()
	if query.Get("story_fbid") != "" || query.Get("multi_permalinks") != "" {
		return true
	}
	if strings.Contains(path, "/posts/") ||
		strings.Contains(path, "/permalink/") ||
		strings.Contains(path, "/videos/") ||
		strings.Contains(path, "/reel/") ||
		strings.Contains(path, "/watch/") ||
		strings.Contains(path, "/share/") {
		return true
	}
	if strings.HasSuffix(path, "/photo.php") && query.Get("fbid") != "" {
		return true
	}
	return false
}

func queueGroupPost(ctx context.Context, db *store.Store, msgGen *ai.MessageGenerator, args map[string]any, notify func(string)) (string, error) {
	targets := []string{}
	if u := argString(args, "group_url"); u != "" {
		targets = append(targets, u)
	} else {
		orgID := argInt64(args, "org_id")
		groups, err := db.Crawl().GetAllGroups(orgID)
		if err != nil {
			return "", err
		}
		for _, g := range groups {
			if g.Active && strings.TrimSpace(g.URL) != "" {
				targets = append(targets, g.URL)
				if len(targets) >= 3 {
					break
				}
			}
		}
	}
	if len(targets) == 0 {
		return "khong co group target de queue group_post", nil
	}
	return queueFacebookPostTargets(ctx, db, msgGen, args, "group_post", targets, notify)
}

func queueProfilePost(ctx context.Context, db *store.Store, msgGen *ai.MessageGenerator, args map[string]any, notify func(string)) (string, error) {
	orgID := argInt64(args, "org_id")
	userID := argInt64(args, "user_id")
	role := argString(args, "user_role")
	accountID, err := resolveCallerAccountID(db, orgID, userID, role, argInt64(args, "account_id"), false)
	if err != nil {
		return "", err
	}
	// Persist the resolved+owner-checked account_id so queueFacebookPostTargets
	// uses it instead of resolving again.
	args["account_id"] = accountID
	target, skipReason := resolveProfilePostTarget(db.Identities().GetAccountForOrg, orgID, accountID, argString(args, "profile_url"))
	if skipReason != "" {
		return fmt.Sprintf("queued_profile_post=0 skipped=1 mode=skipped reasons=map[%s:1]", skipReason), nil
	}
	return queueFacebookPostTargets(ctx, db, msgGen, args, "profile_post", []string{target}, notify)
}

// accountFetcher captures the subset of *store.Store that
// resolveProfilePostTarget needs. Function-typed (not interface) so
// tests can pass a closure without standing up the full store.
type accountFetcher func(accountID, orgID int64) (*models.Account, error)

// resolveProfilePostTarget picks the explicit profile_url first, then
// falls back to the account's FBProfileURL when account lookup
// succeeds. Returns ("", "no_profile_url_resolved") when neither
// resolves — the caller MUST refuse to queue rather than implicitly
// post to /me. Dropping the /me fallback per outbound-audit #5
// closed deterministic-boundary leak: /me resolves per-logged-in
// account, so multi-account ops could cross-post identities silently.
func resolveProfilePostTarget(fetch accountFetcher, orgID, accountID int64, requestedURL string) (string, string) {
	if t := strings.TrimSpace(requestedURL); t != "" {
		return t, ""
	}
	if accountID <= 0 || fetch == nil {
		return "", "no_profile_url_resolved"
	}
	acc, err := fetch(accountID, orgID)
	if err != nil || acc == nil {
		return "", "no_profile_url_resolved"
	}
	if t := strings.TrimSpace(acc.FBProfileURL); t != "" {
		return t, ""
	}
	return "", "no_profile_url_resolved"
}

func queueFacebookPostTargets(ctx context.Context, db *store.Store, msgGen *ai.MessageGenerator, args map[string]any, msgType string, targets []string, notify func(string)) (string, error) {
	orgID := argInt64(args, "org_id")
	if orgID <= 0 {
		return "", fmt.Errorf("org_id is required for Facebook posting")
	}
	userID := argInt64(args, "user_id")
	role := argString(args, "user_role")
	accountID, err := resolveCallerAccountID(db, orgID, userID, role, argInt64(args, "account_id"), false)
	if err != nil {
		return "", err
	}

	content := textutil.FirstNonEmpty(argString(args, "content"), argString(args, "description"), argString(args, "title"))
	if msgGen != nil && msgGen.Available() && argString(args, "title") != "" {
		genCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
		generated, err := msgGen.GenerateJobPost(genCtx,
			argString(args, "title"),
			argString(args, "description"),
			argString(args, "requirements"),
			argString(args, "benefits"),
			argString(args, "salary"),
			argString(args, "email"),
		)
		cancel()
		if err == nil && strings.TrimSpace(generated) != "" {
			content = generated
		}
	}
	if strings.TrimSpace(content) == "" {
		return "", fmt.Errorf("Facebook post content is required")
	}

	requestedAuto := argBool(args, "auto")
	queued, skipped := 0, 0
	approvedCount := 0
	for _, target := range targets {
		result, err := db.QueueOutboundForOrg(&models.OutboundMessage{
			OrgID:     orgID,
			Type:      msgType,
			Platform:  models.PlatformFacebook,
			AccountID: accountID,
			TargetURL: target,
			Content:   strings.TrimSpace(content),
			AIModel:   "agent",
			CreatedBy: userID,
		}, 24*time.Hour)
		if err != nil {
			return "", err
		}
		if !result.Decision.Allowed {
			skipped++
			continue
		}
		queued++
		if result.ExecutionState == models.ExecPlanned {
			approvedCount++
		}
	}
	mode := "draft"
	switch {
	case approvedCount > 0 && approvedCount == queued:
		mode = "approved_auto"
	case approvedCount > 0:
		mode = "mixed"
	case requestedAuto:
		mode = "draft_org_not_auto"
	}
	if notify != nil && queued > 0 {
		notify(formatOutboundNotification(orgID, accountID, msgType, queued, skipped, mode))
	}
	return fmt.Sprintf("queued_%s=%d skipped=%d mode=%s", msgType, queued, skipped, mode), nil
}

// resolveCallerAccountID picks the FB account_id the skill executor will use,
// enforcing execution-layer ownership per RBAC-1 (see
// feedback_shared_battlefield_not_crm.md):
//
//   - If requestedAccountID > 0: load it and verify the caller owns it.
//     Admin / platform roles pass; sales must match acc.AssignedUserID.
//   - If requestedAccountID <= 0 and the caller is identified (userID > 0):
//     pick from the caller's OWNED accounts only (sales = GetAccountsForUser,
//     admin / platform = GetAllAccounts).
//   - If userID <= 0 (Telegram bot / legacy unauthenticated path): pick
//     from any account in the org (preserves current behaviour; future PR
//     resolves Telegram operator → DB user).
//
// preferLoggedIn rewards the first FB-platform, browser-logged-in, active
// account in the candidate list (legacy lead-outreach behaviour). Set to
// false for post / profile_post paths that don't need a logged-in browser.
// resolveUserActionContext produces the campaign-ready models.ActionContext for
// a member-initiated (Source=manual) outbound. It wraps the deterministic
// account resolution; a future resolveCampaignActionContext returns the SAME
// shape so the execution path stays source-agnostic (campaign is additive).
// ConnectorID/CampaignID/ExecutionSourceID are left 0 — filled by the future
// connector-availability + campaign layers.
func resolveUserActionContext(db *store.Store, orgID, userID int64, role string, requestedAccountID int64, preferLoggedIn bool) (models.ActionContext, error) {
	accID, err := resolveCallerAccountID(db, orgID, userID, role, requestedAccountID, preferLoggedIn)
	if err != nil {
		return models.ActionContext{}, err
	}
	return models.ActionContext{
		OrgID:           orgID,
		Source:          models.ActionSourceManual,
		InitiatorUserID: userID,
		AccountID:       accID,
	}, nil
}

func resolveCallerAccountID(db *store.Store, orgID, userID int64, role string, requestedAccountID int64, preferLoggedIn bool) (int64, error) {
	if requestedAccountID > 0 {
		acc, err := db.Identities().GetAccountForOrg(requestedAccountID, orgID)
		if err != nil || acc == nil {
			return 0, fmt.Errorf("account_id %d not found in org %d", requestedAccountID, orgID)
		}
		if userID > 0 && !models.IsAccountOwnerAllowed(acc, userID, role) {
			return 0, fmt.Errorf("you do not own account #%d", requestedAccountID)
		}
		return acc.ID, nil
	}

	var candidates []models.Account
	var err error
	if userID > 0 {
		r := models.UserRole(strings.ToLower(strings.TrimSpace(role)))
		if models.IsPlatformRole(r) || r == models.RoleAdmin {
			candidates, err = db.Identities().GetAllAccounts(orgID)
		} else {
			candidates, err = db.Identities().GetAccountsForUser(orgID, userID)
		}
	} else {
		// Legacy / unauthenticated path: any org account.
		candidates, err = db.Identities().GetAllAccounts(orgID)
	}
	if err != nil {
		return 0, err
	}
	if len(candidates) == 0 {
		if userID > 0 {
			return 0, fmt.Errorf("you have no Facebook account assigned in org %d; ask an admin to assign one", orgID)
		}
		return 0, fmt.Errorf("no Facebook account available for org %d", orgID)
	}
	if preferLoggedIn {
		// Deterministic ExecutionContext (Organic Sales Network): NO heuristic,
		// NO "first logged-in", NO newest-connector, NO auto-magic default.
		// Resolution order: explicit account_id (handled above) -> user Default
		// Account -> exactly-one-owned-account -> error execution_context_required.
		ownedIDs := make(map[int64]bool, len(candidates))
		for _, acc := range candidates {
			ownedIDs[acc.ID] = true
		}
		if def := db.GetUserDefaultAccount(orgID, userID); def > 0 && ownedIDs[def] {
			return def, nil
		}
		var usable []int64
		for _, acc := range candidates {
			if acc.Platform == models.PlatformFacebook && acc.Status == models.AccountActive {
				usable = append(usable, acc.ID)
			}
		}
		if len(usable) == 1 {
			return usable[0], nil
		}
		if len(usable) == 0 {
			return 0, fmt.Errorf("execution_context_required: no usable Facebook account — pair a Chrome connector and log into Facebook first")
		}
		return 0, fmt.Errorf("execution_context_required: you have %d Facebook accounts — set a Default Account in Settings (or pass account_id)", len(usable))
	}
	return candidates[0].ID, nil
}

func formatOutboundNotification(orgID, accountID int64, msgType string, queued, skipped int, mode string) string {
	label := "outbound"
	switch msgType {
	case "comment":
		label = "Facebook comments"
	case "inbox":
		label = "Facebook inbox"
	case "group_post":
		label = "Facebook posting"
	case "profile_post":
		label = "Facebook profile posting"
	}
	state := "drafts waiting for approval"
	if mode == "approved_auto" {
		state = "approved for Chrome Extension execution"
	}
	return fmt.Sprintf("[THG Agent] %s queued: %d (%s). Org #%d, account #%d, skipped %d by guardrails.", label, queued, state, orgID, accountID, skipped)
}
