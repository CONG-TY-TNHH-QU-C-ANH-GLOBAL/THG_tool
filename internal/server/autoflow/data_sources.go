package autoflow

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/thg/scraper/internal/store"
)

const maxConnectorFetchBytes = 2 * 1024 * 1024

var googleSheetIDRe = regexp.MustCompile(`/spreadsheets/d/([a-zA-Z0-9-_]+)`)

func (h *Handler) listDataSources(c *fiber.Ctx) error {
	orgID := c.Locals("org_id").(int64)
	sources, err := h.deps.DB.ListDataSources(orgID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"sources": sources, "count": len(sources)})
}

func (h *Handler) createDataSource(c *fiber.Ctx) error {
	orgID := c.Locals("org_id").(int64)
	var req struct {
		Type      string `json:"type"`
		Name      string `json:"name"`
		SourceURL string `json:"source_url"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}
	req.Type = strings.TrimSpace(strings.ToLower(req.Type))
	req.Name = strings.TrimSpace(req.Name)
	req.SourceURL = strings.TrimSpace(req.SourceURL)
	if req.Type != "google_sheet" && req.Type != "google_drive" {
		return c.Status(400).JSON(fiber.Map{"error": "type must be google_sheet or google_drive"})
	}
	if req.SourceURL == "" || !isAllowedGoogleDataURL(req.SourceURL) {
		return c.Status(400).JSON(fiber.Map{"error": "source_url must be a Google Sheets or Drive URL"})
	}
	if req.Name == "" {
		req.Name = defaultDataSourceName(req.Type)
	}
	status := "pending"
	if req.Type == "google_drive" {
		status = "needs_auth"
	}
	id, err := h.deps.DB.CreateDataSource(&store.DataSource{
		OrgID:        orgID,
		Type:         req.Type,
		Name:         req.Name,
		SourceURL:    req.SourceURL,
		Status:       status,
		MetadataJSON: "{}",
	})
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	src, _ := h.deps.DB.GetDataSourceForOrg(orgID, id)
	return c.Status(201).JSON(src)
}

func (h *Handler) syncDataSource(c *fiber.Ctx) error {
	orgID := c.Locals("org_id").(int64)
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}
	src, err := h.deps.DB.GetDataSourceForOrg(orgID, id)
	if err != nil || src == nil {
		return c.Status(404).JSON(fiber.Map{"error": "data source not found"})
	}

	switch src.Type {
	case "google_sheet":
		if err := h.syncGoogleSheetSource(c.UserContext(), orgID, src); err != nil {
			_ = h.deps.DB.UpdateDataSourceSyncResult(orgID, src.ID, "error", src.ItemCount, src.Summary, src.MetadataJSON, err.Error(), false)
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
	case "google_drive":
		msg := "Google Drive private media sync requires read-only OAuth; source is saved and waiting for OAuth connection"
		meta := `{"requires_oauth":true,"scope":"drive.readonly"}`
		if err := h.deps.DB.UpdateDataSourceSyncResult(orgID, src.ID, "needs_auth", 0, "", meta, msg, false); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
	default:
		return c.Status(400).JSON(fiber.Map{"error": "unsupported data source type"})
	}

	_ = h.refreshDataSourcesContext(orgID)
	updated, err := h.deps.DB.GetDataSourceForOrg(orgID, id)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(updated)
}

func (h *Handler) deleteDataSource(c *fiber.Ctx) error {
	orgID := c.Locals("org_id").(int64)
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}
	if err := h.deps.DB.DeleteDataSourceForOrg(orgID, id); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	_ = h.refreshDataSourcesContext(orgID)
	return c.JSON(fiber.Map{"status": "deleted"})
}

func (h *Handler) syncGoogleSheetSource(ctx context.Context, orgID int64, src *store.DataSource) error {
	csvURL, err := googleSheetCSVURL(src.SourceURL)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, csvURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "THG-AutoFlow/1.0")
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("fetch sheet: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("fetch sheet returned HTTP %d; publish the sheet as CSV or connect OAuth later", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxConnectorFetchBytes+1))
	if err != nil {
		return fmt.Errorf("read sheet: %w", err)
	}
	if len(body) > maxConnectorFetchBytes {
		return fmt.Errorf("sheet is too large for quick sync; limit is %d MB", maxConnectorFetchBytes/1024/1024)
	}
	reader := csv.NewReader(strings.NewReader(string(body)))
	reader.FieldsPerRecord = -1
	records, err := reader.ReadAll()
	if err != nil {
		return fmt.Errorf("parse csv: %w", err)
	}
	summary, headers, itemCount := summarizeSheetRecords(src.Name, records)
	metaBytes, _ := json.Marshal(fiber.Map{
		"csv_url": csvURL,
		"headers": headers,
	})
	if err := h.deps.DB.UpdateDataSourceSyncResult(orgID, src.ID, "synced", itemCount, summary, string(metaBytes), "", true); err != nil {
		return err
	}
	return nil
}

func summarizeSheetRecords(name string, records [][]string) (string, []string, int) {
	if len(records) == 0 {
		return fmt.Sprintf("%s: empty sheet", name), nil, 0
	}
	headers := trimCells(records[0])
	rows := records[1:]
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Google Sheet: %s\n", name))
	if len(headers) > 0 {
		b.WriteString("Columns: " + strings.Join(headers, ", ") + "\n")
	}
	limit := len(rows)
	if limit > 12 {
		limit = 12
	}
	for i := 0; i < limit; i++ {
		cells := trimCells(rows[i])
		if len(cells) == 0 {
			continue
		}
		b.WriteString(fmt.Sprintf("- Row %d: %s\n", i+1, strings.Join(cells, " | ")))
	}
	return strings.TrimSpace(b.String()), headers, len(rows)
}

func trimCells(in []string) []string {
	out := make([]string, 0, len(in))
	for _, cell := range in {
		v := strings.TrimSpace(cell)
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}

func (h *Handler) refreshDataSourcesContext(orgID int64) error {
	sources, err := h.deps.DB.ListDataSources(orgID)
	if err != nil {
		return err
	}
	var b strings.Builder
	for _, src := range sources {
		if strings.TrimSpace(src.Summary) == "" {
			continue
		}
		b.WriteString(fmt.Sprintf("## %s (%s)\n%s\n\n", src.Name, src.Type, src.Summary))
	}
	return h.deps.DB.Leads().SetContext(orgContextKey(orgID, "data_sources_summary"), strings.TrimSpace(b.String()))
}

func googleSheetCSVURL(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("invalid Google Sheet URL")
	}
	if strings.Contains(u.RawQuery, "output=csv") || strings.Contains(u.RawQuery, "tqx=out:csv") {
		return raw, nil
	}
	m := googleSheetIDRe.FindStringSubmatch(u.Path)
	if len(m) < 2 {
		return "", fmt.Errorf("could not find spreadsheet id in URL")
	}
	gid := u.Query().Get("gid")
	if gid == "" && strings.Contains(u.Fragment, "gid=") {
		for _, part := range strings.Split(u.Fragment, "&") {
			if strings.HasPrefix(part, "gid=") {
				gid = strings.TrimPrefix(part, "gid=")
				break
			}
		}
	}
	out := fmt.Sprintf("https://docs.google.com/spreadsheets/d/%s/export?format=csv", m[1])
	if gid != "" {
		out += "&gid=" + url.QueryEscape(gid)
	}
	return out, nil
}

func isAllowedGoogleDataURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return host == "docs.google.com" || host == "drive.google.com" || strings.HasSuffix(host, ".google.com")
}

func defaultDataSourceName(sourceType string) string {
	switch sourceType {
	case "google_sheet":
		return "Google Sheet"
	case "google_drive":
		return "Google Drive"
	default:
		return "Data Source"
	}
}
