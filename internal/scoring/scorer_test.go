package scoring

import "testing"

func TestScoreRejectsProviderPromotion(t *testing.T) {
	s := New(DefaultConfig())
	result := s.Score(
		"Bên em chuyên cung cấp dịch vụ vận chuyển quốc tế, nhận ship hàng Mỹ, inbox em để được báo giá ưu đãi.",
		[]string{"logistics", "vận chuyển", "ship hàng"},
		8,
		2,
		"https://facebook.com/profile",
	)
	if result.Category != "rejected" {
		t.Fatalf("expected provider ad to be rejected, got %q score %.1f signals=%v", result.Category, result.Score, result.Signals)
	}
}

func TestScorePromotesBuyerDemand(t *testing.T) {
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

func TestScoreCapsKeywordOnlyPosts(t *testing.T) {
	s := New(DefaultConfig())
	result := s.Score(
		"Logistics ecommerce và fulfillment xuyên biên giới đang thay đổi rất nhanh trong năm nay.",
		[]string{"logistics", "ecommerce", "fulfillment"},
		90,
		40,
		"https://facebook.com/profile",
	)
	if result.Category != "cold" {
		t.Fatalf("expected keyword-only post to stay cold, got %q score %.1f signals=%v", result.Category, result.Score, result.Signals)
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
		t.Fatalf("expected supplier/provider post to pass when org targets suppliers, got %q score %.1f signals=%v", result.Category, result.Score, result.Signals)
	}
}
