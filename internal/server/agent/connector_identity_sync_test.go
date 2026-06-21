package agent

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/server/agent/account"
	"github.com/thg/scraper/internal/store"
	"github.com/thg/scraper/internal/store/storetest"
)

// heartbeatTimeoutMs bounds app.Test so it WAITS for the synchronous heartbeat
// handler (account create + identity upsert + session + presence — several SQLite
// writes) to finish before returning. Fiber's default is 1000ms; on a slow/loaded
// CI runner the first bind can exceed that, and a timed-out app.Test orphans the
// still-running handler goroutine, which then hits the test DB after t.Cleanup
// closes it ("sql: database is closed"). 10s is generous enough that only a real
// hang fails — it does not mask the lifecycle bug, it removes the orphaned
// goroutine by awaiting the (synchronous, bounded) handler.
const heartbeatTimeoutMs = 10_000

// heartbeatProof posts a connector heartbeat as the given agent principal and
// returns the HTTP status + decoded JSON body. Because the handler binds the
// account synchronously, a 2xx response guarantees the bind has committed (no
// background work outlives this call) — the caller can assert immediately.
func heartbeatProof(t *testing.T, h *Handler, agentID, orgID, createdBy, assigned int64, jsonBody string) (int, map[string]any) {
	t.Helper()
	app := fiber.New()
	app.Post("/heartbeat", func(c *fiber.Ctx) error {
		c.Locals("agent_id", agentID)
		c.Locals("agent_org_id", orgID)
		c.Locals("agent_created_by", createdBy)
		c.Locals("agent_assigned_account_id", assigned)
		return h.agentHeartbeat(c)
	})
	req := httptest.NewRequest("POST", "/heartbeat", strings.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, heartbeatTimeoutMs)
	if err != nil {
		t.Fatalf("heartbeat request: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	out := map[string]any{}
	_ = json.Unmarshal(raw, &out)
	return resp.StatusCode, out
}

func fbAccountCount(t *testing.T, db *store.Store, orgID int64) int {
	t.Helper()
	accs, err := db.Identities().GetAllAccounts(orgID)
	if err != nil {
		t.Fatalf("GetAllAccounts: %v", err)
	}
	n := 0
	for _, a := range accs {
		if a.Platform == models.PlatformFacebook {
			n++
		}
	}
	return n
}

func insertConnector(t *testing.T, db *store.Store, orgID, createdBy int64, hash string) int64 {
	t.Helper()
	res, err := db.DB().Exec(
		`INSERT INTO agent_tokens (org_id, name, created_by, token_hash, kind, transport,
		   assigned_account_id, stream_status, version, active, last_seen, created_at)
		 VALUES (?, 'ext', ?, ?, 'extension_connector', 'chrome_extension', 0, 'idle', '0.5.55', 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		orgID, createdBy, hash,
	)
	if err != nil {
		t.Fatalf("insert connector: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

// The pairing → session-proof → account-connected lifecycle. Pairing alone must
// NOT create a Facebook account; a heartbeat carrying a logged-in fb_user_id binds
// the account to its owner+workspace (idempotently); a different member reporting
// the same fb_user_id is rejected as an ownership conflict. This is the bug where
// the popup showed an FB session but the dashboard stayed at 0 accounts.
func TestHeartbeatBindsFacebookAccount(t *testing.T) {
	dst := storetest.CopyTemplate(t, bootstrapInputRBACStore, "hb_bind.db")
	db, err := store.New(dst)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	orgID, err := db.CreateOrganization(&models.Organization{Name: "THG Fulfill", PlanTier: models.PlanFree, Active: true})
	if err != nil {
		t.Fatalf("create org: %v", err)
	}
	ownerA, _ := db.CreateUser(&models.User{OrgID: orgID, Email: "a@example.com", Name: "Seller A", PasswordHash: "x", Role: models.RoleSales})
	memberB, _ := db.CreateUser(&models.User{OrgID: orgID, Email: "b@example.com", Name: "Seller B", PasswordHash: "x", Role: models.RoleSales})
	connA := insertConnector(t, db, orgID, ownerA, "hashA")

	h := &Handler{db: db}
	const fbUID = "61577875521752"
	proof := `{"fb_user_id":"` + fbUID + `","fb_display_name":"Seller A FB","stream_status":"facebook_logged_in"}`

	// 1. Pairing alone (connector exists, no session proof) → ZERO Facebook accounts.
	if n := fbAccountCount(t, db, orgID); n != 0 {
		t.Fatalf("pairing alone must not create an account, got %d", n)
	}

	// 2. Heartbeat with a logged-in fb_user_id → account created + bound to owner A.
	if code, out := heartbeatProof(t, h, connA, orgID, ownerA, 0, proof); code != 200 {
		t.Fatalf("heartbeat status=%d (%v)", code, out)
	}
	acc, err := db.Identities().GetAccountByFacebookIdentity(orgID, fbUID)
	if err != nil || acc == nil {
		t.Fatalf("account not bound after session proof: acc=%v err=%v", acc, err)
	}
	if acc.AssignedUserID != ownerA {
		t.Fatalf("account owner = %d, want %d", acc.AssignedUserID, ownerA)
	}
	if n := fbAccountCount(t, db, orgID); n != 1 {
		t.Fatalf("want 1 FB account after proof, got %d", n)
	}

	// 5 + 6. Dashboard readiness matrix now returns the connected account (8: a
	// 'detected' verification corresponds to a really-persisted account).
	matrix, err := account.BuildAccountReadinessMatrix(db, orgID, ownerA, "sales")
	if err != nil || len(matrix) != 1 || matrix[0].FBUserID != fbUID {
		t.Fatalf("readiness matrix = %v err=%v, want 1 account with fb %s", matrix, err, fbUID)
	}

	// 3. Same owner reconnecting the same fb_user_id → idempotent refresh, no dup.
	if code, _ := heartbeatProof(t, h, connA, orgID, ownerA, acc.ID, proof); code != 200 {
		t.Fatalf("re-proof status=%d", code)
	}
	if n := fbAccountCount(t, db, orgID); n != 1 {
		t.Fatalf("re-proof must not duplicate, got %d FB accounts", n)
	}

	// 4. A DIFFERENT member reporting the SAME fb_user_id → ownership conflict.
	connB := insertConnector(t, db, orgID, memberB, "hashB")
	code, out := heartbeatProof(t, h, connB, orgID, memberB, 0, proof)
	if code != 409 {
		t.Fatalf("cross-member proof status=%d (%v), want 409", code, out)
	}
	if out["error_code"] != connectorIdentityConflictCode {
		t.Fatalf("conflict error_code = %v, want %s", out["error_code"], connectorIdentityConflictCode)
	}
	// Ownership never stolen: the account still belongs to A.
	if acc2, _ := db.Identities().GetAccountByFacebookIdentity(orgID, fbUID); acc2 == nil || acc2.AssignedUserID != ownerA {
		t.Fatalf("ownership must remain with A after conflict: %v", acc2)
	}
}
