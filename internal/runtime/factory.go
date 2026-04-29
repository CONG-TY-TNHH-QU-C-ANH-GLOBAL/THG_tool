package runtime

import (
	"context"
	"log"
	"os"

	"github.com/thg/scraper/internal/store"
)

// NewFromSession returns a CDPRuntime if there is an active browser session
// with a reachable CDP port. Mock data is opt-in via ALLOW_MOCK_RUNTIME=true.
func NewFromSession(ctx context.Context, appStore *store.AppStore) Runtime {
	if appStore != nil {
		sess, err := appStore.GetFirstActiveCDPSession(ctx)
		if err == nil && sess != nil && sess.CDPPort > 0 {
			rt, err := NewCDPRuntime(sess.CDPPort)
			if err == nil {
				log.Printf("[Runtime] Using CDPRuntime — account %d cdp_port=%d", sess.AccountID, sess.CDPPort)
				return rt
			}
			log.Printf("[Runtime] CDPRuntime init failed (port %d): %v", sess.CDPPort, err)
		}
	}
	if os.Getenv("ALLOW_MOCK_RUNTIME") == "true" {
		log.Printf("[Runtime] No active browser session; ALLOW_MOCK_RUNTIME=true so using MockRuntime")
		return NewMockRuntime()
	}
	log.Printf("[Runtime] No active browser session")
	return nil
}
