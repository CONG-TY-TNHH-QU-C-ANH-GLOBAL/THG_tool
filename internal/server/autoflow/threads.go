package autoflow

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
)

func (h *Handler) autoflowListThreads(c *fiber.Ctx) error {
	orgID := c.Locals("org_id").(int64)
	threads, err := h.deps.DB.GetThreadsByOrg(orgID, 100)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	unreadCount, err := h.deps.DB.CountThreadUnreadByOrg(orgID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	type row struct {
		ID          int64     `json:"id"`
		ProfileName string    `json:"profile_name"`
		ProfileURL  string    `json:"profile_url"`
		Status      string    `json:"status"`
		UnreadCount int       `json:"unread_count"`
		LastMessage string    `json:"last_message"`
		LastAt      time.Time `json:"last_at"`
	}
	out := make([]row, 0, len(threads))
	for _, t := range threads {
		out = append(out, row{
			ID: t.ID, ProfileName: t.ProfileName, ProfileURL: t.ProfileURL,
			Status: t.Status, UnreadCount: t.UnreadCount, LastMessage: t.LastMessage, LastAt: t.LastAt,
		})
	}
	return c.JSON(fiber.Map{"threads": out, "count": len(out), "unread_count": unreadCount})
}

func (h *Handler) autoflowGetMessages(c *fiber.Ctx) error {
	orgID := c.Locals("org_id").(int64)
	threadID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}
	ok, err := h.deps.DB.ThreadBelongsToOrg(threadID, orgID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	if !ok {
		return c.Status(404).JSON(fiber.Map{"error": "thread not found"})
	}
	_ = h.deps.DB.ClearThreadUnread(threadID)
	msgs, err := h.deps.DB.GetThreadMessages(threadID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"messages": msgs})
}

func (h *Handler) autoflowSendMessage(c *fiber.Ctx) error {
	orgID := c.Locals("org_id").(int64)
	threadID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}
	ok, err := h.deps.DB.ThreadBelongsToOrg(threadID, orgID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	if !ok {
		return c.Status(404).JSON(fiber.Map{"error": "thread not found"})
	}
	var body struct {
		Content string `json:"content"`
	}
	if err := c.BodyParser(&body); err != nil || body.Content == "" {
		return c.Status(400).JSON(fiber.Map{"error": "content required"})
	}
	if err := h.deps.DB.AddThreadMessage(threadID, "outbound", body.Content, false); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(201).JSON(fiber.Map{
		"direction":  "outbound",
		"content":    body.Content,
		"created_at": time.Now(),
	})
}

// Ã¢â€â‚¬Ã¢â€â‚¬ Facebook Session Summary Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬

func (h *Handler) autoflowFacebookStatus(c *fiber.Ctx) error {
	orgID := c.Locals("org_id").(int64)
	summary := h.deps.DB.Identities().GetFacebookStatusForOrg(orgID)
	return c.JSON(fiber.Map{
		"connected":   summary.Connected,
		"account":     summary.Account,
		"groups":      summary.Groups,
		"leads_today": summary.LeadsToday,
	})
}

func orgContextKey(orgID int64, name string) string {
	return fmt.Sprintf("org:%d:%s", orgID, name)
}

func (h *Handler) getBusinessContext(c *fiber.Ctx) error {
	orgID := c.Locals("org_id").(int64)
	files, _ := h.deps.DB.GetContext(orgContextKey(orgID, "private_files_summary"))
	sources, _ := h.deps.DB.GetContext(orgContextKey(orgID, "data_sources_summary"))
	resp := fiber.Map{
		"private_files": files,
		"data_sources":  sources,
	}
	for _, key := range businessCalibrationKeys() {
		value, _ := h.deps.DB.GetContext(orgContextKey(orgID, key))
		resp[key] = value
	}
	return c.JSON(resp)
}

