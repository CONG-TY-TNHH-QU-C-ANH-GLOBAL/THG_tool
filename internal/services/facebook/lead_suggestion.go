package facebook

import (
	"context"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/thg/scraper/internal/ai"
	"github.com/thg/scraper/internal/models"
	knowledgeRuntime "github.com/thg/scraper/internal/workspace_knowledge/runtime"
)

// Operator reply-suggestion helpers (Telegram lead notice). These are PURE:
// the composition root does the retrieval + generation IO and hands the results
// here; the sink only renders the assembled fields. No store, no network.

// LeadSuggestion is the operator-facing reply suggestion attached to a new-lead
// Telegram notice. Any field may be empty (suggestions are best-effort + opt-in).
type LeadSuggestion = models.LeadSuggestion

type ShippingQuoteFunc func(context.Context, models.ShippingRequest) (*models.ShippingReference, error)

// OrgAllowlist is a fail-closed rollout policy. The wildcard is accepted only
// when it is the complete value; any malformed token invalidates the full list.
type OrgAllowlist struct {
	all bool
	ids map[int64]struct{}
}

func ParseOrgAllowlist(raw string) OrgAllowlist {
	raw = strings.TrimSpace(raw)
	if raw == "*" {
		return OrgAllowlist{all: true}
	}
	if raw == "" {
		return OrgAllowlist{}
	}
	ids := make(map[int64]struct{})
	for _, token := range strings.Split(raw, ",") {
		id, err := strconv.ParseInt(strings.TrimSpace(token), 10, 64)
		if err != nil || id <= 0 {
			return OrgAllowlist{}
		}
		ids[id] = struct{}{}
	}
	return OrgAllowlist{ids: ids}
}

func (a OrgAllowlist) Allows(orgID int64) bool {
	if orgID <= 0 {
		return false
	}
	if a.all {
		return true
	}
	_, ok := a.ids[orgID]
	return ok
}

func (a OrgAllowlist) Configured() bool {
	return a.all || len(a.ids) > 0
}

type SuggestedProduct struct {
	Name, URL, ImageURL string
}

func validHTTPSURL(raw string) string {
	raw = strings.TrimSpace(raw)
	u, err := url.ParseRequestURI(raw)
	if err != nil || !u.IsAbs() || !strings.EqualFold(u.Scheme, "https") || u.Hostname() == "" || u.User != nil {
		return ""
	}
	return raw
}

// PickSuggestedProductDetails returns only persisted, safe-to-render catalog
// fields from the first ranked available product.
func PickSuggestedProductDetails(candidates []models.KnowledgeCandidate) SuggestedProduct {
	for _, c := range candidates {
		if c.Kind != "POD_product" {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(c.Availability)) {
		case "out_of_stock", "discontinued":
			continue
		}
		link := validHTTPSURL(c.SourceURL)
		if link == "" {
			continue
		}
		return SuggestedProduct{
			Name: strings.TrimSpace(c.Title), URL: link, ImageURL: validHTTPSURL(c.ImageURL),
		}
	}
	return SuggestedProduct{}
}

// PickSuggestedProduct returns the first grounded POD-product candidate that
// carries a real catalog PDP link, as (name, url). Retrieval already ranked the
// candidates, so the first product with a link is the best-matched one. Returns
// ("","") when no product candidate has a link — the notice then omits the
// product block. Never fabricates: only a candidate's own Title/SourceURL is used.
func PickSuggestedProduct(candidates []models.KnowledgeCandidate) (name, url string) {
	p := PickSuggestedProductDetails(candidates)
	return p.Name, p.URL
}

// A retrieval hit can be broadly relevant to the workspace without matching the
// item in this post. Require a product term from the post before offering it.
func matchesLeadProduct(leadText, title string) bool {
	words := func(s string) map[string]bool {
		out := map[string]bool{}
		for _, word := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) }) {
			if len([]rune(word)) >= 3 && !leadProductStopWords[word] {
				out[word] = true
			}
		}
		return out
	}
	post := words(leadText)
	matches := 0
	for word := range words(title) {
		if post[word] {
			matches++
			if matches >= 2 || leadProductSpecificTerms[word] {
				return true
			}
		}
	}
	return false
}

var leadProductSpecificTerms = map[string]bool{
	"hoodie": true, "stocking": true, "tshirt": true,
	"sweatshirt": true, "shirt": true, "thun": true,
}

var leadProductStopWords = map[string]bool{
	"sản": true, "phẩm": true, "hàng": true, "cần": true, "tìm": true,
	"giá": true, "cho": true, "với": true, "của": true, "bên": true,
	"mình": true, "shop": true, "bán": true, "đơn": true, "ship": true,
	"the": true, "and": true, "for": true, "product": true, "new": true,
}

func matchingCandidates(leadText string, candidates []models.KnowledgeCandidate) []models.KnowledgeCandidate {
	matched := make([]models.KnowledgeCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if matchesLeadProduct(leadText, candidate.Title) {
			matched = append(matched, candidate)
		}
	}
	return matched
}

// The post's buying intent decides which business path to offer. A matching
// catalog title alone does not make a bulk sourcing request a POD request.
func wantsBulkSourcing(text string) bool {
	text = strings.ToLower(text)
	if bulkQuantityPattern.MatchString(text) {
		return true
	}
	for _, phrase := range []string{"số lượng lớn", "sll", "nhập hàng", "lấy sỉ", "mua sỉ", "nguồn hàng", "giá sỉ", "1688", "taobao", "hộp/tháng", "cái/tháng", "sp/tháng"} {
		if strings.Contains(text, phrase) {
			return true
		}
	}
	return false
}

var bulkQuantityPattern = regexp.MustCompile(`\d{2,}\s*(hộp|cái|chiếc|sản phẩm|pcs|units|đôi)`)

func wantsPersonalizedPOD(text string) bool {
	text = strings.ToLower(text)
	for _, phrase := range []string{"in theo logo", "in logo", "in theo thiết kế", "in theo yêu cầu", "cá nhân hóa", "custom logo", "personalized", "print on demand", "pod"} {
		if strings.Contains(text, phrase) {
			return true
		}
	}
	return false
}

// BuildLeadSuggestion performs the optional, operator-facing suggestion work.
// It is intentionally best-effort: retrieval or generation failure may omit
// a draft and must never affect lead ingestion.
func BuildLeadSuggestion(ctx context.Context, builder *knowledgeRuntime.Builder, msgGen *ai.MessageGenerator, profile *ai.BusinessProfile, orgID int64, leadText, author string, shippingQuote ShippingQuoteFunc, supplierLookup SupplierLookupFunc) LeadSuggestion {
	var candidates []models.KnowledgeCandidate
	if builder != nil {
		var err error
		candidates, _, err = builder.CandidatesForLead(ctx, orgID, leadText)
		if err != nil {
			// Live sourcing can still resolve a product when the local knowledge
			// retrieval is temporarily unavailable.
			candidates = nil
		}
	}
	matched := matchingCandidates(leadText, candidates)
	product := PickSuggestedProductDetails(matched)
	var supplier *models.SupplierMatch
	if wantsBulkSourcing(leadText) && !wantsPersonalizedPOD(leadText) {
		product = SuggestedProduct{}
	}
	if product.URL == "" {
		supplier = PickSuggestedSupplier(matched)
		if linkedURL, _ := leadMarketplaceURL(leadText); linkedURL != "" && supplier != nil && supplier.URL != linkedURL {
			// An exact seller link in the post outranks a similarly titled item
			// from the approved index.
			supplier = nil
		}
		if !freshSupplierPrice(supplier, time.Now()) {
			supplier = nil
		}
		var liveWeight *float64
		if !supplier.HasOffer() && supplierLookup != nil {
			resolved, lookupErr := supplierLookup(ctx, leadText)
			if lookupErr == nil && resolved != nil {
				supplier, liveWeight = resolved.Match, resolved.WeightKG
			}
		}
		// CRM may return a one-parcel reference for a bulk post, labelled as
		// such. It never treats that figure as the shipment total.
		if supplier.HasOffer() && shippingQuote != nil && leadDestinationCountry(leadText) != "" {
			quantity := leadQuantity(leadText)
			mode := "parcel"
			if wantsBulkSourcing(leadText) {
				mode = "bulk"
			}
			cargo := "unknown"
			if isOrdinaryApparel(supplier.Name) {
				cargo = "standard"
			}
			quoteForWeight := func(weight float64) {
				if quantity > 0 && weight > 0 && weight <= 20 {
					supplier.Shipping, _ = shippingQuote(ctx, models.ShippingRequest{
						OriginCountry: "CN", DestinationCountry: leadDestinationCountry(leadText),
						Quantity: quantity, ShipmentMode: mode, CargoCategory: cargo, WeightKG: weight,
					})
				}
			}
			if liveWeight != nil && *liveWeight > 0 && *liveWeight <= 20 {
				quoteForWeight(*liveWeight)
			} else {
				for _, candidate := range matched {
					if candidate.SourceURL == supplier.URL && candidate.Supplier != nil && candidate.Supplier.WeightKG != nil {
						quoteForWeight(*candidate.Supplier.WeightKG)
						break
					}
				}
			}
		}
	}
	// The company's matching POD item takes priority. Marketplace sourcing is the
	// fallback, never a second unrelated offer in the same lead suggestion.
	if product.Name == "" && product.URL == "" && !supplier.HasOffer() {
		return LeadSuggestion{}
	}
	out := LeadSuggestion{
		ProductName: product.Name, ProductURL: product.URL, ProductImageURL: product.ImageURL,
		Supplier: supplier,
	}
	if supplier.HasOffer() {
		// Sourcing facts already provide a concise, copy-ready operator draft.
		// Assemble it deterministically so an LLM cannot change the price or
		// turn a one-parcel reference into a quote for the whole shipment.
		out.Reply = supplierFallbackReply(author, supplier, leadText) + " " + supplier.URL
		return out
	}
	if msgGen == nil || !msgGen.Available() || profile == nil {
		out.Reply = leadSalutation(author) + ", bên mình có " + shortLeadTitle(product.Name) + " phù hợp nhu cầu của bạn. Mình gửi chi tiết qua inbox nhé? " + product.URL
		return out
	}
	reply, err := msgGen.GenerateLeadReplySuggestion(ctx, ai.LeadReplyRequest{
		PostContent:     leadText,
		AuthorName:      author,
		BusinessContext: profile.ToPromptBlock(),
		BrandName:       profile.Name,
		GroundedFacts:   BuildGroundedFacts(product, supplier),
	})
	if err != nil {
		return out
	}
	out.Reply = strings.TrimSpace(reply)
	selectedURL := product.URL
	if product.URL != "" && len([]rune(out.Reply)) > 320 {
		out.Reply = leadSalutation(author) + ", bên mình có " + shortLeadTitle(product.Name) + " phù hợp nhu cầu của bạn. Mình gửi thêm chi tiết qua inbox nhé?"
	}
	if selectedURL != "" && !strings.Contains(out.Reply, selectedURL) {
		out.Reply = strings.TrimSpace(out.Reply + " " + selectedURL)
	}
	return out
}

