package scoring

import "testing"

func TestScoreRejectsOnlyOnGuidanceRejectPhrase(t *testing.T) {
	s := New(DefaultConfig())
	content := "Bên em chuyên cung cấp dịch vụ vận chuyển quốc tế, nhận ship hàng Mỹ, inbox em để được báo giá ưu đãi."

	// Without org guidance, the scorer is generic — provider-style copy must
	// not be hard-rejected. Baking that judgement into the core scorer would
	// hardcode one industry's reading (CLAUDE.md hard rule).
	plain := s.Score(content, []string{"vận chuyển", "ship"}, 8, 2, "https://facebook.com/profile")
	if plain.Category == "rejected" {
		t.Fatalf("scorer must not reject provider-style content without guidance, got %q score=%.1f", plain.Category, plain.Score)
	}

	// With an org-driven reject phrase that matches, the scorer rejects.
	gated := s.ScoreWithGuidance(
		content,
		[]string{"vận chuyển", "ship"},
		8, 2, "https://facebook.com/profile",
		Guidance{RejectPhrases: []string{"nhận ship"}},
	)
	if gated.Category != "rejected" {
		t.Fatalf("scorer must reject when content matches Guidance.RejectPhrases, got %q score=%.1f", gated.Category, gated.Score)
	}
}

func TestScorePromotesBuyerDemand(t *testing.T) {
	// Strong buyer-demand content + matching keywords + decent engagement —
	// the generic scorer (no guidance) should land it in hot/warm.
	s := New(DefaultConfig())
	result := s.Score(
		"Mình cần tìm bên fulfillment có thể tìm hàng từ Việt Nam hoặc Trung Quốc rồi ship đi Mỹ. Ai có kinh nghiệm cho mình xin contact với.",
		[]string{"fulfillment", "ship", "trung quốc", "việt nam"},
		4,
		5,
		"https://facebook.com/profile",
	)
	if result.Category != "hot" && result.Category != "warm" {
		t.Fatalf("expected buyer demand to become a lead, got %q score %.1f signals=%v", result.Category, result.Score, result.Signals)
	}
}

func TestScoreTargetSignalBoostsScore(t *testing.T) {
	s := New(DefaultConfig())
	content := "Bài viết về sourcing áo thun, ai có xưởng may POD ngon thì share nhé."
	keywords := []string{"pod", "sourcing"}

	noGuidance := s.Score(content, keywords, 2, 1, "https://facebook.com/profile")
	withGuidance := s.ScoreWithGuidance(content, keywords, 2, 1, "https://facebook.com/profile",
		Guidance{TargetSignals: []string{"xưởng may", "sourcing áo"}},
	)
	if withGuidance.Score <= noGuidance.Score {
		t.Fatalf("expected target_signal hit to lift score, got %.1f vs %.1f", withGuidance.Score, noGuidance.Score)
	}
}

func TestScoreAllowsProviderWhenOrgTargetsSuppliers(t *testing.T) {
	s := New(DefaultConfig())
	result := s.ScoreWithGuidance(
		"Bên em là xưởng may POD, nhận sourcing áo thun và ship hàng đi Mỹ cho seller.",
		[]string{"pod", "sourcing", "ship hàng"},
		6,
		3,
		"https://facebook.com/profile",
		Guidance{TargetAuthorRole: "suppliers", TargetSignals: []string{"xưởng", "sourcing"}},
	)
	if result.Category != "hot" && result.Category != "warm" {
		t.Fatalf("expected supplier post to qualify when guidance targets suppliers, got %q score %.1f signals=%v", result.Category, result.Score, result.Signals)
	}
}
