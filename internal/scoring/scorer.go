package scoring

import (
	"fmt"
	"strings"
	"unicode"
)

// Config controls scoring thresholds and dimension weights.
// All weights should sum to 1.0.
type Config struct {
	HotThreshold  float64 // score >= this → "hot"   (default 70)
	WarmThreshold float64 // score >= this → "warm"  (default 40)
	Weights       Weights
}

type Weights struct {
	KeywordRelevance float64 // default 0.40
	Engagement       float64 // default 0.30
	ContentQuality   float64 // default 0.30
}

func DefaultConfig() Config {
	return Config{
		HotThreshold:  70,
		WarmThreshold: 40,
		Weights: Weights{
			KeywordRelevance: 0.40,
			Engagement:       0.30,
			ContentQuality:   0.30,
		},
	}
}

// Result is the output of a single Score call.
type Result struct {
	Score    float64  // 0–100
	Category string   // "hot" | "warm" | "cold" | "rejected"
	Signals  []string // human-readable signals that fired
}

type Guidance struct {
	TargetAuthorRole string
	TargetSignals    []string
	RejectPhrases    []string
}

// Scorer computes lead scores inline. Stateless and concurrency-safe.
type Scorer struct{ cfg Config }

func New(cfg Config) *Scorer { return &Scorer{cfg: cfg} }

// Score computes a 0–100 lead score from content signals.
// Called inside the crawl loop — must be O(n) where n = len(content).
func (s *Scorer) Score(content string, keywords []string, reactions, comments int, authorURL string) Result {
	return s.ScoreWithGuidance(content, keywords, reactions, comments, authorURL, Guidance{})
}

func (s *Scorer) ScoreWithGuidance(content string, keywords []string, reactions, comments int, authorURL string, guidance Guidance) Result {
	market := classifyMarketSignal(content)
	signals := append([]string{}, market.signals...)
	if containsAny(strings.ToLower(content), normalizePhrases(guidance.RejectPhrases)) {
		return Result{Score: 0, Category: "rejected", Signals: append(signals, "reject:org_negative_signal")}
	}
	if market.spam {
		return Result{Score: 0, Category: "rejected", Signals: append(signals, "reject:spam_or_low_trust")}
	}
	targetRole := strings.ToLower(strings.TrimSpace(guidance.TargetAuthorRole))
	providerTarget := targetRole == "supplier" || targetRole == "suppliers" || targetRole == "partner" || targetRole == "partners" || targetRole == "provider" || targetRole == "providers" || targetRole == "reseller" || targetRole == "resellers"
	if market.providerAd && !market.buyerDemand && !providerTarget {
		return Result{Score: 0, Category: "rejected", Signals: append(signals, "reject:provider_promotion_without_buyer_demand")}
	}

	kwScore := keywordRelevance(content, keywords)
	if kwScore >= 0.6 {
		signals = append(signals, fmt.Sprintf("keyword_hit:%.0f%%", kwScore*100))
	}

	engScore := engagementScore(reactions, comments)
	if engScore >= 0.5 {
		signals = append(signals, fmt.Sprintf("engagement:%d", reactions+comments))
	}

	qualScore := contentQuality(content, authorURL)
	if qualScore >= 0.5 {
		signals = append(signals, "content_quality")
	}

	score := (kwScore*s.cfg.Weights.KeywordRelevance +
		engScore*s.cfg.Weights.Engagement +
		qualScore*s.cfg.Weights.ContentQuality) * 100

	if market.buyerDemand {
		score += 35
		signals = append(signals, "intent:buyer_demand")
	}
	if providerTarget && market.providerAd {
		score += 35
		signals = append(signals, "intent:target_provider_role")
	}
	targetSignalHit := containsAny(strings.ToLower(content), normalizePhrases(guidance.TargetSignals))
	if targetSignalHit {
		score += 15
		signals = append(signals, "intent:org_target_signal")
	}
	if market.question {
		score += 8
		signals = append(signals, "intent:question_or_request")
	}
	if market.providerAd && !providerTarget {
		score -= 25
		signals = append(signals, "penalty:provider_promotion")
	}
	if !market.buyerDemand && !market.question && !(providerTarget && (market.providerAd || targetSignalHit)) && score > 35 {
		score = 35
		signals = append(signals, "cap:no_explicit_buyer_demand")
	}
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}

	category := "cold"
	switch {
	case score >= s.cfg.HotThreshold:
		category = "hot"
		signals = append(signals, "hot_lead")
	case score >= s.cfg.WarmThreshold:
		category = "warm"
	}

	return Result{Score: score, Category: category, Signals: signals}
}