func (h *Handler) updateBusinessContext(c *fiber.Ctx) error {
	orgID := c.Locals("org_id").(int64)
	var body struct {
		BusinessProfile  string `json:"business_profile"`
		BusinessName     string `json:"business_name"`
		BusinessIndustry string `json:"business_industry"`
		Services         string `json:"services"`
		TargetCustomers  string `json:"target_customers"`
		TargetAuthorRole string `json:"target_author_role"`
		TargetSignals    string `json:"target_signals"`
		NegativeSignals  string `json:"negative_signals"`
		BusinessLocation string `json:"business_location"`
		Markets          string `json:"markets"`
		BusinessUSP      string `json:"business_usp"`
		Tone             string `json:"tone"`
		ApprovalPolicy   string `json:"approval_policy"`
		RejectRules      string `json:"reject_rules"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid body"})
	}
	values := map[string]string{
		"business_profile":   strings.TrimSpace(body.BusinessProfile),
		"business_name":      strings.TrimSpace(body.BusinessName),
		"business_industry":  strings.TrimSpace(body.BusinessIndustry),
		"services":           strings.TrimSpace(body.Services),
		"target_customers":   strings.TrimSpace(body.TargetCustomers),
		"target_author_role": normalizeTargetAuthorRole(body.TargetAuthorRole),
		"target_signals":     strings.TrimSpace(body.TargetSignals),
		"negative_signals":   strings.TrimSpace(body.NegativeSignals),
		"business_location":  strings.TrimSpace(body.BusinessLocation),
		"markets":            strings.TrimSpace(body.Markets),
		"business_usp":       strings.TrimSpace(body.BusinessUSP),
		"tone":               strings.TrimSpace(body.Tone),
		"approval_policy":    strings.TrimSpace(body.ApprovalPolicy),
		"reject_rules":       strings.TrimSpace(body.RejectRules),
	}
	if values["business_profile"] == "" {
		values["business_profile"] = buildBusinessCalibrationSummary(values)
	}
	for _, key := range businessCalibrationKeys() {
		if err := h.deps.DB.SetContext(orgContextKey(orgID, key), values[key]); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
	}
	return c.JSON(fiber.Map{"ok": true, "business_profile": values["business_profile"]})
}

func businessCalibrationKeys() []string {
	return []string{
		"business_profile",
		"business_name",
		"business_industry",
		"services",
		"target_customers",
		"target_author_role",
		"target_signals",
		"negative_signals",
		"business_location",
		"markets",
		"business_usp",
		"tone",
		"approval_policy",
		"reject_rules",
	}
}

func normalizeTargetAuthorRole(raw string) string {
	v := strings.ToLower(strings.TrimSpace(raw))
	switch v {
	case "supplier", "suppliers":
		return "suppliers"
	case "partner", "partners", "reseller", "resellers":
		return "partners"
	case "candidate", "candidates":
		return "candidates"
	case "provider", "providers", "service_providers":
		return "providers"
	default:
		return "customers"
	}
}

func buildBusinessCalibrationSummary(values map[string]string) string {
	var b strings.Builder
	for _, item := range []struct {
		label string
		key   string
	}{
		{"Business", "business_name"},
		{"Industry", "business_industry"},
		{"Services", "services"},
		{"Target customers", "target_customers"},
		{"Target author role", "target_author_role"},
		{"Target signals", "target_signals"},
		{"Negative signals", "negative_signals"},
		{"Markets", "markets"},
		{"USP", "business_usp"},
		{"Tone", "tone"},
		{"Approval policy", "approval_policy"},
		{"Reject rules", "reject_rules"},
	} {
		if value := strings.TrimSpace(values[item.key]); value != "" {
			if b.Len() > 0 {
				b.WriteByte('\n')
			}
			b.WriteString(item.label)
			b.WriteString(": ")
			b.WriteString(value)
		}
	}
	return b.String()
}

func (h *Handler) billingSummary(c *fiber.Ctx) error {
	orgID := c.Locals("org_id").(int64)
	org, err := h.deps.DB.GetOrganization(orgID)
	if err != nil || org == nil {
		return c.Status(404).JSON(fiber.Map{"error": "organization not found"})
	}
	accountCount, _ := h.deps.DB.CountAccountsByOrg(orgID)
	staff, _ := h.deps.DB.GetStaffWithKPI(orgID)
	fb := h.deps.DB.Identities().GetFacebookStatusForOrg(orgID)
	outboxCounts, _ := h.deps.DB.CountOutboundByStatusForOrg(orgID)
	return c.JSON(fiber.Map{
		"plan_tier":      org.PlanTier,
		"max_accounts":   org.MaxAccounts,
		"account_count":  accountCount,
		"staff_count":    len(staff),
		"groups":         fb.Groups,
		"leads_today":    fb.LeadsToday,
		"outbox_counts":  outboxCounts,
		"payment_status": "manual",
	})
}
