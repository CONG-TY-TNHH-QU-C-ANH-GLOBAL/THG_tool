package autoflow

import (
	"os"
	"strings"
	"sync"

	"github.com/gofiber/fiber/v2"
	"github.com/thg/scraper/internal/ai"
)

// inferrerOnce guards lazy construction of the BusinessProfileInferrer.
// We instantiate it on the first request rather than at router-build time
// because the OpenAI key is read from env at runtime and the Inferrer is
// a stateless HTTP client — no benefit to long-lived singleton wiring,
// no downside to lazy init.
var (
	inferrerOnce sync.Once
	inferrer     *ai.BusinessProfileInferrer
)

func getInferrer() *ai.BusinessProfileInferrer {
	inferrerOnce.Do(func() {
		inferrer = ai.NewBusinessProfileInferrer(os.Getenv("OPENAI_API_KEY"), os.Getenv("OPENAI_MODEL"))
	})
	return inferrer
}

// inferBusinessContext powers the "Magic Omnibox" on the Knowledge Base
// page. The user pastes a catalog URL and/or a 1-line description; we
// fetch the URL, run an LLM extractor, and return a proposed profile
// with per-field confidence. We do NOT save — the FE shows the proposal
// inline and only persists once the user clicks Save in the existing
// updateBusinessContext flow.
func (h *Handler) inferBusinessContext(c *fiber.Ctx) error {
	if _, ok := c.Locals("org_id").(int64); !ok {
		return c.Status(401).JSON(fiber.Map{"error": "org context required"})
	}
	var body struct {
		SourceURL string `json:"source_url"`
		Note      string `json:"note"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}
	body.SourceURL = strings.TrimSpace(body.SourceURL)
	body.Note = strings.TrimSpace(body.Note)
	if body.SourceURL == "" && body.Note == "" {
		return c.Status(400).JSON(fiber.Map{"error": "cần dán URL website/catalog hoặc nhập 1 câu mô tả doanh nghiệp"})
	}

	inf := getInferrer()
	if !inf.Available() {
		return c.Status(503).JSON(fiber.Map{
			"error": "AI inference chưa được cấu hình (thiếu OPENAI_API_KEY).",
		})
	}

	result, err := inf.Infer(c.Context(), ai.InferenceInput{
		URL:  body.SourceURL,
		Note: body.Note,
	})
	if err != nil {
		return c.Status(502).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(result)
}
