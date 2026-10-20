package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/thg/scraper/internal/ai"
	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/platform/facebook/media"
	"github.com/thg/scraper/internal/services/facebook"
	"github.com/thg/scraper/internal/workspace_knowledge/assets"
	knowledgeRuntime "github.com/thg/scraper/internal/workspace_knowledge/runtime"
)

type pipelinePreview struct {
	Mode              string                `json:"mode"`
	Model             string                `json:"model"`
	RuntimeEnabled    bool                  `json:"runtime_enabled"`
	KnowledgeApproved bool                  `json:"knowledge_approved"`
	SourcingEnabled   bool                  `json:"live_sourcing_enabled"`
	DurationMS        int64                 `json:"pipeline_duration_ms"`
	Suggestion        models.LeadSuggestion `json:"actual_pipeline_result_unreviewed"`
}

func exitOnPreviewError(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// livePipeline uses the production suggestion facade with public facts in an
// explicitly simulated approval state. It cannot import/approve the pack, use
// a real org/database, call Elim/shipping or send Telegram/Facebook messages.
func livePipeline(ctx context.Context, dir, post, author, imageURL, envFile string, review, publicOnly bool) error {
	if !review || !publicOnly || strings.TrimSpace(post) == "" {
		return errors.New("pipeline preview requires a synthetic -query, -review-draft and -public-facts-only")
	}
	var images []string
	if imageURL != "" {
		images = media.URLs([]string{imageURL})
		if len(images) != 1 {
			return errors.New("pipeline preview requires an authorized original HTTPS Facebook CDN photo")
		}
	}
	store, err := simulatedPublicReviewStore(dir)
	if err != nil {
		return err
	}
	generator, model, err := previewModel(envFile)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	started := time.Now()
	out := facebook.BuildLeadSuggestionWithContext(ctx, knowledgeRuntime.NewBuilder(store), generator,
		&ai.BusinessProfile{Name: "THG"}, localOrgID, post, author, images, nil, nil)
	return json.NewEncoder(os.Stdout).Encode(pipelinePreview{
		Mode: "live_pipeline_simulated_public_approval", Model: model, Suggestion: out, DurationMS: time.Since(started).Milliseconds(),
	})
}

func simulatedPublicReviewStore(dir string) (*memoryDraft, error) {
	store, _, _, err := loadDraft(dir, true)
	if err != nil {
		return nil, err
	}
	simulation := &memoryDraft{}
	for _, item := range store.items {
		if _, ok := assets.ReplyKnowledgeFacts(item.Payload); !ok {
			continue
		}
		copy := *item
		copy.State = assets.StateApproved
		simulation.items = append(simulation.items, &copy)
	}
	return simulation, nil
}
