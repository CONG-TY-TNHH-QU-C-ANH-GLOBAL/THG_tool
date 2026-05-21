// Domain: prompts (see internal/store/DOMAINS.md)
package prompts

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/thg/scraper/internal/store/dbutil"
)

// Watchpoint B — Prompt Routing Observability store queries.
//
// All queries are read-only and org-scoped. They project the
// routing_decision_json column from prompt_logs into structured data
// the observability dashboard renders without further JSON parsing.
//
// Heuristic identifiers used by the conflict-candidate query live in
// this file (deterministicActions / askBackActions / retryPattern) so
// the dashboard surface and the heuristics are reviewed together.

// askBackActions enumerates the action_taken values written when the
// orchestrator surfaced a clarification ask rather than executing.
// Includes both the brain-emitted ask_user/chat and the preflight blockers.
var askBackActions = []string{
	"brain_ask_user",
	"brain_chat",
	"brain_business_preflight",
	"brain_browser_preflight",
	"brain_invalid_plan",
	"business_preflight",
	"browser_preflight",
	"facebook_scope_guard",
}

// retryPattern matches the second-prompt copy that indicates the user
// was unhappy with a deterministic dispatch — used for the
// false-positive conflict candidate heuristic. Multi-lingual (EN + VN
// ASCII-folded). Tightened to whole-word boundaries so "cancel" doesn't
// match "cancellation policy" in an unrelated chat.
var retryPattern = regexp.MustCompile(`(?i)\b(cancel|stop|wrong|undo|retry|again|nope|huỷ|huy|sai|sai\s+rồi|sai\s+roi|không\s+phải|khong\s+phai|lại\s+đi|lai\s+di|thử\s+lại|thu\s+lai|khác|khac)\b`)

// PromptRoutingBucket is one cell of the route × reason distribution
// the dashboard renders.
type PromptRoutingBucket struct {
	Route      string `json:"route"`
	ReasonCode string `json:"reason_code"`
	Action     string `json:"action"` // action_taken from the row — useful for drill-down
	Count      int    `json:"count"`
}

