// Package supplier_catalog pre-indexes sourceable 1688 / Taobao items into the
// workspace knowledge base through the THG Pricing Hub.
//
// Why pre-index instead of looking a product up per lead: the upstream
// marketplace API is metered per month and shared with the human quoting tool.
// Indexing once turns per-lead product matching into a local retrieval that
// costs nothing and cannot exhaust anyone's budget. Freshness is traded away
// deliberately — a sync run re-reads prices, and the operator-facing message
// says when the price was captured.
package supplier_catalog

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/thg/scraper/internal/suppliersourcing"
	"github.com/thg/scraper/internal/workspace_knowledge/ingestion"
	"github.com/thg/scraper/internal/workspace_knowledge/sources"
	"github.com/thg/scraper/internal/workspace_knowledge/suppliers"
)

// Ingestor implements ingestion.Ingestor for sources.SourceSupplierCatalog.
type Ingestor struct {
	// Now is injectable so tests can pin the capture timestamp.
	Now func() time.Time
}

func New() *Ingestor { return &Ingestor{} }

func (i *Ingestor) Type() sources.SourceType { return sources.SourceSupplierCatalog }

func (i *Ingestor) now() time.Time {
	if i != nil && i.Now != nil {
		return i.Now()
	}
	return time.Now().UTC()
}

// budget tracks the hard per-run ceiling on upstream calls.
type budget struct {
	remaining int
}

func (b *budget) take() bool {
	if b.remaining <= 0 {
		return false
	}
	b.remaining--
	return true
}

func (i *Ingestor) Sync(ctx context.Context, src *sources.Source, w ingestion.AssetWriter) (ingestion.SyncResult, error) {
	if src == nil || w == nil {
		return ingestion.SyncResult{}, ingestion.WrapPermanent(errors.New("supplier_catalog: source and writer are required"))
	}
	cfg, err := parseConfig(src.ConnectionConfig)
	if err != nil {
		return ingestion.SyncResult{}, ingestion.WrapPermanent(err)
	}
	secret, err := loadSecret(cfg)
	if err != nil {
		return ingestion.SyncResult{}, ingestion.WrapPermanent(err)
	}
	client := suppliersourcing.NewClient(cfg.BaseURL, secret, time.Duration(cfg.TimeoutSeconds)*time.Second)
	if client == nil {
		return ingestion.SyncResult{}, ingestion.WrapPermanent(errors.New("supplier_catalog: pricing hub client is not configured"))
	}

	run := &syncRun{ingestor: i, cfg: cfg, src: src, writer: w, client: client, spend: &budget{remaining: cfg.MaxAPICalls}}
	run.indexLinks(ctx)
	run.indexQueries(ctx)
	if run.spend.remaining == 0 {
		run.result.Errors = append(run.result.Errors, ingestion.SyncError{
			Reason: "api_budget_exhausted",
			Detail: fmt.Sprintf("stopped after %d upstream calls; raise max_api_calls or split the run", cfg.MaxAPICalls),
		})
	}
	return run.result, nil
}

// syncRun holds the mutable state of one Sync call so the steps below stay
// small and independently readable.
type syncRun struct {
	ingestor *Ingestor
	cfg      Config
	src      *sources.Source
	writer   ingestion.AssetWriter
	client   *suppliersourcing.Client
	spend    *budget
	result   ingestion.SyncResult
}

// indexLinks resolves the product URLs the operator picked by hand.
func (r *syncRun) indexLinks(ctx context.Context) {
	for _, link := range r.cfg.Links {
		platform := platformFromLink(link)
		if platform == "" {
			r.result.AssetsRejected++
			r.result.Errors = append(r.result.Errors, ingestion.SyncError{
				ExternalID: link, Reason: "unsupported_marketplace",
			})
			continue
		}
		if !r.spend.take() {
			return
		}
		product, err := r.client.Detail(ctx, platform, "", link)
		if err != nil {
			r.recordFailure(link, err)
			continue
		}
		r.store(ctx, product)
	}
}

