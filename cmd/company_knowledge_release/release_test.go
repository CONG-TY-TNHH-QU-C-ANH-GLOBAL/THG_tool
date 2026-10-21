package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thg/scraper/internal/store"
	"github.com/thg/scraper/internal/store/knowledge"
	"github.com/thg/scraper/internal/workspace_knowledge/assets"
)

const manifestPath = "../../docs/business/company-knowledge/release-review-2026-10-21.json"

func releaseFixture(t *testing.T) (*knowledge.Store, int64, []releaseRecord) {
	t.Helper()
	top, err := store.New(filepath.Join(t.TempDir(), "release.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { top.Close() })
	s := top.Knowledge()
	// Seed a synthetic org, never production customer data.
	res, err := s.DB().Exec("INSERT INTO organizations(name) VALUES ('THGFulfill')")
	if err != nil {
		t.Fatal(err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	records, err := loadRelease(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	return s, id, records
}

func TestReleaseScopeIdempotenceAndRetrieval(t *testing.T) {
	s, org, records := releaseFixture(t)
	ctx := context.Background()
	for _, op := range []string{"apply", "apply", "check"} {
		status, err := release(ctx, s, org, records, op)
		if err != nil || status != "loaded_and_retrievable" {
			t.Fatalf("%s: %s %v", op, status, err)
		}
	}
	all, err := s.ListAssetsForOrg(ctx, org, assets.ListFilter{})
	if err != nil || len(all) != 8 {
		t.Fatalf("expected exactly eight assets: %d %v", len(all), err)
	}
	foreign, err := s.ListAssetsForOrg(ctx, org+1000, assets.ListFilter{})
	if err != nil || len(foreign) != 0 {
		t.Fatal("release leaked to foreign org")
	}
	audit, err := s.ListFeedbackForOrg(ctx, org, 20)
	if err != nil || len(audit) != 8 || !strings.Contains(string(audit[0].Data), approvalReference) {
		t.Fatal("approval audit missing or duplicated")
	}
}

func TestReleaseWithdrawal(t *testing.T) {
	s, org, records := releaseFixture(t)
	ctx := context.Background()
	if _, err := release(ctx, s, org, records, "apply"); err != nil {
		t.Fatal(err)
	}
	for _, op := range []string{"withdraw", "withdraw"} {
		if _, err := release(ctx, s, org, records, op); err != nil {
			t.Fatal(err)
		}
	}
	visible, err := s.ListAssetsForOrg(ctx, org, assets.ListFilter{States: []assets.AssetState{assets.StateApproved}, ExcludeUnhealthySources: true})
	if err != nil || len(visible) != 0 {
		t.Fatal("withdrawn facts still visible")
	}
	if _, err := release(ctx, s, org, records, "apply"); err == nil {
		t.Fatal("withdrawn release silently re-approved")
	}
}

func TestReleaseRefusesChangedApprovedAsset(t *testing.T) {
	s, org, records := releaseFixture(t)
	ctx := context.Background()
	if _, err := release(ctx, s, org, records, "apply"); err != nil {
		t.Fatal(err)
	}
	all, _ := s.ListAssetsForOrg(ctx, org, assets.ListFilter{})
	if err := s.SetAssetState(ctx, all[0].ID, org, assets.StateHidden); err != nil {
		t.Fatal(err)
	}
	if _, err := release(ctx, s, org, records, "apply"); err == nil {
		t.Fatal("operator hide overwritten")
	}
	a, _ := s.GetAsset(ctx, all[0].ID, org)
	if a.State != assets.StateHidden {
		t.Fatal("hidden state changed")
	}
}

func TestReleaseRequiresFrozenManifest(t *testing.T) {
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "review.json")
	changed := strings.Replace(string(raw), "public_reference", "internal_only", 1)
	if err := os.WriteFile(path, []byte(changed), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadRelease(path); err == nil {
		t.Fatal("modified manifest accepted")
	}
}

func TestExistingDatabaseAndUnambiguousOrgRequired(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.db")
	if db, err := openExistingDB(path, false); err == nil {
		db.Close()
		t.Fatal("missing database accepted")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("missing database created")
	}
	s, org, _ := releaseFixture(t)
	if got, err := resolveOrg(context.Background(), s.DB()); err != nil || got != org {
		t.Fatalf("org lookup failed: %d %v", got, err)
	}
	if _, err := s.DB().Exec("INSERT INTO organizations(name) VALUES ('THG Fulfill')"); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveOrg(context.Background(), s.DB()); err == nil {
		t.Fatal("ambiguous workspace accepted")
	}
}