// PromptRoutingDistribution returns counts grouped by (route, reason_code,
// action) for the requesting org over the given window. Rows with no
// routing_decision_json (legacy rows from before Watchpoint B) collapse
// into route="legacy", reason_code="unknown" so the dashboard renders
// historical data without crashing.
func (s *Store) PromptRoutingDistribution(ctx context.Context, orgID int64, since time.Time) ([]PromptRoutingBucket, error) {
	if orgID <= 0 {
		return nil, fmt.Errorf("prompt_routing: org_id required")
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT COALESCE(action_taken, '') AS action,
		        COALESCE(routing_decision_json, '{}') AS decision,
		        COUNT(*) AS n
		   FROM prompt_logs
		  WHERE org_id = ? AND created_at >= ?
		  GROUP BY action_taken, routing_decision_json`,
		orgID, since.UTC().Format("2006-01-02 15:04:05"),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	// Re-aggregate by (route, reason_code, action) in Go because grouping
	// by the full JSON blob in SQL would over-fragment (every distinct
	// reason string makes its own bucket). We extract just the two
	// stable keys.
	type key struct {
		route, reason, action string
	}
	agg := map[key]int{}
	for rows.Next() {
		var action, decision string
		var n int
		if err := rows.Scan(&action, &decision, &n); err != nil {
			return nil, err
		}
		route, reason := decodeRouteReason(decision)
		agg[key{route, reason, action}] += n
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]PromptRoutingBucket, 0, len(agg))
	for k, v := range agg {
		out = append(out, PromptRoutingBucket{Route: k.route, ReasonCode: k.reason, Action: k.action, Count: v})
	}
	// Sort descending by count so the dashboard's top-N rendering is stable.
	// Inline sort to avoid the sort package import dance.
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j].Count > out[i].Count {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out, nil
}

// PromptRoutingRow is one row of the recent-prompts feed. Includes the
// parsed RoutingDecision shape so the dashboard renders straight from
// the JSON without secondary parsing.
type PromptRoutingRow struct {
	ID                 int64                  `json:"id"`
	OrgID              int64                  `json:"org_id"`
	AccountID          int64                  `json:"account_id"`
	Source             string                 `json:"source"`
	UserPrompt         string                 `json:"user_prompt"`
	AIResponse         string                 `json:"ai_response"`
	ActionTaken        string                 `json:"action_taken"`
	Success            bool                   `json:"success"`
	CreatedAt          string                 `json:"created_at"`
	Route              string                 `json:"route"`
	ReasonCode         string                 `json:"reason_code"`
	Reason             string                 `json:"reason,omitempty"`
	SufficiencyScore   float64                `json:"sufficiency_score"`
	MissingSignals     []string               `json:"missing_signals,omitempty"`
	InferredSignals    []string               `json:"inferred_signals,omitempty"`
	DecisionRaw        map[string]any         `json:"decision_raw,omitempty"`
}

// RecentPromptRouting returns the newest-first list of prompts with
// their parsed routing decisions. Limit capped at 500. Used by the
// "Routing Reality" recent feed AND consumed in-process by the
// conflict-candidate heuristic.
func (s *Store) RecentPromptRouting(ctx context.Context, orgID int64, since time.Time, limit int) ([]PromptRoutingRow, error) {
	if orgID <= 0 {
		return nil, fmt.Errorf("prompt_routing: org_id required")
	}
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, org_id, account_id, source, user_prompt, ai_response,
		        action_taken, success, created_at,
		        COALESCE(routing_decision_json, '{}')
		   FROM prompt_logs
		  WHERE org_id = ? AND created_at >= ?
		  ORDER BY created_at DESC, id DESC
		  LIMIT ?`,
		orgID, since.UTC().Format("2006-01-02 15:04:05"), limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PromptRoutingRow
	for rows.Next() {
		var r PromptRoutingRow
		var success int
		var createdAt string
		var decision string
		if err := rows.Scan(
			&r.ID, &r.OrgID, &r.AccountID, &r.Source, &r.UserPrompt, &r.AIResponse,
			&r.ActionTaken, &success, &createdAt, &decision,
		); err != nil {
			return nil, err
		}
		r.Success = success != 0
		r.CreatedAt = dbutil.ParseSQLiteTime(createdAt).Format(time.RFC3339)
		fillDecisionFields(&r, decision)
		out = append(out, r)
	}
	return out, rows.Err()
}

// PromptRoutingConflict is one heuristic-flagged row + the evidence that
// triggered the flag. Two kinds today:
//
//   - "false_positive_deterministic" — deterministic route fired, then
//     a retry/cancel/correction prompt followed within 5 minutes from
//     the same org. The deterministic dispatch was probably wrong.
//   - "false_negative_deterministic" — brain asked back, but the prompt
//     already carried URL + crawl verb + inferred signals (i.e. the
//     boolean gate `promptIsSelfSufficient` would have routed it).
//
// Heuristic, not authoritative — every row is "investigate this", not
// "this is broken". The dashboard surfaces them; humans triage.
type PromptRoutingConflict struct {
	Kind            string           `json:"kind"`
	Row             PromptRoutingRow `json:"row"`
	FollowUpPrompt  string           `json:"follow_up_prompt,omitempty"`
	FollowUpAtRel   string           `json:"follow_up_at_rel,omitempty"`
}

// PromptRoutingConflictCandidates returns the heuristic-flagged rows in
// the window. Bounded at 50 conflicts per kind so the dashboard stays
// responsive on noisy days.
//
// Implementation:
//  1. Load recent rows (bounded fetch).
//  2. Two passes, both in Go (SQL self-joins on regex/text would be
//     fragile and SQLite-version-specific).
//  3. Return sorted by created_at DESC.
//
// `selfSufficient` is a callback so this store helper doesn't import
// internal/ai — the call site (handler) supplies the predicate.
func (s *Store) PromptRoutingConflictCandidates(
	ctx context.Context,
	orgID int64,
	since time.Time,
	selfSufficient func(prompt string) bool,
) ([]PromptRoutingConflict, error) {
	rows, err := s.RecentPromptRouting(ctx, orgID, since, 500)
	if err != nil {
		return nil, err
	}
	var conflicts []PromptRoutingConflict
	const fpLimit, fnLimit = 50, 50
	fpFound, fnFound := 0, 0

	// False-positive heuristic: deterministic route → retry-pattern
	// follow-up within 5 minutes.
	// Build a quick lookup of (account_id, time-bucketed) → next prompt.
	// rows are newest-first, so iterate newest → oldest and look BACKWARDS
	// for older deterministic rows by the same account.
	type idxKey struct {
		accountID int64
		i         int
	}
	for i, r := range rows {
		if fpFound >= fpLimit {
			break
		}
		// Look for a deterministic row OLDER than this one (j > i in
		// newest-first ordering means j is older) by the same account,
		// within 5 minutes, where THIS row contains retry-pattern text.
		if !retryPattern.MatchString(r.UserPrompt) {
			continue
		}
		thisTime, err := time.Parse(time.RFC3339, r.CreatedAt)
		if err != nil {
			continue
		}
		for j := i + 1; j < len(rows); j++ {
			cand := rows[j]
			if cand.AccountID != r.AccountID && (cand.AccountID != 0 && r.AccountID != 0) {
				continue
			}
			if cand.Route != "deterministic" {
				continue
			}
			candTime, err := time.Parse(time.RFC3339, cand.CreatedAt)
			if err != nil {
				continue
			}
			if thisTime.Sub(candTime) > 5*time.Minute {
				// Once we're past the window, no older row in this loop
				// can be inside the window — bail.
				break
			}
			conflicts = append(conflicts, PromptRoutingConflict{
				Kind:           "false_positive_deterministic",
				Row:            cand,
				FollowUpPrompt: r.UserPrompt,
				FollowUpAtRel:  formatRelativeShort(thisTime.Sub(candTime)),
			})
			fpFound++
			_ = idxKey{accountID: cand.AccountID, i: j}
			break
		}
	}

	// False-negative heuristic: brain ask-back, but prompt was actually
	// self-sufficient. Reuses the in-process gate.
	for _, r := range rows {
		if fnFound >= fnLimit {
			break
		}
		if !isAskBackAction(r.ActionTaken) {
			continue
		}
		if selfSufficient == nil || !selfSufficient(r.UserPrompt) {
			continue
		}
		conflicts = append(conflicts, PromptRoutingConflict{
			Kind: "false_negative_deterministic",
			Row:  r,
		})
		fnFound++
	}

	return conflicts, nil
}

// MissingSignalDistribution returns counts of MissingSignals across
// ask-back rows in the window — feeds the "Ambiguous Prompt Surface"
// panel's "most common missing signal" view.
type MissingSignalBucket struct {
	Signal string `json:"signal"`
	Count  int    `json:"count"`
}

// MissingSignalDistribution counts how often each signal (source / action /
// target / market / quantity) was missing across ask-back rows in the
// window. The dashboard renders this so operators can see "70% of asks
// were because users forgot to specify a source" — a recurring training
// signal, not a bug per se.
func (s *Store) MissingSignalDistribution(ctx context.Context, orgID int64, since time.Time) ([]MissingSignalBucket, error) {
	rows, err := s.RecentPromptRouting(ctx, orgID, since, 500)
	if err != nil {
		return nil, err
	}
	counts := map[string]int{}
	for _, r := range rows {
		if !isAskBackAction(r.ActionTaken) {
			continue
		}
		for _, s := range r.MissingSignals {
			counts[s]++
		}
	}
	out := make([]MissingSignalBucket, 0, len(counts))
	for sig, n := range counts {
		out = append(out, MissingSignalBucket{Signal: sig, Count: n})
	}
	// Sort by count DESC.
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j].Count > out[i].Count {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out, nil
}

// ── helpers ──────────────────────────────────────────────────────────────────

func isAskBackAction(action string) bool {
	for _, a := range askBackActions {
		if a == action {
			return true
		}
	}
	return false
}

// decodeRouteReason extracts (route, reason_code) from a raw JSON
// decision blob, returning ("legacy", "unknown") on empty / unparseable
// rows so the dashboard always has something to render.
func decodeRouteReason(decision string) (string, string) {
	if strings.TrimSpace(decision) == "" || decision == "{}" {
		return "legacy", "unknown"
	}
	var d struct {
		Route      string `json:"route"`
		ReasonCode string `json:"reason_code"`
	}
	if err := json.Unmarshal([]byte(decision), &d); err != nil {
		return "legacy", "unknown"
	}
	if d.Route == "" {
		d.Route = "legacy"
	}
	if d.ReasonCode == "" {
		d.ReasonCode = "unknown"
	}
	return d.Route, d.ReasonCode
}

// fillDecisionFields parses the routing_decision_json into the
// PromptRoutingRow's flat fields. Tolerates malformed JSON.
func fillDecisionFields(r *PromptRoutingRow, decision string) {
	r.Route = "legacy"
	r.ReasonCode = "unknown"
	if strings.TrimSpace(decision) == "" || decision == "{}" {
		return
	}
	var d struct {
		Route            string   `json:"route"`
		SufficiencyScore float64  `json:"sufficiency_score"`
		MissingSignals   []string `json:"missing_signals"`
		InferredSignals  []string `json:"inferred_signals"`
		ReasonCode       string   `json:"reason_code"`
		Reason           string   `json:"reason"`
	}
	if err := json.Unmarshal([]byte(decision), &d); err != nil {
		return
	}
	if d.Route != "" {
		r.Route = d.Route
	}
	if d.ReasonCode != "" {
		r.ReasonCode = d.ReasonCode
	}
	r.Reason = d.Reason
	r.SufficiencyScore = d.SufficiencyScore
	r.MissingSignals = d.MissingSignals
	r.InferredSignals = d.InferredSignals
	// Also keep the raw payload so the dashboard can render any
	// future fields we didn't enumerate.
	var raw map[string]any
	if err := json.Unmarshal([]byte(decision), &raw); err == nil {
		r.DecisionRaw = raw
	}
}

func formatRelativeShort(d time.Duration) string {
	s := int(d.Seconds())
	if s < 60 {
		return fmt.Sprintf("%ds", s)
	}
	return fmt.Sprintf("%dm", s/60)
}

