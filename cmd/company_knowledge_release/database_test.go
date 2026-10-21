package main

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/thg/scraper/internal/workspace_knowledge/assets"
	"github.com/thg/scraper/internal/workspace_knowledge/sources"
)

func TestReleaseCheckDatabaseIsReadOnly(t *testing.T) {
	s, org, _ := releaseFixture(t)
	var seq int
	var name, path string
	if err := s.DB().QueryRow("PRAGMA database_list").Scan(&seq, &name, &path); err != nil {
		t.Fatal(err)
	}
	db, err := openExistingDB(filepath.Clean(path), true)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if id, err := resolveOrg(context.Background(), db); err != nil || id != org {
		t.Fatal("read-only connection failed org lookup")
	}
	if _, err := db.Exec("INSERT INTO organizations(name) VALUES ('must not write')"); err == nil {
		t.Fatal("check connection permitted a write")
	}
}

func TestReleasePreservesOtherSourcesAndPendingAssets(t *testing.T) {
	s, org, records := releaseFixture(t)
	ctx := context.Background()
	src, err := s.UpsertSource(ctx, &sources.Source{OrgID: org, Type: sources.SourceCSV, Label: "unrelated source"})
	if err != nil {
		t.Fatal(err)
	}
	sentinel, err := s.UpsertAsset(ctx, &assets.Asset{OrgID: org, SourceID: src.ID, ExternalID: "do-not-approve",
		Type: assets.AssetFAQ, Title: "Private pending draft", State: assets.StatePending})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := release(ctx, s, org, records, "apply"); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetAsset(ctx, sentinel.ID, org)
	if err != nil || got.State != assets.StatePending || got.Title != sentinel.Title {
		t.Fatal("unrelated pending asset changed")
	}
	if _, err := release(ctx, s, org, records, "withdraw"); err != nil {
		t.Fatal(err)
	}
	got, err = s.GetAsset(ctx, sentinel.ID, org)
	if err != nil || got.State != assets.StatePending {
		t.Fatal("withdrawal changed unrelated asset")
	}
}
