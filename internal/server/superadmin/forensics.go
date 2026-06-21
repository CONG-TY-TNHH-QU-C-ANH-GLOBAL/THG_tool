package superadmin

import (
	"strings"

	"github.com/gofiber/fiber/v2"
)

// Comment Verification Forensics endpoint (spec: specs/COMMENT_ASYNC_REVERIFY.md, PR-1
// Part A). Founder-only operational diagnostic: "for these target URLs, what does the
// verification evidence actually say?". Read-only; the classification lives in
// models.ClassifyCommentForensics. Registered under the superadmin group.

// superAdminCommentForensics handles GET/POST /api/superadmin/comment-forensics.
// URLs come from the `urls` query param (comma/newline separated) or a JSON body
// {"urls": [...], "org_id": N}. Founder may target any org via org_id.
func (h *Handler) superAdminCommentForensics(c *fiber.Ctx) error {
	var body struct {
		OrgID int64    `json:"org_id"`
		URLs  []string `json:"urls"`
	}
	_ = c.BodyParser(&body)

	orgID := int64(c.QueryInt("org_id", 0))
	if orgID <= 0 {
		orgID = body.OrgID
	}
	if orgID <= 0 {
		if v, ok := c.Locals("org_id").(int64); ok {
			orgID = v
		}
	}
	if orgID <= 0 {
		return c.Status(400).JSON(fiber.Map{"error": "org_id required"})
	}

	urls := splitForensicsURLs(c.Query("urls", ""))
	urls = append(urls, body.URLs...)
	urls = dedupeNonEmpty(urls)
	if len(urls) == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "provide urls (query comma/newline separated, or JSON body)"})
	}

	rows, err := h.deps.DB.Coordination().CommentForensicsByTargetURLs(c.Context(), orgID, urls)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"org_id": orgID, "count": len(rows), "rows": rows})
}

func splitForensicsURLs(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	return strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r' || r == ' ' || r == '\t'
	})
}

func dedupeNonEmpty(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, u := range in {
		u = strings.TrimSpace(u)
		if u == "" || seen[u] {
			continue
		}
		seen[u] = true
		out = append(out, u)
	}
	return out
}
