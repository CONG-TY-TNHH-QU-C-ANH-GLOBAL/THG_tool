package suppliersourcing

import "encoding/json"

// PriceTier is one MOQ break on a 1688 wholesale offer.
type PriceTier struct {
	MOQ            int      `json:"moq"`
	Price          *float64 `json:"price"`
	PromotionPrice *float64 `json:"promotion_price"`
}

// Product is the normalized detail record. Every field is optional: the
// upstream fills what the marketplace exposed and nothing is inferred here.
type Product struct {
	Platform   string      `json:"platform"`
	ID         string      `json:"id"`
	Title      string      `json:"title"`
	TitleCN    string      `json:"titleCn"`
	ShopName   string      `json:"shopName"`
	Category   string      `json:"category"`
	QuoteType  string      `json:"quoteType"`
	MOQ        *int        `json:"moq"`
	Unit       string      `json:"unit"`
	Sold       *int        `json:"sold"`
	Price      *float64    `json:"price"`
	PriceNote  string      `json:"priceNote"`
	PriceRange []PriceTier `json:"priceRange"`
	Images     []string    `json:"images"`
	Link       string      `json:"link"`
	ShipFrom   string      `json:"shipFrom"`
	WeightKG   *float64    `json:"weight"`
	Length     *float64    `json:"length"`
	Width      *float64    `json:"width"`
	Height     *float64    `json:"height"`
	Cached     bool        `json:"cached"`
	FetchedAt  string      `json:"fetchedAt"`
	CachedAt   string      `json:"cachedAt"`
}

// SearchItem is one hit from a keyword search. Search results carry no weight
// or MOQ — those only exist on the detail record.
type SearchItem struct {
	ID     string   `json:"id"`
	Title  string   `json:"title"`
	Link   string   `json:"link"`
	Image  string   `json:"image"`
	Seller string   `json:"seller"`
	Price  *float64 `json:"price"`
	Sales  *int     `json:"sales"`
	Unit   string   `json:"unit"`
}

// Quota reports upstream plan status and local usage counters.
type Quota struct {
	Plan  json.RawMessage `json:"plan"`
	Local struct {
		Month        int `json:"month"`
		Today        int `json:"today"`
		SavedByCache int `json:"savedByCache"`
	} `json:"local"`
}
