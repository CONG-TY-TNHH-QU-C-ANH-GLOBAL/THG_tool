package hybrid

import (
	"context"
	"reflect"
	"testing"

	"github.com/thg/scraper/internal/workspace_knowledge/assets"
	"github.com/thg/scraper/internal/workspace_knowledge/retrieval"
)

func TestMultilingualRanksVietnameseWholeWords(t *testing.T) {
	lister := &fakeLister{all: []*assets.Asset{
		mkAsset(1, "Dịch vụ POD", "In áo", assets.AssetFAQ, nil, assets.StateApproved, false, 0, nil),
		mkAsset(2, "Dịch vụ tìm nguồn", "Taobao", assets.AssetFAQ, nil, assets.StateApproved, false, 0, nil),
		mkAsset(3, "Kho Mỹ và fulfillment nội địa", "Nhập kho, quản lý SKU và xử lý đơn", assets.AssetFAQ, nil, assets.StateApproved, false, 0, nil),
	}}
	var reference []retrieval.Hit
	for _, query := range []string{"dịch vụ kho Mỹ", "dich vu kho My", "di\u0323ch vu\u0323 kho My\u0303"} {
		hits, trace, err := NewMultilingual(lister).TopKWithTrace(context.Background(), 7, query, retrieval.SearchFilter{}, 3)
		if err != nil || len(hits) == 0 || hits[0].Asset.ID != 3 {
			t.Fatalf("query %q: warehouse should lead: %+v, %v", query, hits, err)
		}
		if trace.SearcherImpl != MultilingualSearcherImpl {
			t.Fatalf("missing v2 replay version: %q", trace.SearcherImpl)
		}
		if reference == nil {
			reference = hits
		} else if !reflect.DeepEqual(reference, hits) {
			t.Fatal("equivalent Vietnamese forms changed order or scores")
		}
	}
	_, oldTrace, _ := New(lister).TopKWithTrace(context.Background(), 7, "dịch vụ kho Mỹ", retrieval.SearchFilter{}, 3)
	if oldTrace.SearcherImpl != SearcherImpl {
		t.Fatal("legacy constructor must retain its replay version")
	}
}

func TestMultilingualDescriptionAndWordBoundaries(t *testing.T) {
	lister := &fakeLister{all: []*assets.Asset{
		mkAsset(1, "Chính sách", "kiểm tra đóng gói trước khi giao", assets.AssetFAQ, nil, assets.StateApproved, false, 0, nil),
		mkAsset(2, "Đóng gói", "", assets.AssetFAQ, nil, assets.StateApproved, false, 0, nil),
		mkAsset(3, "M40 phonecase", "", assets.AssetPODProduct, nil, assets.StateApproved, false, 0, nil),
	}}
	hits, _, err := NewMultilingual(lister).TopKWithTrace(context.Background(), 7, "dong goi", retrieval.SearchFilter{}, 3)
	if err != nil || len(hits) != 2 || hits[0].Asset.ID != 2 {
		t.Fatalf("title should beat prose but prose must remain searchable: %+v, %v", hits, err)
	}
	for _, query := range []string{"m4", "case"} {
		hits, _, _ := NewMultilingual(lister).TopKWithTrace(context.Background(), 7, query, retrieval.SearchFilter{}, 3)
		if len(hits) != 0 {
			t.Fatalf("substring is not a word/model match: %q -> %+v", query, hits)
		}
	}
}

func TestMultilingualGovernanceAndScoreComponents(t *testing.T) {
	lister := &fakeLister{all: []*assets.Asset{
		mkAsset(1, "Kho Mỹ", "", assets.AssetBannedClaim, nil, assets.StateApproved, true, 100, nil),
		mkAsset(2, "Kho Mỹ", "", assets.AssetFAQ, nil, assets.StateHidden, false, 0, nil),
		mkAsset(3, "Kho Mỹ", "", assets.AssetFAQ, nil, assets.StateApproved, true, 50, nil),
	}}
	hits, trace, err := NewMultilingual(lister).TopKWithTrace(context.Background(), 7, "kho My", retrieval.SearchFilter{}, 3)
	if err != nil || len(hits) != 1 || hits[0].Asset.ID != 3 || trace.TotalByReason[retrieval.RejectGovernance] != 1 {
		t.Fatalf("governance/state changed: %+v, %+v, %v", hits, trace, err)
	}
	bd := trace.Selected[0].Breakdown
	if bd.Pin != 0.2 || bd.Boost != 0.1 || abs(hits[0].Score-(bd.TextMatch+bd.Pin+bd.Boost+bd.Recency)) > 1e-12 {
		t.Fatalf("score contributions no longer match: %+v", bd)
	}
}

func TestMultilingualMaterialInTitleOutranksIncidentalProse(t *testing.T) {
	lister := &fakeLister{all: []*assets.Asset{
		mkAsset(1, "Polyester tote bag", "Alternative to a cotton tote bag", assets.AssetPODProduct, nil, assets.StateApproved, false, 0, nil),
		mkAsset(2, "Cotton tote bag", "Ready for your design", assets.AssetPODProduct, nil, assets.StateApproved, false, 0, nil),
	}}
	hits, _, err := NewMultilingual(lister).TopKWithTrace(context.Background(), 7, "cotton tote bag", retrieval.SearchFilter{}, 2)
	if err != nil || len(hits) != 2 || hits[0].Asset.ID != 2 {
		t.Fatalf("incidental description mention displaced matching product: %+v, %v", hits, err)
	}
}
