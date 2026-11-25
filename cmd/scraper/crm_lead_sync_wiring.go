package main

import (
	"context"
	"log"
	"os"
	"strings"

	"github.com/thg/scraper/internal/crmleadsync"
	"github.com/thg/scraper/internal/store"
)

const defaultCRMLeadSyncURL = "https://crm.thgfulfill.com/api/integrations/thg-tool/leads"

// crmLeadSyncSecret keeps the secret out of source and allows the production
// systemd unit to use a root-owned file rather than an environment dump.
func crmLeadSyncSecret() string {
	if value := strings.TrimSpace(os.Getenv("CRM_LEAD_SYNC_KEY")); value != "" {
		return value
	}
	value, err := os.ReadFile("/etc/thg-scraper/crm_lead_sync_key")
	if err != nil && !os.IsNotExist(err) {
		log.Printf("CRM lead sync key file unreadable: %v", err)
	}
	return strings.TrimSpace(string(value))
}

// NewHub (CRM extension trên ERP) nhận CÙNG bản tin trong giai đoạn chạy song song. Chỉ bật khi có cả
// NEWHUB_LEAD_SYNC_URL lẫn khoá (biến môi trường, hoặc tệp root-only như khoá CRM); thiếu ⇒ chỉ gửi CRM như cũ.
func newHubLeadSyncConfig() (string, string) {
	endpoint := strings.TrimSpace(os.Getenv("NEWHUB_LEAD_SYNC_URL"))
	key := strings.TrimSpace(os.Getenv("NEWHUB_LEAD_SYNC_KEY"))
	if key == "" {
		value, err := os.ReadFile("/etc/thg-scraper/newhub_lead_sync_key")
		if err != nil && !os.IsNotExist(err) {
			log.Printf("NewHub lead sync key file unreadable: %v", err)
		}
		key = strings.TrimSpace(string(value))
	}
	return endpoint, key
}

func startCRMLeadSync(ctx context.Context, mainStore *store.Store) *crmleadsync.Dispatcher {
	if mainStore == nil {
		return nil
	}
	endpoint := strings.TrimSpace(os.Getenv("CRM_LEAD_SYNC_URL"))
	if endpoint == "" {
		endpoint = defaultCRMLeadSyncURL
	}
	dispatcher := crmleadsync.NewDispatcher(crmleadsync.NewStore(mainStore.DB()), endpoint, crmLeadSyncSecret())
	if dispatcher == nil {
		log.Println("CRM lead sync outbox disabled: CRM_LEAD_SYNC_KEY is not configured")
		return nil
	}
	if url, key := newHubLeadSyncConfig(); dispatcher.AddTarget("NewHub", crmleadsync.NewStoreFor(mainStore.DB(), crmleadsync.NewHubOutboxTable), url, key) {
		log.Println("NewHub lead sync outbox enabled (side-by-side with CRM)")
	}
	go dispatcher.Run(ctx)
	log.Printf("Lead sync outbox enabled for connector and worker lead paths: %v", dispatcher.Targets())
	return dispatcher
}
