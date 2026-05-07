package runtime

import (
	"context"
	"fmt"
	"time"
)

// MockRuntime returns deterministic fake items for MVP testing.
// Each call simulates a page of results; returns empty when offset >= totalItems.
//
// DEV-ONLY: production must never instantiate this. cmd/worker only constructs
// it when the env var ALLOW_MOCK_RUNTIME=true is set; cmd/scraper never does.
// If you find this in a code path that runs against real tenants, that's a
// bug — fail loudly instead of falling back to fake leads.
type MockRuntime struct {
	TotalItems int
	Delay      time.Duration
}

func NewMockRuntime() *MockRuntime {
	return &MockRuntime{
		TotalItems: 50,
		Delay:      100 * time.Millisecond,
	}
}

func (m *MockRuntime) FetchBatch(ctx context.Context, sourceURL string, offset, batchSize int) ([]RawItem, error) {
	if offset >= m.TotalItems {
		return nil, nil
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(m.Delay):
	}

	end := offset + batchSize
	if end > m.TotalItems {
		end = m.TotalItems
	}

	items := make([]RawItem, 0, end-offset)
	for i := offset; i < end; i++ {
		items = append(items, RawItem{
			ID:               fmt.Sprintf("item_%s_%d", sourceKey(sourceURL), i),
			Content:          fmt.Sprintf("Mock post %d từ %s — đang tìm kiếm khách hàng mua hàng mỹ giá tốt", i, sourceURL),
			AuthorName:       fmt.Sprintf("User %d", i),
			AuthorProfileURL: fmt.Sprintf("https://facebook.com/user%d", i),
			SourceURL:        sourceURL,
			Timestamp:        time.Now().UTC().Add(-time.Duration(i) * time.Hour),
			Reactions:        (i % 10) * 3,
			Comments:         i % 5,
			Shares:           i % 3,
		})
	}
	return items, nil
}

func sourceKey(url string) string {
	if len(url) > 20 {
		return url[len(url)-10:]
	}
	return url
}
