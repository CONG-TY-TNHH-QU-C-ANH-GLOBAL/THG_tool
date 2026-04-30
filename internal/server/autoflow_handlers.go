package server

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/thg/scraper/internal/store"
)

// ── Staff + KPI ───────────────────────────────────────────────────────────

func (s *Server) autoflowGetStaff(c *fiber.Ctx) error {
	orgID := c.Locals("org_id").(int64)
	staff, err := s.db.GetStaffWithKPI(orgID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	type row struct {
		ID        int64  `json:"id"`
		OrgID     int64  `json:"org_id"`
		Name      string `json:"name"`
		Email     string `json:"email"`
		Role      string `json:"role"`
		Status    string `json:"status"`
		Joined    string `json:"joined"`
		Convs     int    `json:"convs"`
		Converted int    `json:"converted"`
		Cmts      int    `json:"cmts"`
		Pts       int    `json:"pts"`
	}
	out := make([]row, 0, len(staff))
	for _, m := range staff {
		status := "Active"
		if !m.Active {
			status = "Suspended"
		}
		out = append(out, row{
			ID: m.UserID, OrgID: m.OrgID, Name: m.Name, Email: m.Email, Role: m.Role,
			Status: status, Joined: m.Joined,
			Convs: m.Convs, Converted: m.Converted, Cmts: m.Cmts, Pts: m.Pts,
		})
	}
	return c.JSON(fiber.Map{"staff": out, "count": len(out)})
}

func (s *Server) autoflowUpdateKPI(c *fiber.Ctx) error {
	orgID := c.Locals("org_id").(int64)
	staffID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}
	var body struct {
		Convs     *int `json:"convs"`
		Converted *int `json:"converted"`
		Cmts      *int `json:"cmts"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid body"})
	}
	if err := s.db.UpsertStaffKPI(staffID, orgID, store.KPIDelta{
		Convs:     body.Convs,
		Converted: body.Converted,
		Cmts:      body.Cmts,
	}); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"ok": true})
}

// ── KPI Config ────────────────────────────────────────────────────────────

func (s *Server) autoflowGetKPIConfig(c *fiber.Ctx) error {
	orgID := c.Locals("org_id").(int64)
	cfg, err := s.db.GetKPIConfig(orgID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{
		"conv_pts":  cfg.ConvPts,
		"conv2_pts": cfg.Conv2Pts,
		"cmt_pts":   cfg.CmtPts,
		"bonus_pts": cfg.BonusPts,
		"bonus_amt": cfg.BonusAmt,
		"pen_pts":   cfg.PenPts,
		"pen_amt":   cfg.PenAmt,
	})
}

func (s *Server) autoflowUpdateKPIConfig(c *fiber.Ctx) error {
	orgID := c.Locals("org_id").(int64)
	var body struct {
		ConvPts  *int `json:"conv_pts"`
		Conv2Pts *int `json:"conv2_pts"`
		CmtPts   *int `json:"cmt_pts"`
		BonusPts *int `json:"bonus_pts"`
		BonusAmt *int `json:"bonus_amt"`
		PenPts   *int `json:"pen_pts"`
		PenAmt   *int `json:"pen_amt"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid body"})
	}
	existing, _ := s.db.GetKPIConfig(orgID)
	cfg := store.KPIConfig{OrgID: orgID, ConvPts: existing.ConvPts, Conv2Pts: existing.Conv2Pts, CmtPts: existing.CmtPts, BonusPts: existing.BonusPts, BonusAmt: existing.BonusAmt, PenPts: existing.PenPts, PenAmt: existing.PenAmt}
	if body.ConvPts != nil {
		cfg.ConvPts = *body.ConvPts
	}
	if body.Conv2Pts != nil {
		cfg.Conv2Pts = *body.Conv2Pts
	}
	if body.CmtPts != nil {
		cfg.CmtPts = *body.CmtPts
	}
	if body.BonusPts != nil {
		cfg.BonusPts = *body.BonusPts
	}
	if body.BonusAmt != nil {
		cfg.BonusAmt = *body.BonusAmt
	}
	if body.PenPts != nil {
		cfg.PenPts = *body.PenPts
	}
	if body.PenAmt != nil {
		cfg.PenAmt = *body.PenAmt
	}
	if err := s.db.UpsertKPIConfig(orgID, cfg); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"ok": true})
}

// ── Private Files ─────────────────────────────────────────────────────────

const fileUploadDir = "data/files"
const maxFileSize = 50 * 1024 * 1024 // 50 MB

var allowedMimes = map[string]bool{
	"application/pdf": true,
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document": true,
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":       true,
	"text/plain": true,
	"text/csv":   true,
}

