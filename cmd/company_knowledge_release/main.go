package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/thg/scraper/internal/store/dbutil"
	"github.com/thg/scraper/internal/store/knowledge"
	_ "modernc.org/sqlite"
)

func main() {
	dbPath := flag.String("db", "", "Existing local SQLite runtime database")
	manifest := flag.String("manifest", "docs/business/company-knowledge/release-review-2026-10-21.json", "Frozen owner-reviewed manifest")
	operation := flag.String("operation", "check", "check, apply or withdraw")
	flag.Parse()
	if err := run(*dbPath, *manifest, *operation); err != nil {
		// Never serialize SQL/HTTP errors: they can contain private data.
		fmt.Fprintln(os.Stderr, "company_knowledge_release=failed; no raw error or database content logged")
		os.Exit(1)
	}
}

func run(dbPath, manifest, operation string) error {
	if operation != "check" && operation != "apply" && operation != "withdraw" {
		return errors.New("invalid operation")
	}
	records, err := loadRelease(manifest)
	if err != nil {
		return err
	}
	db, err := openExistingDB(dbPath, operation == "check")
	if err != nil {
		return err
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	orgID, err := resolveOrg(ctx, db)
	if err != nil {
		return err
	}
	s := knowledge.NewStore(db, dbutil.NewSQLiteDialect())
	status, err := release(ctx, s, orgID, records, operation)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{
		"operation": operation, "status": status, "review_sha256": releaseHash,
		"approved_scope": 8, "messages_sent": 0,
	})
}

func openExistingDB(path string, readOnly bool) (*sql.DB, error) {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return nil, errors.New("existing regular database file required")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	mode := "rw"
	if readOnly {
		mode = "ro"
	}
	uriPath := filepath.ToSlash(abs)
	if !strings.HasPrefix(uriPath, "/") {
		uriPath = "/" + uriPath // Windows drive is a path, not a URI authority.
	}
	u := url.URL{Scheme: "file", Path: uriPath}
	q := url.Values{"mode": {mode}, "_pragma": {"foreign_keys(1)", "busy_timeout(5000)"}}
	u.RawQuery = q.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err == nil {
		db.SetMaxOpenConns(1)
		err = db.Ping()
	}
	if err != nil && db != nil {
		db.Close()
	}
	return db, err
}

func resolveOrg(ctx context.Context, db *sql.DB) (int64, error) {
	// Read-only identity lookup; all knowledge mutations stay in their owner.
	var count int
	var id sql.NullInt64
	err := db.QueryRowContext(ctx, `SELECT COUNT(*), MIN(id) FROM organizations
		WHERE lower(replace(name, ' ', '')) = 'thgfulfill'`).Scan(&count, &id)
	if err != nil {
		return 0, err
	}
	if count != 1 || !id.Valid || id.Int64 <= 0 {
		return 0, errors.New("exactly one THGFulfill workspace required")
	}
	return id.Int64, nil
}
