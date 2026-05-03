package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/thg/scraper/internal/models"
)

func TestValidateBrainPlanRejectsUnknownTool(t *testing.T) {
	plan := &BrainPlanResponse{
		DomainScope: "facebook",
		Decision:    "execute",
		Confidence:  0.9,
		Actions: []BrainAction{{
			Tool: "delete_everything",
			Args: map[string]any{},
		}},
	}
	if err := validateBrainPlan(plan); err == nil {
		t.Fatal("expected unknown tool to be rejected")
	}
}

func TestValidateBrainPlanRejectsMissingConcreteSource(t *testing.T) {
	plan := &BrainPlanResponse{
		DomainScope: "facebook",
		Decision:    "execute",
		Confidence:  0.9,
		Actions: []BrainAction{{
			Tool: "scrape_group",
			Args: map[string]any{"url": ""},
		}},
	}
	if err := validateBrainPlan(plan); err == nil {
		t.Fatal("expected missing crawl source to be rejected")
	}
}

func TestPrepareBrainActionArgsInjectsTrustedTenantAndAccount(t *testing.T) {
	action := BrainAction{
		Tool: "scrape_group",
		Args: map[string]any{
			"url":        "https://www.facebook.com/groups/123",
			"org_id":     999,
			"account_id": 888,
			"auto":       true,
			"max_items":  500,
		},
	}
	args := prepareBrainActionArgs(action, BrainMarketSignalGate{}, "crawl 20 posts", 7, 9)
	if got := brainInt64(args["org_id"]); got != 7 {
		t.Fatalf("org_id = %d, want 7", got)
	}
	if got := brainInt64(args["account_id"]); got != 9 {
		t.Fatalf("account_id = %d, want 9", got)
	}
	if _, ok := args["auto"]; ok {
		t.Fatal("non-outbound tool must not inherit untrusted auto flag")
	}
	if got := brainInt64(args["max_items"]); got != brainDefaultCap {
		t.Fatalf("max_items = %d, want cap %d", got, brainDefaultCap)
	}
}

func TestOutboundBrainArgsDefaultToDraft(t *testing.T) {
	action := BrainAction{
		Tool: "comment_all_leads",
		Args: map[string]any{"auto": true},
	}
	args := prepareBrainActionArgs(action, BrainMarketSignalGate{}, "comment cho lead hot", 7, 9)
	if got, ok := args["auto"].(bool); !ok || got {
		t.Fatalf("auto = %#v, want false unless prompt explicitly asks auto", args["auto"])
	}
}

func TestBrainClientAvailabilityAndPlan(t *testing.T) {
	if NewBrainClient("", time.Second).Available() {
		t.Fatal("empty brain url should be unavailable")
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != brainPlanPath {
			t.Fatalf("path = %s, want %s", r.URL.Path, brainPlanPath)
		}
		var req BrainPlanRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.OrgID != 12 {
			t.Fatalf("org_id = %d, want 12", req.OrgID)
		}
		_ = json.NewEncoder(w).Encode(BrainPlanResponse{
			DomainScope:     "facebook",
			Intent:          "discover_sources",
			Decision:        "ask_user",
			Confidence:      0.8,
			ResponseSummary: "need source",
		})
	}))
	defer server.Close()

	client := NewBrainClient(server.URL, time.Second)
	plan, err := client.Plan(context.Background(), BrainPlanRequest{OrgID: 12, Prompt: "find leads"})
	if err != nil {
		t.Fatalf("Plan error: %v", err)
	}
	if plan.Intent != "discover_sources" {
		t.Fatalf("intent = %q, want discover_sources", plan.Intent)
	}
}

func TestFacebookBrowserPreflightRejectsWrongOrMissingReadyAccount(t *testing.T) {
	accounts := []models.Account{{
		ID:              1,
		Platform:        models.PlatformFacebook,
		Status:          models.AccountActive,
		BrowserLoggedIn: true,
		FBUserID:        "1001",
	}}
	if ok, _ := facebookBrowserPreflight(accounts, 2); ok {
		t.Fatal("selected account outside org account list must be rejected")
	}
	accounts[0].BrowserLoggedIn = false
	if ok, _ := facebookBrowserPreflight(accounts, 0); ok {
		t.Fatal("missing logged-in browser session must be rejected")
	}
}