func freshSupplierPrice(supplier *models.SupplierMatch, now time.Time) bool {
	if !supplier.HasOffer() || supplier.PriceText == "" {
		return false
	}
	captured, err := time.Parse(time.RFC3339, supplier.CapturedAt)
	if err != nil {
		return false
	}
	age := now.Sub(captured)
	return age >= 0 && age <= 7*24*time.Hour
}

func supplierFallbackReply(author string, supplier *models.SupplierMatch, leadText string) string {
	first := leadSalutation(author) + ", bên mình có thể tìm nguồn " + shortLeadTitle(supplier.Name)
	if supplier.PriceText != "" {
		first += ", giá nguồn tham khảo " + supplier.PriceText
	}
	if supplier.Shipping != nil && supplier.Shipping.PriceText != "" {
		first += "; cước tham chiếu " + supplier.Shipping.PriceText + "/kiện"
		if strings.Contains(supplier.Shipping.Basis, "không phải tổng cước lô") {
			first += " (chưa phải tổng cước lô)"
		}
		if supplier.Shipping.Transit != "" {
			first += " (" + supplier.Shipping.Transit + ")"
		}
	} else if destination := leadDestinationCountry(leadText); destination != "" {
		first += "; tuyến CN→" + destination + ", cước cần xác nhận theo cách giao và quy cách kiện"
	}
	return first + ". Mình trao đổi số lượng và báo giá cụ thể qua inbox nhé?"
}

func leadSalutation(author string) string {
	if name := strings.TrimSpace(author); name != "" {
		return name
	}
	return "Bạn"
}

func shortLeadTitle(title string) string {
	runes := []rune(strings.TrimSpace(title))
	if len(runes) > 100 {
		return string(runes[:100]) + "…"
	}
	return string(runes)
}

func isUSDestination(text string) bool {
	text = strings.ToLower(text)
	for _, term := range []string{"sang mỹ", "về mỹ", "đi mỹ", "tại mỹ", "qua mỹ", "to us", "to usa", " united states", " hoa kỳ", " u.s."} {
		if strings.Contains(" "+text, term) {
			return true
		}
	}
	return false
}

var leadQuantityPattern = regexp.MustCompile(`(\d{1,6})\s*(hộp|cái|chiếc|sản phẩm|pcs|units|đôi|boxes|pieces)`)

func leadQuantity(text string) int {
	if match := leadQuantityPattern.FindStringSubmatch(strings.ToLower(text)); len(match) > 1 {
		quantity, err := strconv.Atoi(match[1])
		if err == nil && quantity > 0 {
			return quantity
		}
	}
	for _, phrase := range []string{"một hộp", "một cái", "một chiếc", "one box", "one piece"} {
		if strings.Contains(strings.ToLower(text), phrase) {
			return 1
		}
	}
	return 0
}

func leadDestinationCountry(text string) string {
	if isUSDestination(text) {
		return "US"
	}
	text = strings.ToLower(text)
	for _, term := range []string{"sang anh", "về anh", "đi anh", "to uk", "to gb", " united kingdom"} {
		if strings.Contains(" "+text, term) {
			return "GB"
		}
	}
	return ""
}

func isOrdinaryApparel(name string) bool {
	name = strings.ToLower(name)
	for _, blocked := range []string{"thuốc", "thực phẩm", "bổ sung", "pin", "mỹ phẩm", "chất lỏng", "supplement", "battery", "cosmetic"} {
		if strings.Contains(name, blocked) {
			return false
		}
	}
	for _, term := range []string{"áo", "hoodie", "shirt", "stocking", "tất", "vớ", "quần", "socks", "dress"} {
		if strings.Contains(name, term) {
			return true
		}
	}
	return false
}
