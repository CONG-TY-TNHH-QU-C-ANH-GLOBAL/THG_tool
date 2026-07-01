package outbox

import (
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/server/system"
)

func (h *Handler) getOutbox(c *fiber.Ctx) error {
	orgID := c.Locals("org_id").(int64)
	status := c.Query("status", "")
	msgType := c.Query("type", "")
	limit, _ := strconv.Atoi(c.Query("limit", "50"))
	messages, err := h.db.Outbound().ListByLegacyStatus(orgID, status, msgType, limit)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	counts, _ := h.db.Outbound().CountByState(orgID)
	// Executor attribution projection: resolve each row's account_id to its
	// Facebook identity so the operator sees "đăng bởi <FB actor>", not a bare
	// #account_id. Distinct from CreatedBy (the initiating principal). Best
	// effort — a lookup failure must not break the dashboard list.
	// See specs/COMMENT_INTELLIGENCE_PIPELINE.md §7a (P1a).
	actors, _ := h.db.Identities().AccountIdentitiesForOrg(orgID)
	// Fold the Verified-Actor state (P1b) into the actor projection so the UI
	// can render the verdict chip / blocked badge per account. Composed at the
	// API boundary — identities stays pure, coordination owns the verdict.
	if states, sErr := h.db.Coordination().AccountActorStatesForOrg(c.UserContext(), orgID); sErr == nil {
		for accID, st := range states {
			if a, ok := actors[accID]; ok {
				a.ActorVerdict = st.Verdict
				a.ActorBlocked = st.Blocked
				actors[accID] = a
			}
		}
	}
	// Latest-effective-outcome overlay: a comment whose async/manual correction appended a
	// 'succeeded' ledger row must show as posted even though its (append-only, never-mutated)
	// verification_outcome stays submitted_unverified. The FE renders the effective outcome.
	commentIDs := make([]int64, 0, len(messages))
	for _, m := range messages {
		if m.Type == "comment" {
			commentIDs = append(commentIDs, m.ID)
		}
	}
	corrections, _ := h.db.Coordination().CommentCorrectionsForOutbounds(c.UserContext(), orgID, commentIDs)
	return c.JSON(fiber.Map{"messages": messages, "count": len(messages), "counts": counts, "actors": actors, "corrections": corrections})
}

// clearActorBlock is the operator override that lifts a Verified-Actor block
// on one account (P1b). Admin-only. The account can auto-execute again after
// the operator has confirmed the correct Facebook identity is logged in.
func (h *Handler) clearActorBlock(c *fiber.Ctx) error {
	orgID := c.Locals("org_id").(int64)
	accountID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil || accountID <= 0 {
		return c.Status(400).JSON(fiber.Map{"error": "invalid account id"})
	}
	if err := h.db.Coordination().ClearActorBlock(c.UserContext(), orgID, accountID); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"cleared": true, "account_id": accountID})
}

func (h *Handler) draftOutbound(c *fiber.Ctx) error {
	var req struct {
		Type       string `json:"type"` // comment, inbox
		AccountID  int64  `json:"account_id"`
		TargetURL  string `json:"target_url"`
		TargetName string `json:"target_name"`
		Content    string `json:"content"` // manual content (optional, AI generates if empty)
		Context    string `json:"context"` // original post for AI context
		Auto       bool   `json:"auto"`    // true = queue as approved for immediate agent execution
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}

	if req.Type == "" {
		req.Type = "comment"
	}
	if req.Type != "comment" && req.Type != "inbox" && req.Type != "group_post" && req.Type != "profile_post" {
		return c.Status(400).JSON(fiber.Map{"error": "unsupported outbound type"})
	}
	orgID := c.Locals("org_id").(int64)
	userID, _ := c.Locals("user_id").(int64)
	role, _ := c.Locals("user_role").(string)
	if req.AccountID <= 0 {
		return c.Status(400).JSON(fiber.Map{"error": "account_id is required"})
	}
	// RBAC-1: execution-layer ownership. Sales staff can only queue outbound
	// against accounts they own. Admin / platform roles pass through.
	// See feedback_shared_battlefield_not_crm.md.
	if _, err := h.requireAccountOwner(h.db, c, req.AccountID, orgID, userID, role); err != nil {
		return err
	}

	// Route through the canonical write path so the per-account dedup index +
	// action ledger (Coordination Plane PR-1) apply. Previously called
	// InsertOutboundMessage directly — that was the HTTP-bypass gap flagged
	// in project_outbound_audit_findings.md Critical #1.
	queueRes, err := h.db.Outbound().Queue(&models.OutboundMessage{
		OrgID:      orgID,
		Type:       req.Type,
		Platform:   models.PlatformFacebook,
		AccountID:  req.AccountID,
		TargetURL:  req.TargetURL,
		TargetName: req.TargetName,
		Content:    req.Content,
		Context:    req.Context,
		CreatedBy:  userID, // immutable execution ownership (Organic Sales Network)
	}, 24*time.Hour)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	if !queueRes.Decision.Allowed {
		return c.Status(409).JSON(fiber.Map{
			"error":       "outbound_blocked",
			"reason":      queueRes.Decision.Reason,
			"existing_id": queueRes.Decision.ExistingID,
		})
	}

	if queueRes.ExecutionState == models.ExecPlanned && h.wsReady != nil {
		h.wsReady.NotifyOutboxReady(1)
	}
	system.NotifyOutboundQueued(h.db, h.notifier, orgID, req.AccountID, queueRes.ID, req.Type, queueRes.ExecutionState)
	return c.Status(201).JSON(fiber.Map{
		"message_id":      queueRes.ID,
		"execution_state": string(queueRes.ExecutionState),
	})
}
