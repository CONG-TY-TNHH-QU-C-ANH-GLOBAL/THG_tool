// company_knowledge_preview exercises the existing retriever against the draft
// company pack in memory. Provider calls require the explicit -live-reply flag.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/thg/scraper/internal/workspace_knowledge/assets"
	"github.com/thg/scraper/internal/workspace_knowledge/retrieval"
	"github.com/thg/scraper/internal/workspace_knowledge/retrieval/hybrid"
)

type previewHit struct {
	ID           string   `json:"id"`
	Title        string   `json:"title"`
	Score        float64  `json:"score"`
	Audience     string   `json:"audience"`
	Verification string   `json:"verification"`
	Facts        []string `json:"facts"`
	Guidance     []string `json:"proposed_guidance,omitempty"`
	Avoid        []string `json:"avoid,omitempty"`
	AppliesWhen  []string `json:"applies_when,omitempty"`
	Inputs       []string `json:"required_inputs,omitempty"`
	Steps        []string `json:"steps,omitempty"`
	Outputs      []string `json:"outputs,omitempty"`
	RelatedIDs   []string `json:"related_ids,omitempty"`
	Sources      []source `json:"sources"`
}

func main() {
	dir := flag.String("pack", "docs/business/company-knowledge", "Directory containing the draft JSON files")
	query := flag.String("query", "", "Question or lead text to search locally")
	review := flag.Bool("review-draft", false, "Explicitly inspect pending entries; does not approve them")
	publicOnly := flag.Bool("public-facts-only", false, "Limit to checked public facts and omit proposed instructions")
	top := flag.Int("top", 5, "Maximum number of results (1-20)")
	legacy := flag.Bool("legacy-lexical", false, "Use hybrid-v1 for comparison only")
	evaluate := flag.String("evaluate", "", "Path to offline retrieval cases; requires -review-draft")
	live := flag.Bool("live-reply", false, "Call the configured model once for a synthetic query; no Telegram or DB writes")
	author := flag.String("author", "", "Synthetic author name for -live-reply")
	envFile := flag.String("env-file", "", "Optional private model configuration file; values are never printed")
	liveImage := flag.Bool("live-image-context", false, "Call the configured model once to inspect text and an original Facebook image")
	imageURL := flag.String("image-url", "", "Original Facebook CDN photo reference for -live-image-context")
	pipeline := flag.Bool("live-pipeline", false, "Run need analysis and guarded reply on synthetic input/public facts; no sourcing, DB or Telegram")
	flag.Parse()
	if *pipeline {
		exitOnPreviewError(livePipeline(context.Background(), *dir, *query, *author, *imageURL, *envFile, *review, *publicOnly))
		return
	}
	if *liveImage {
		if err := inspectImage(context.Background(), *query, *imageURL, *envFile); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if *live {
		if err := liveReply(context.Background(), *dir, *query, *author, *envFile, *review, *publicOnly); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if *evaluate != "" {
		if err := evaluateRetrieval(*dir, *evaluate, *review); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if *query == "" || *top < 1 || *top > 20 {
		fmt.Fprintln(os.Stderr, "query is required; top must be between 1 and 20")
		os.Exit(2)
	}
	report, err := previewWithLexical(context.Background(), *dir, *query, *review, *publicOnly, *top, *legacy)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(report); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

type previewReport struct {
	Mode            string       `json:"mode"`
	SearcherImpl    string       `json:"searcher_impl"`
	RuntimeEnabled  bool         `json:"runtime_enabled"`
	LoadedPending   int          `json:"loaded_pending"`
	PolicyOnlyCount int          `json:"policy_only_count"`
	PublicFactsOnly bool         `json:"public_facts_only"`
	Hits            []previewHit `json:"hits"`
}

func preview(ctx context.Context, dir, query string, review, publicOnly bool, top int) (previewReport, error) {
	return previewWithLexical(ctx, dir, query, review, publicOnly, top, false)
}

func previewWithLexical(ctx context.Context, dir, query string, review, publicOnly bool, top int, legacy bool) (previewReport, error) {
	store, records, sources, err := loadDraft(dir, publicOnly)
	report := previewReport{Mode: "approved_only_local", PublicFactsOnly: publicOnly, Hits: []previewHit{}}
	if err != nil {
		return report, err
	}
	report.LoadedPending = len(store.items)
	filter := retrieval.SearchFilter{}
	if review {
		report.Mode = "pending_review_only"
		filter.States = []assets.AssetState{assets.StatePending}
	}
	for _, item := range store.items {
		if item.Type == assets.AssetBannedClaim {
			report.PolicyOnlyCount++
		}
	}
	searcher := hybrid.NewMultilingual(store)
	if legacy {
		searcher = hybrid.New(store)
	}
	hits, trace, err := searcher.TopKWithTrace(ctx, localOrgID, query, filter, top)
	report.SearcherImpl = trace.SearcherImpl
	if err != nil {
		return report, err
	}
	for _, hit := range hits {
		record := records[hit.Asset.ExternalID]
		row := previewHit{ID: record.ID, Title: record.Title, Score: hit.Score,
			Audience: record.Audience, Verification: record.Verification, Facts: record.Facts}
		if !publicOnly {
			row.Guidance, row.Avoid = record.Guidance, record.Avoid
			row.AppliesWhen, row.Inputs = record.AppliesWhen, record.Inputs
			row.Steps, row.Outputs, row.RelatedIDs = record.Steps, record.Outputs, record.RelatedIDs
		}
		for _, id := range record.SourceIDs {
			row.Sources = append(row.Sources, sources[id])
		}
		report.Hits = append(report.Hits, row)
	}
	return report, nil
}