func normalizePhrases(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

type marketSignal struct {
	buyerDemand bool
	providerAd  bool
	question    bool
	spam        bool
	signals     []string
}

// classifyMarketSignal is intentionally industry-agnostic. It looks at the
// author's market role in the post: are they asking for help/buying, or are
// they advertising/selling? This prevents broad industries such as logistics,
// ecommerce, HR, or sourcing from turning every keyword match into a lead.
func classifyMarketSignal(content string) marketSignal {
	lower := strings.ToLower(content)
	out := marketSignal{}
	if containsAny(lower, buyerDemandPhrases()) {
		out.buyerDemand = true
		out.signals = append(out.signals, "role:buyer_demand")
	}
	if containsAny(lower, questionPhrases()) || strings.Contains(lower, "?") {
		out.question = true
		out.signals = append(out.signals, "role:asking_question")
	}
	if containsAny(lower, providerPromotionPhrases()) {
		out.providerAd = true
		out.signals = append(out.signals, "role:provider_promotion")
	}
	if containsAny(lower, spamPhrases()) || countURLs(lower) >= 3 {
		out.spam = true
		out.signals = append(out.signals, "quality:spam_signal")
	}
	return out
}

func containsAny(content string, phrases []string) bool {
	for _, phrase := range phrases {
		if phrase != "" && strings.Contains(content, phrase) {
			return true
		}
	}
	return false
}

func countURLs(content string) int {
	return strings.Count(content, "http://") + strings.Count(content, "https://") + strings.Count(content, "www.")
}

func buyerDemandPhrases() []string {
	return []string{
		"looking for", "looking to", "need a", "need an", "need someone", "i need", "we need",
		"in search of", "any recommendation", "recommend a", "can anyone", "does anyone know",
		"where can i", "quote for", "looking for supplier", "need supplier",
		"cần tìm", "can tim", "đang tìm", "dang tim", "mình cần", "minh can", "tôi cần", "toi can",
		"bên nào", "ben nao", "cần bên", "can ben", "cần đơn vị", "can don vi",
		"cần nhà cung cấp", "can nha cung cap", "cần supplier", "can supplier",
		"cần báo giá", "can bao gia", "xin báo giá", "xin bao gia", "cho mình xin giá", "cho minh xin gia",
		"cần tư vấn", "can tu van", "ai biết", "ai biet", "có ai", "co ai",
		"tìm bên", "tim ben", "tìm đơn vị", "tim don vi", "muốn thuê", "muon thue",
		"muốn mua", "muon mua", "muốn tìm", "muon tim", "cần mua", "can mua", "cần thuê", "can thue",
	}
}

func questionPhrases() []string {
	return []string{
		"anyone know", "anyone recommend", "who can", "how can i", "where to",
		"ai có", "ai co", "ai làm", "ai lam", "bạn nào", "ban nao", "anh chị nào", "anh chi nao",
		"xin contact", "xin liên hệ", "xin lien he", "cho xin", "nhờ tư vấn", "nho tu van",
	}
}

func providerPromotionPhrases() []string {
	return []string{
		"we provide", "we offer", "our service", "contact us", "dm me", "inbox me",
		"chúng tôi cung cấp", "chung toi cung cap", "bên em cung cấp", "ben em cung cap",
		"bên mình cung cấp", "ben minh cung cap", "dịch vụ của", "dich vu cua",
		"nhận làm", "nhan lam", "nhận order", "nhan order", "nhận ship", "nhan ship",
		"em chuyên", "em chuyen", "mình chuyên", "minh chuyen", "công ty chúng tôi", "cong ty chung toi",
		"hotline", "liên hệ em", "lien he em", "liên hệ ngay", "lien he ngay",
		"ib em", "inbox em", "zalo", "ưu đãi", "uu dai", "khuyến mãi", "khuyen mai",
	}
}

func spamPhrases() []string {
	return []string{
		"kiếm tiền online", "kiem tien online", "thu nhập thụ động", "thu nhap thu dong",
		"cam kết thu nhập", "cam ket thu nhap", "tuyển ctv", "tuyen ctv",
		"forex", "casino", "crypto", "betting", "vay tiền", "vay tien",
	}
}

// ── dimension functions ───────────────────────────────────────────────────────

func keywordRelevance(content string, keywords []string) float64 {
	if len(keywords) == 0 {
		return 0.5
	}
	lower := strings.ToLower(content)
	matched := 0
	for _, kw := range keywords {
		if strings.Contains(lower, strings.ToLower(kw)) {
			matched++
		}
	}
	return float64(matched) / float64(len(keywords))
}

func engagementScore(reactions, comments int) float64 {
	total := reactions + comments*2
	if total >= 50 {
		return 1.0
	}
	return float64(total) / 50.0
}

func contentQuality(content, authorURL string) float64 {
	var score float64
	runes := []rune(content)

	if len(runes) >= 50 {
		score += 0.3
	}
	if len(runes) >= 200 {
		score += 0.2
	}
	if authorURL != "" {
		score += 0.3
	}

	// penalise ALL-CAPS spam
	var letters, upper int
	for _, r := range runes {
		if unicode.IsLetter(r) {
			letters++
			if unicode.IsUpper(r) {
				upper++
			}
		}
	}
	if letters == 0 || float64(upper)/float64(letters) < 0.5 {
		score += 0.2
	}

	return score
}
