package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"

	"github.com/thg/scraper/internal/ai/visualcontext"
	"github.com/thg/scraper/internal/platform/facebook/media"
)

// inspectImage exercises perception only, using authorized test content. It
// does not source an item, approve knowledge, send Telegram or write a database.
func inspectImage(ctx context.Context, post, imageURL, envFile string) error {
	urls := media.URLs([]string{imageURL})
	if strings.TrimSpace(post) == "" || len(urls) != 1 {
		return errors.New("image preview requires -query and an original HTTPS Facebook CDN -image-url")
	}
	generator, model, err := previewModel(envFile)
	if err != nil {
		return err
	}
	observations, err := generator.AnalyzeLeadImages(ctx, post, urls)
	if err != nil {
		return errors.New("image context unavailable; no outbound message or sourcing request sent")
	}
	return json.NewEncoder(os.Stdout).Encode(struct {
		Mode    string                `json:"mode"`
		Model   string                `json:"model"`
		Context visualcontext.Context `json:"actual_model_observations_unreviewed"`
	}{"live_image_context_review_only", model, observations})
}
