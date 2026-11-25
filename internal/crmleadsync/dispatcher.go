package crmleadsync

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/thg/scraper/internal/leadingest"
	"github.com/thg/scraper/internal/models"
)

// target is one receiver of the enriched lead events. Every target owns its
// outbox table, so a slow or failing receiver never holds back another one.
type target struct {
	name   string
	store  *Store
	url    string
	secret string
}

// Dispatcher delivers the exact lead snapshot to the CRM and, while the two
// systems run side by side, to the NewHub CRM extension as well.
type Dispatcher struct {
	targets []target
	client  *http.Client
}

func newTarget(name string, store *Store, endpoint, secret string) (target, bool) {
	if store == nil || strings.TrimSpace(endpoint) == "" || strings.TrimSpace(secret) == "" {
		return target{}, false
	}
	return target{name: name, store: store, url: strings.TrimRight(strings.TrimSpace(endpoint), "/"), secret: secret}, true
}

func NewDispatcher(store *Store, endpoint, secret string) *Dispatcher {
	t, ok := newTarget("CRM", store, endpoint, secret)
	if !ok {
		return nil
	}
	return &Dispatcher{targets: []target{t}, client: &http.Client{Timeout: 8 * time.Second}}
}

// AddTarget adds another receiver (for example NewHub). It returns false and
// changes nothing when the endpoint or the secret is not configured, so an
// unconfigured NewHub leaves the CRM delivery exactly as it was.
func (d *Dispatcher) AddTarget(name string, store *Store, endpoint, secret string) bool {
	if d == nil {
		return false
	}
	t, ok := newTarget(name, store, endpoint, secret)
	if !ok {
		return false
	}
	for _, existing := range d.targets {
		if existing.store.table == store.table {
			panic("crmleadsync: two targets share outbox table " + store.table)
		}
	}
	d.targets = append(d.targets, t)
	return true
}

// Targets names the configured receivers, for start-up logs.
func (d *Dispatcher) Targets() []string {
	names := make([]string, 0, len(d.targets))
	for _, t := range d.targets {
		names = append(names, t.name)
	}
	return names
}

func (d *Dispatcher) Run(ctx context.Context) {
	d.Dispatch(ctx)
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.Dispatch(ctx)
		}
	}
}

// Enqueue queues the same snapshot for every target. A failure on one target
// does not stop the others; the first error is returned for logging.
func (d *Dispatcher) Enqueue(ctx context.Context, event leadingest.LeadEvent, suggestions ...models.LeadSuggestion) error {
	var first error
	for _, t := range d.targets {
		if err := t.store.Enqueue(ctx, event, suggestions...); err != nil {
			err = fmt.Errorf("%s: %w", t.name, err)
			if first == nil {
				first = err
			}
			slog.ErrorContext(ctx, "lead sync enqueue failed", "target", t.name, "error", err)
		}
	}
	return first
}

func (d *Dispatcher) Dispatch(ctx context.Context) {
	for _, t := range d.targets {
		items, err := t.store.Claim(ctx, time.Now(), 10)
		if err != nil {
			slog.ErrorContext(ctx, "lead sync claim failed", "target", t.name, "error", err)
			continue
		}
		for _, item := range items {
			d.deliver(ctx, t, item)
		}
	}
}

func (d *Dispatcher) deliver(ctx context.Context, t target, item item) {
	body, err := json.Marshal(item.Payload)
	if err != nil {
		_ = t.store.Block(ctx, item, fmt.Sprintf("encode payload: %v", err))
		return
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.url, bytes.NewReader(body))
	if err != nil {
		_ = t.store.Block(ctx, item, fmt.Sprintf("build request: %v", err))
		return
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("x-thg-integration-key", t.secret)
	response, err := d.client.Do(req)
	if err != nil {
		_ = t.store.Retry(ctx, item, time.Now(), err.Error())
		return
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	if response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices {
		if err := t.store.Succeed(ctx, item.ID); err != nil {
			slog.ErrorContext(ctx, "lead sync acknowledgement failed", "target", t.name, "id", item.ID, "error", err)
		}
		return
	}
	message := fmt.Sprintf("%s responded %d", t.name, response.StatusCode)
	if response.StatusCode >= http.StatusBadRequest && response.StatusCode < http.StatusInternalServerError && response.StatusCode != http.StatusTooManyRequests {
		_ = t.store.Block(ctx, item, message)
		return
	}
	_ = t.store.Retry(ctx, item, time.Now(), message)
}
