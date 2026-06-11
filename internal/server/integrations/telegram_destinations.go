package integrations

import (
	"encoding/json"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	tgstore "github.com/thg/scraper/internal/store/telegram"
	"github.com/thg/scraper/internal/telegram/control"
)

const connectCodeTTL = 30 * time.Minute

// destinationDTO is the wire shape: decodes event_types and surfaces last_delivery_at; chat_id is
// never exposed.
type destinationDTO struct {
	tgstore.Destination
	EventTypes     []string   `json:"event_types"`
	LastDeliveryAt *time.Time `json:"last_delivery_at"`
}

func toDestinationDTO(d tgstore.Destination) destinationDTO {
	var types []string
	_ = json.Unmarshal([]byte(d.EventTypes), &types)
	if types == nil {
		types = []string{}
	}
	dto := destinationDTO{Destination: d, EventTypes: types}
	if d.LastDeliveryAt.Valid {
		t := d.LastDeliveryAt.Time
		dto.LastDeliveryAt = &t
	}
	return dto
}

// listDestinations returns the org's notification destinations + the available event/filter catalog.
func (h *Handler) listDestinations(c *fiber.Ctx) error {
	orgID, _, _ := reqCtx(c)
	if orgID == 0 {
		return noOrg(c)
	}
	dests, err := h.deps.Control.ListDestinations(orgID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "load destinations failed"})
	}
	out := make([]destinationDTO, 0, len(dests))
	for _, d := range dests {
		out = append(out, toDestinationDTO(d))
	}
	return c.JSON(fiber.Map{
		"destinations":          out,
		"available_event_types": control.EventTypes,
		"available_filters":     control.ChannelFilters,
	})
}

// connectDestination connects a PUBLIC channel by @username (synchronous), OR — for a PRIVATE
// channel (type="private") — issues a one-time connect code the admin posts in the channel.
func (h *Handler) connectDestination(c *fiber.Ctx) error {
	orgID, userID, _ := reqCtx(c)
	if orgID == 0 || userID == 0 {
		return noOrg(c)
	}
	var body struct {
		Type     string `json:"type"`     // "public" (default) | "private"
		Username string `json:"username"` // public channel @handle
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid body"})
	}
	if body.Type == "private" {
		code := tgstore.GenerateCode(8)
		if _, err := h.deps.DB.Telegram().CreateBindCode(orgID, userID, code, connectCodeTTL); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "issue connect code failed"})
		}
		return c.Status(201).JSON(fiber.Map{
			"connect_code": code,
			"instructions": "Thêm bot làm admin của channel riêng tư, rồi đăng tin nhắn: /connect " + code,
			"ttl_seconds":  int(connectCodeTTL.Seconds()),
		})
	}
	if body.Username == "" {
		return c.Status(400).JSON(fiber.Map{"error": "username_required"})
	}
	dest, reason := h.deps.Control.ConnectPublicChannel(orgID, userID, body.Username)
	if dest == nil {
		return c.Status(400).JSON(fiber.Map{"error": reason})
	}
	return c.Status(201).JSON(fiber.Map{"destination": toDestinationDTO(*dest)})
}

// deleteDestination disconnects (soft-disables) a destination.
func (h *Handler) deleteDestination(c *fiber.Ctx) error {
	orgID, id, ok := destID(c)
	if !ok {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}
	okRes, reason := h.deps.Control.DisableDestination(orgID, id)
	if !okRes {
		return c.Status(404).JSON(fiber.Map{"error": reason})
	}
	return c.JSON(fiber.Map{"disconnected": true})
}

// testDestination sends a test notification to a destination.
func (h *Handler) testDestination(c *fiber.Ctx) error {
	orgID, id, ok := destID(c)
	if !ok {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}
	sent, reason := h.deps.Control.TestDestination(orgID, id)
	if !sent {
		return c.Status(400).JSON(fiber.Map{"error": reason})
	}
	return c.JSON(fiber.Map{"sent": true})
}

// updateDestinationPreferences sets a destination's subscribed event types + channel filter.
func (h *Handler) updateDestinationPreferences(c *fiber.Ctx) error {
	orgID, id, ok := destID(c)
	if !ok {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}
	var body struct {
		EventTypes    []string `json:"event_types"`
		ChannelFilter string   `json:"channel_filter"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid body"})
	}
	okRes, reason := h.deps.Control.SetDestinationPreferences(orgID, id, body.EventTypes, body.ChannelFilter)
	if !okRes {
		return c.Status(400).JSON(fiber.Map{"error": reason})
	}
	return c.JSON(fiber.Map{"updated": true})
}

func destID(c *fiber.Ctx) (orgID, id int64, ok bool) {
	orgID, _, _ = reqCtx(c)
	if orgID == 0 {
		return 0, 0, false
	}
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil || id <= 0 {
		return orgID, 0, false
	}
	return orgID, id, true
}
