package export

import (
	"log"
	"os"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/thg/scraper/internal/store"
)

// Env var names that opt the export endpoint in. Both must be set for the
// route to mount; absent config means the endpoint does not exist.
const (
	envServiceKey = "KNOWLEDGE_EXPORT_SERVICE_KEY"
	envOrgID      = "KNOWLEDGE_EXPORT_ORG_ID"
)

// MountIfConfigured registers the export route on group only when both the
// service key and a valid positive org id are present in the environment.
// This keeps the endpoint strictly opt-in: an unconfigured deployment
// exposes nothing new, so enabling the vectorhub sync is a deliberate
// operational step and cannot happen by accident.
func MountIfConfigured(group fiber.Router, db *store.Store) {
	key := os.Getenv(envServiceKey)
	if key == "" {
		return
	}
	orgID, err := strconv.ParseInt(os.Getenv(envOrgID), 10, 64)
	if err != nil || orgID <= 0 {
		log.Printf("[Server] %s set but %s missing/invalid; export endpoint disabled", envServiceKey, envOrgID)
		return
	}
	Routes(group, Deps{DB: db, ServiceKey: key, OrgID: orgID})
	log.Printf("[Server] Knowledge export endpoint enabled for org %d", orgID)
}
