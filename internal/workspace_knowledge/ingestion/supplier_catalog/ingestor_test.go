package supplier_catalog

import (
	"testing"
	"time"

	"github.com/thg/scraper/internal/suppliersourcing"
)

func f(v float64) *float64 { return &v }

// A pilot run returned 0.001 kg for a fleece hoodie: the upstream weight is an
// AI estimate and is sometimes junk. An implausible value must read as unknown,
// never as a measurement a shipping quote could be built on.
func TestPayloadFrom_DropsImplausibleWeight(t *testing.T) {
	cases := []struct {
		name  string
		in    *float64
		wantN bool // want nil
	}{
		{"one gram hoodie", f(0.001), true},
		{"zero", f(0), true},
		{"absent", nil, true},
		{"at the floor", f(0.01), false},
		{"real garment", f(0.292), false},
	}
	for _, tc := range cases {
		got := payloadFrom(&suppliersourcing.Product{Platform: "alibaba", ID: "1", WeightKG: tc.in}, time.Now())
		if (got.WeightKG == nil) != tc.wantN {
			t.Errorf("%s: WeightKG nil = %v, want nil = %v", tc.name, got.WeightKG == nil, tc.wantN)
		}
	}
}

func TestPayloadFrom_CopiesTiersAndPrefersPromotionPrice(t *testing.T) {
	got := payloadFrom(&suppliersourcing.Product{
		Platform: "alibaba", ID: "1",
		PriceRange: []suppliersourcing.PriceTier{
			{MOQ: 2, Price: f(295)},
			{MOQ: 20, Price: f(285), PromotionPrice: f(270)},
			{MOQ: 100}, // no price at all — must be skipped, not zero-filled
		},
	}, time.Now())
	if len(got.PriceTiers) != 2 {
		t.Fatalf("tiers = %+v, want the two priced ones", got.PriceTiers)
	}
	if got.PriceTiers[1].Price != 270 {
		t.Errorf("tier price = %v, want the promotion price", got.PriceTiers[1].Price)
	}
}

func TestBudget_TruncatedOnlyWhenWorkWasCutShort(t *testing.T) {
	r := &syncRun{spend: &budget{remaining: 1}}
	if !r.spend.take() {
		t.Fatal("first take must succeed")
	}
	if r.truncated {
		t.Error("spending the last unit is not truncation")
	}
	if r.spend.take() {
		t.Error("budget must be empty now")
	}
}
