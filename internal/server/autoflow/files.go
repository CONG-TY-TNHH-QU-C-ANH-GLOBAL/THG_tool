package autoflow

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

const fileUploadDir = "data/files"
const maxFileSize = 50 * 1024 * 1024 // 50 MB

var allowedMimes = map[string]bool{
	"application/pdf": true,
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document": true,
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":       true,
	"text/plain": true,
	"text/csv":   true,
}

func (h *Handler) autoflowListFiles(c *fiber.Ctx) error {
	orgID := c.Locals("org_id").(int64)
	files, err := h.deps.DB.GetPrivateFiles(orgID)
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

func (h *Handler) autoflowUploadFile(c *fiber.Ctx) error {
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
	id, err := h.deps.DB.InsertPrivateFile(rec)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	h.refreshPrivateFilesContext(orgID)
	return c.Status(201).JSON(fiber.Map{
		"id": id, "name": fh.Filename, "size_bytes": fh.Size, "mime_type": mime, "created_at": time.Now(),
	})
}

func (h *Handler) refreshPrivateFilesContext(orgID int64) {
	key := orgContextKey(orgID, "private_files_summary")
	files, err := h.deps.DB.GetPrivateFiles(orgID)
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
	_ = h.deps.DB.SetContext(key, strings.TrimSpace(b.String()))
}

func (h *Handler) autoflowDeleteFile(c *fiber.Ctx) error {
	orgID := c.Locals("org_id").(int64)
	fileID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}
	files, _ := h.deps.DB.GetPrivateFiles(orgID)
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
	if err := h.deps.DB.DeletePrivateFile(fileID, orgID); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	h.refreshPrivateFilesContext(orgID)
	return c.JSON(fiber.Map{"ok": true})
}

// Ã¢â€â‚¬Ã¢â€â‚¬ Conversation Threads Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬
