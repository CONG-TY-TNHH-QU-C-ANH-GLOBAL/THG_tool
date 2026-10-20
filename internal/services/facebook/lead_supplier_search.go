package facebook

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/thg/scraper/internal/suppliersourcing"
)

// Each lookup owns its counters/errors. No state is shared between leads.
type supplierSearch struct {
	client                 supplierReader
	query, lang, replyLang string
	quantity, attempts     int
	lookupErr              error
}

func supplierLookupPlatforms(text string) []string {
	text = strings.ToLower(text)
	if strings.Contains(text, "1688") && !strings.Contains(text, "taobao") {
		return []string{suppliersourcing.PlatformAlibaba}
	}
	if strings.Contains(text, "taobao") && !strings.Contains(text, "1688") {
		return []string{suppliersourcing.PlatformTaobao}
	}
	return []string{suppliersourcing.PlatformAlibaba, suppliersourcing.PlatformTaobao}
}

func (s *supplierSearch) lookup(ctx context.Context, platforms []string) (*ResolvedSupplier, error) {
	for _, platform := range platforms {
		if err := supplierLookupBudget(ctx, 9*time.Second); err != nil {
			return nil, err
		}
		items, err := s.client.SearchLocalized(ctx, s.query, platform, 10, s.lang)
		if err != nil {
			if errors.Is(err, suppliersourcing.ErrQuotaExceeded) {
				return nil, err
			}
			s.lookupErr = err
			continue
		}
		resolved, err := s.details(ctx, platform, items)
		if err != nil || resolved != nil {
			return resolved, err
		}
		if s.attempts >= 2 {
			break
		}
	}
	return nil, s.lookupErr
}

func (s *supplierSearch) details(ctx context.Context, platform string, items []suppliersourcing.SearchItem) (*ResolvedSupplier, error) {
	for _, item := range items {
		if !matchesSupplierQuery(s.query, item.Title) || !marketplaceProductURL(item.Link) {
			continue
		}
		if s.attempts >= 2 {
			break
		}
		if err := supplierLookupBudget(ctx, 5*time.Second); err != nil {
			return nil, err
		}
		s.attempts++
		resolved, err := s.detail(ctx, platform, item)
		if err != nil || resolved != nil {
			return resolved, err
		}
	}
	return nil, nil
}

func (s *supplierSearch) detail(ctx context.Context, platform string, item suppliersourcing.SearchItem) (*ResolvedSupplier, error) {
	product, err := s.client.DetailLocalized(ctx, platform, item.ID, item.Link, s.lang)
	if err != nil {
		s.lookupErr = err
		if errors.Is(err, suppliersourcing.ErrQuotaExceeded) {
			return nil, err
		}
		return nil, nil
	}
	if product == nil || !matchesSupplierQuery(s.query, product.Title) {
		return nil, nil
	}
	resolved, err := resolvedSupplier(product, platform, s.quantity, s.replyLang)
	if err != nil {
		return nil, nil // An unusable detail can fall through to the next item.
	}
	return resolved, nil
}
