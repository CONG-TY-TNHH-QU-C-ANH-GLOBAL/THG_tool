package facebook

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/thg/scraper/internal/ai"
	"github.com/thg/scraper/internal/textutil"
)

// FacebookPostContentInput is the parsed, transport-free post-content request.
// The composition root extracts these from the raw action args so this service
// never sees the untyped map[string]any.
type FacebookPostContentInput struct {
	Content      string
	Description  string
	Title        string
	Requirements string
	Benefits     string
	Salary       string
	Email        string
}

// ResolveFacebookPostContent builds the post body (behavior moved verbatim from
// cmd/scraper): explicit content/description/title, then an AI-generated job post
// when a title + an available generator are present. Returns an error when no
// content can be resolved (message preserved). msgGen is the ai dependency
// services/facebook already relies on — not a store/cmd coupling.
func ResolveFacebookPostContent(ctx context.Context, msgGen *ai.MessageGenerator, in FacebookPostContentInput) (string, error) {
	content := textutil.FirstNonEmpty(in.Content, in.Description, in.Title)
	if msgGen != nil && msgGen.Available() && in.Title != "" {
		genCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
		generated, err := msgGen.GenerateJobPost(genCtx,
			in.Title,
			in.Description,
			in.Requirements,
			in.Benefits,
			in.Salary,
			in.Email,
		)
		cancel()
		if err == nil && strings.TrimSpace(generated) != "" {
			content = generated
		}
	}
	if strings.TrimSpace(content) == "" {
		return "", fmt.Errorf("Facebook post content is required")
	}
	return content, nil
}
