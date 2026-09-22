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
	go dispatcher.Run(ctx)
	log.Println("CRM lead sync outbox enabled for connector and worker lead paths")
	return dispatcher
}

// crmEnrichURL is the CRM origin the suggestion builder asks for product price
// and shipping cost. Same host as the lead-sync endpoint, so one override
// covers a staging CRM.
func crmEnrichURL() string {
	if value := strings.TrimSpace(os.Getenv("CRM_ENRICH_URL")); value != "" {
		return value
	}
	return "https://crm.thgfulfill.com"
}