// indexQueries discovers items by keyword, then promotes the top hits to a
// detail lookup — search results carry no weight or MOQ.
func (r *syncRun) indexQueries(ctx context.Context) {
	for _, query := range r.cfg.Queries {
		if !r.spend.take() {
			return
		}
		items, err := r.client.Search(ctx, query.Q, query.Platform, query.Size)
		if err != nil {
			r.recordFailure(query.Q, err)
			continue
		}
		if exhausted := r.promoteHits(ctx, query, items); exhausted {
			return
		}
	}
}

// promoteHits turns the top search hits into indexed products. It reports
// whether the run's upstream budget ran out mid-way, which ends the whole sync
// rather than silently under-indexing the remaining queries.
func (r *syncRun) promoteHits(ctx context.Context, query Query, items []suppliersourcing.SearchItem) bool {
	promoted := 0
	for _, item := range items {
		if promoted >= r.cfg.DetailPerQuery {
			return false
		}
		if strings.TrimSpace(item.ID) == "" && strings.TrimSpace(item.Link) == "" {
			continue
		}
		if !r.spend.take() {
			return true
		}
		product, err := r.client.Detail(ctx, query.Platform, item.ID, item.Link)
		if err != nil {
			r.recordFailure(item.ID, err)
			continue
		}
		r.store(ctx, product)
		promoted++
	}
	return false
}

// store maps one upstream product onto an asset and writes it.
func (r *syncRun) store(ctx context.Context, product *suppliersourcing.Product) {
	if product == nil {
		return
	}
	r.result.AssetsSeen++
	payload := payloadFrom(product, r.ingestor.now())
	asset, ok := payload.Asset(r.src.OrgID, r.src.ID, r.cfg.Tags)
	if !ok {
		r.result.AssetsRejected++
		r.result.Errors = append(r.result.Errors, ingestion.SyncError{
			ExternalID: payload.ExternalID(), Reason: "incomplete_product",
		})
		return
	}
	if err := r.writer.Write(ctx, asset); err != nil {
		r.result.AssetsRejected++
		r.result.Errors = append(r.result.Errors, ingestion.SyncError{
			ExternalID: asset.ExternalID, Reason: "write_failed", Detail: err.Error(),
		})
		return
	}
	r.result.AssetsUpdated++
}

func (r *syncRun) recordFailure(ref string, err error) {
	r.result.AssetsRejected++
	r.result.Errors = append(r.result.Errors, ingestion.SyncError{
		ExternalID: ref, Reason: "upstream_failed", Detail: err.Error(),
	})
}

// payloadFrom copies the upstream record into the persisted schema. It only
// copies: nothing here estimates a weight, converts a currency, or invents a
// tier the marketplace did not publish.
func payloadFrom(product *suppliersourcing.Product, fetchedAt time.Time) suppliers.PayloadV1 {
	payload := suppliers.PayloadV1{
		Platform: product.Platform, ProductID: product.ID,
		Title: product.Title, TitleCN: product.TitleCN,
		ShopName: product.ShopName, Category: product.Category,
		PriceCNY: product.Price, PriceNote: product.PriceNote,
		MOQ: product.MOQ, Unit: product.Unit,
		WeightKG: product.WeightKG, LengthCM: product.Length,
		WidthCM: product.Width, HeightCM: product.Height,
		ShipFrom: product.ShipFrom, SoldCount: product.Sold,
		Images: product.Images, SourceURL: product.Link,
		SourceFetchedAt: fetchedAt.UTC(),
	}
	for _, tier := range product.PriceRange {
		price := tier.PromotionPrice
		if price == nil {
			price = tier.Price
		}
		if price == nil {
			continue
		}
		payload.PriceTiers = append(payload.PriceTiers, suppliers.PriceTier{MOQ: tier.MOQ, Price: *price})
	}
	return payload
}

// platformFromLink recognises which marketplace a pasted URL belongs to.
func platformFromLink(link string) string {
	lower := strings.ToLower(link)
	switch {
	case strings.Contains(lower, "1688.com"):
		return suppliersourcing.PlatformAlibaba
	case strings.Contains(lower, "taobao.com"), strings.Contains(lower, "tmall.com"), strings.Contains(lower, "tb.cn"):
		return suppliersourcing.PlatformTaobao
	default:
		return ""
	}
}
