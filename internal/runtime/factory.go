package runtime

import (
	"context"
	"log"

	"github.com/thg/scraper/internal/store"
)

// NewFromSession returns a CDPRuntime if there is an active browser session
// with a reachable CDP port, otherwise falls back to MockRuntime.
func NewFromSession(ctx context.Context, appStore *store.AppStore) Runtime {
	if appStore != nil {
		sess, err := appStore.GetFirstActiveCDPSession(ctx)
		if err == nil && sess != nil && sess.CDPPort > 0 {
			rt, err := NewCDPRuntime(sess.CDPPort)
			if err == nil {
				log.Printf("[Runtime] Using CDPRuntime — account %d cdp_port=%d", sess.AccountID, sess.CDPPort)
				return rt
			}
			log.Printf("[Runtime] CDPRuntime init failed (port %d): %v — falling back to mock", sess.CDPPort, err)
		}
	}
	log.Printf("[Runtime] No active browser session — using MockRuntime")
	return NewMockRuntime()
}
