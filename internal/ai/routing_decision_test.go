package ai

import (
	"encoding/json"
	"strings"
	"testing"
)

// RoutingDecision is the load-bearing substrate for the prompt-routing
// dashboard. ToJSON must always emit a valid JSON object — the column
// constraint is "always parseable", so dashboards json.Unmarshal without
// per-row error handling.

func TestRoutingDecision_ToJSON_Stable(t *testing.T) {
	t.Parallel()
	d := RoutingDecision{
		Route:            RouteDeterministic,
		SufficiencyScore: 1.0,
		InferredSignals:  []string{"pod", "dropship"},
		ReasonCode:       ReasonSelfSufficient,
		Reason:           "test",
	}
	raw := d.ToJSON()
	var back map[string]any
	if err := json.Unmarshal([]byte(raw), &back); err != nil {
		t.Fatalf("ToJSON produced invalid JSON: %v\n%s", err, raw)
	}
	if back["route"] != "deterministic" {
		t.Errorf("route lost in round-trip: %v", back["route"])
	}
	if back["reason_code"] != "self_sufficient_prompt" {
		t.Errorf("reason_code lost: %v", back["reason_code"])
	}
}

// Zero-value decision (e.g. legacy code path that forgot to construct
// one) serialises to "{}" rather than null. Dashboards render this as
// "legacy/unknown" — the safety contract for back-compat rows.
func TestRoutingDecision_ZeroValue_EmptyObject(t *testing.T) {
	t.Parallel()
	var d RoutingDecision
	if got := d.ToJSON(); got != "{}" {
		t.Errorf("zero-value ToJSON = %q; want %q", got, "{}")
	}
}

// analyseMissingSignals is the function powering the "Ambiguous Prompt
// Surface" panel. Pin its outputs so a regression there doesn't
// silently break the dashboard's most actionable feature.
func TestAnalyseMissingSignals(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		prompt string
		want   []string // signals MUST appear (subset match)
	}{
		{
			name:   "no URL → source missing",
			prompt: "find me 50 leads needing POD",
			want:   []string{SignalSource},
		},
		{
			name:   "URL + crawl verb + quantity → only target/market may be missing",
			prompt: "crawl 50 posts in https://facebook.com/groups/12345",
			want:   []string{SignalTarget, SignalMarket},
		},
		{
			name:   "no verb → action missing",
			prompt: "https://facebook.com/groups/12345",
			want:   []string{SignalAction, SignalQuantity, SignalTarget, SignalMarket},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := analyseMissingSignals(c.prompt)
			set := map[string]bool{}
			for _, s := range got {
				set[s] = true
			}
			for _, w := range c.want {
				if !set[w] {
					t.Errorf("expected MissingSignals to contain %q; got %v", w, got)
				}
			}
		})
	}
}

// NewDeterministicDecision must include the inferred signals so the
// dashboard can show "system understood these signals before dispatching".
func TestNewDeterministicDecision_IncludesInferredSignals(t *testing.T) {
	t.Parallel()
	d := NewDeterministicDecision("Cào 50 bài tìm seller cần fulfill POD dropship", "scrape_group", ReasonSelfSufficient)
	if d.Route != RouteDeterministic {
		t.Fatalf("route = %q; want deterministic", d.Route)
	}
	if d.ReasonCode != ReasonSelfSufficient {
		t.Fatalf("reason_code = %q", d.ReasonCode)
	}
	if d.SufficiencyScore != 1.0 {
		t.Errorf("sufficiency_score = %v; want 1.0", d.SufficiencyScore)
	}
	if len(d.InferredSignals) == 0 {
		t.Errorf("expected inferred signals from POD/dropship/fulfill prompt; got empty")
	}
	joined := strings.Join(d.InferredSignals, ",")
	if !strings.Contains(joined, "pod") {
		t.Errorf("expected 'pod' in inferred signals; got %v", d.InferredSignals)
	}
}

// NewBrainDecision must map the brain's raw decision string to a stable
// ReasonCode so the dashboard can aggregate "ask_user rate" without
// brittle string matching.
func TestNewBrainDecision_ReasonCodeMapping(t *testing.T) {
	t.Parallel()
	cases := []struct {
		brain string
		want  string
	}{
		{"ask_user", ReasonBrainAskUser},
		{"execute", ReasonBrainExecute},
		{"chat", ReasonBrainChat},
		{"refuse", ReasonBrainRefuse},
		{"unknown_value", ReasonBrainExecute}, // safe default — won't trigger ask-back rate
	}
	for _, c := range cases {
		got := NewBrainDecision(c.brain, "test").ReasonCode
		if got != c.want {
			t.Errorf("brain decision %q → %q; want %q", c.brain, got, c.want)
		}
	}
}

// PromptIsSelfSufficient (the exported wrapper) must match the
// package-private gate. Pinning this prevents accidental divergence
// between the orchestrator's gate and the observability handler's
// false-negative detection.
func TestPromptIsSelfSufficient_Exported_Matches_Private(t *testing.T) {
	t.Parallel()
	cases := []string{
		"Cào 50 bài tìm seller cần fulfill POD dropship trong https://facebook.com/groups/12345",
		"find clients", // ambiguous
		"crawl https://facebook.com/groups/12345 to find POD buyers",
	}
	for _, p := range cases {
		if PromptIsSelfSufficient(p) != promptIsSelfSufficient(p) {
			t.Errorf("Exported vs private divergence on %q", p)
		}
	}
}