func (s *Server) autoflowListFiles(c *fiber.Ctx) error {
	orgID := c.Locals("org_id").(int64)
	files, err := s.db.GetPrivateFiles(orgID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	type row struct {
		ID        int64     `json:"id"`
		Name      string    `json:"name"`
		SizeBytes int64     `json:"size_bytes"`
		MimeType  string    `json:"mime_type"`
		CreatedAt time.Time `json:"created_at"`
	}
	out := make([]row, 0, len(files))
	for _, f := range files {
		out = append(out, row{ID: f.ID, Name: f.Name, SizeBytes: f.SizeBytes, MimeType: f.MimeType, CreatedAt: f.CreatedAt})
	}
	return c.JSON(fiber.Map{"files": out, "count": len(out)})
}

func (s *Server) autoflowUploadFile(c *fiber.Ctx) error {
	orgID := c.Locals("org_id").(int64)
	fh, err := c.FormFile("file")
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "no file provided"})
	}
	if fh.Size > maxFileSize {
		return c.Status(413).JSON(fiber.Map{"error": "file too large (max 50MB)"})
	}
	mime := fh.Header.Get("Content-Type")
	if !allowedMimes[mime] {
		return c.Status(415).JSON(fiber.Map{"error": "unsupported file type"})
	}
	dir := filepath.Join(fileUploadDir, strconv.FormatInt(orgID, 10))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "storage error"})
	}
	safeName := fmt.Sprintf("%d_%s", time.Now().UnixMilli(), filepath.Base(fh.Filename))
	dest := filepath.Join(dir, safeName)
	if err := c.SaveFile(fh, dest); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "save error"})
	}
	rec := &store.PrivateFile{
		OrgID:     orgID,
		Name:      fh.Filename,
		Path:      dest,
		SizeBytes: fh.Size,
		MimeType:  mime,
	}
	id, err := s.db.InsertPrivateFile(rec)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	s.refreshPrivateFilesContext(orgID)
	return c.Status(201).JSON(fiber.Map{
		"id": id, "name": fh.Filename, "size_bytes": fh.Size, "mime_type": mime, "created_at": time.Now(),
	})
}

func (s *Server) refreshPrivateFilesContext(orgID int64) {
	key := orgContextKey(orgID, "private_files_summary")
	files, err := s.db.GetPrivateFiles(orgID)
	if err != nil {
		return
	}
	var b strings.Builder
	for _, file := range files {
		snippet := ""
		if file.MimeType == "text/plain" || file.MimeType == "text/csv" {
			if f, err := os.Open(file.Path); err == nil {
				func() {
					defer f.Close()
					bytes, _ := io.ReadAll(io.LimitReader(f, 4096))
					snippet = strings.TrimSpace(string(bytes))
				}()
			}
		}
		b.WriteString(fmt.Sprintf("- %s (%s)", file.Name, file.MimeType))
		if snippet != "" {
			b.WriteString("\n  Notes: " + snippet)
		}
		b.WriteString("\n")
	}
	_ = s.db.SetContext(key, strings.TrimSpace(b.String()))
}

func (s *Server) autoflowDeleteFile(c *fiber.Ctx) error {
	orgID := c.Locals("org_id").(int64)
	fileID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}
	files, _ := s.db.GetPrivateFiles(orgID)
	var path string
	for _, f := range files {
		if f.ID == fileID {
			path = f.Path
			break
		}
	}
	if path != "" {
		_ = os.Remove(path)
	}
	if err := s.db.DeletePrivateFile(fileID, orgID); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	s.refreshPrivateFilesContext(orgID)
	return c.JSON(fiber.Map{"ok": true})
}

// ── Conversation Threads ──────────────────────────────────────────────────

func (s *Server) autoflowListThreads(c *fiber.Ctx) error {
	orgID := c.Locals("org_id").(int64)
	threads, err := s.db.GetThreadsByOrg(orgID, 100)
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
	return c.JSON(fiber.Map{"threads": out, "count": len(out)})
}

func (s *Server) autoflowGetMessages(c *fiber.Ctx) error {
	threadID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}
	_ = s.db.ClearThreadUnread(threadID)
	msgs, err := s.db.GetThreadMessages(threadID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"messages": msgs})
}

func (s *Server) autoflowSendMessage(c *fiber.Ctx) error {
	threadID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}
	var body struct {
		Content string `json:"content"`
	}
	if err := c.BodyParser(&body); err != nil || body.Content == "" {
		return c.Status(400).JSON(fiber.Map{"error": "content required"})
	}
	if err := s.db.AddThreadMessage(threadID, "outbound", body.Content, false); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(201).JSON(fiber.Map{
		"direction":  "outbound",
		"content":    body.Content,
		"created_at": time.Now(),
	})
}

// ── Facebook Session Summary ──────────────────────────────────────────────

func (s *Server) autoflowFacebookStatus(c *fiber.Ctx) error {
	orgID := c.Locals("org_id").(int64)
	summary := s.db.GetFacebookStatusForOrg(orgID)
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

func (s *Server) getBusinessContext(c *fiber.Ctx) error {
	orgID := c.Locals("org_id").(int64)
	profile, _ := s.db.GetContext(orgContextKey(orgID, "business_profile"))
	files, _ := s.db.GetContext(orgContextKey(orgID, "private_files_summary"))
	sources, _ := s.db.GetContext(orgContextKey(orgID, "data_sources_summary"))
	return c.JSON(fiber.Map{
		"business_profile": profile,
		"private_files":    files,
		"data_sources":     sources,
	})
}

func (s *Server) updateBusinessContext(c *fiber.Ctx) error {
	orgID := c.Locals("org_id").(int64)
	var body struct {
		BusinessProfile string `json:"business_profile"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid body"})
	}
	profile := strings.TrimSpace(body.BusinessProfile)
	if err := s.db.SetContext(orgContextKey(orgID, "business_profile"), profile); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"ok": true, "business_profile": profile})
}

func (s *Server) billingSummary(c *fiber.Ctx) error {
	orgID := c.Locals("org_id").(int64)
	org, err := s.db.GetOrganization(orgID)
	if err != nil || org == nil {
		return c.Status(404).JSON(fiber.Map{"error": "organization not found"})
	}
	accountCount, _ := s.db.CountAccountsByOrg(orgID)
	staff, _ := s.db.GetStaffWithKPI(orgID)
	fb := s.db.GetFacebookStatusForOrg(orgID)
	outboxCounts, _ := s.db.CountOutboundByStatusForOrg(orgID)
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
