package main

import (
	"encoding/json"
	"testing"

	"github.com/thg/scraper/internal/jobs"
)

func TestConnectorCrawlEnvelopeIncludesLegacyFlatTaskFields(t *testing.T) {
	const sourceURL = "https://www.facebook.com/groups/1312868109620530"
	task := &jobs.Task{
		TaskID:    "open-crawl-test",
		Intent:    "scrape_group",
		Keywords:  []string{"pod", "dropship"},
		CrawlPlan: jobs.CrawlPlan{Sources: []jobs.Source{{Type: "group", URL: sourceURL}}, MaxItems: 50},
		Filters:   jobs.Filters{Keywords: []string{"pod", "dropship"}},
	}

	env, err := connectorCrawlEnvelopeForTask(task)
	if err != nil {
		t.Fatalf("connectorCrawlEnvelopeForTask returned error: %v", err)
	}
	if env.NavigateTo != sourceURL {
		t.Fatalf("navigate_to = %q, want %q", env.NavigateTo, sourceURL)
	}
	if len(env.CrawlPlan.Sources) != 1 || env.CrawlPlan.Sources[0].URL != sourceURL {
		t.Fatalf("legacy crawl_plan source missing: %#v", env.CrawlPlan.Sources)
	}

	raw, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	if decoded["navigate_to"] != sourceURL {
		t.Fatalf("json navigate_to = %#v", decoded["navigate_to"])
	}
	crawlPlan, ok := decoded["crawl_plan"].(map[string]any)
	if !ok {
		t.Fatalf("json crawl_plan missing or wrong type: %#v", decoded["crawl_plan"])
	}
	sources, ok := crawlPlan["sources"].([]any)
	if !ok || len(sources) != 1 {
		t.Fatalf("json crawl_plan.sources missing: %#v", crawlPlan["sources"])
	}
	source, ok := sources[0].(map[string]any)
	if !ok || source["url"] != sourceURL {
		t.Fatalf("json legacy source url = %#v", sources[0])
	}
	if _, ok := decoded["task"].(map[string]any); !ok {
		t.Fatalf("json task missing or wrong type: %#v", decoded["task"])
	}
}
