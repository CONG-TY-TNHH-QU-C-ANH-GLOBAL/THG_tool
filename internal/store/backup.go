// Domain: infra (see internal/store/DOMAINS.md)
package store

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// StartAutoBackup runs a daily SQLite backup in a goroutine.
// Keeps the last 7 backups. Backups are stored in data/backups/.
func (s *Store) StartAutoBackup(dbPath string) {
	backupDir := filepath.Join(filepath.Dir(dbPath), "backups")
	if err := os.MkdirAll(backupDir, 0700); err != nil {
		log.Printf("[Backup] ⚠️ Failed to create backup dir: %v", err)
		return
	}

	// Run first backup immediately
	go func() {
		doBackup(dbPath, backupDir)

		// Then run daily at 3:00 AM
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			doBackup(dbPath, backupDir)
		}
	}()

	log.Printf("[Backup] ✅ Auto-backup enabled (daily, keep 7 days, dir: %s)", backupDir)
}

func doBackup(dbPath, backupDir string) {
	timestamp := time.Now().Format("20060102_150405")
	backupPath := filepath.Join(backupDir, fmt.Sprintf("scraper_%s.db", timestamp))

	src, err := os.Open(dbPath)
	if err != nil {
		log.Printf("[Backup] ❌ Failed to open DB: %v", err)
		return
	}
	defer src.Close()

	dst, err := os.Create(backupPath)
	if err != nil {
		log.Printf("[Backup] ❌ Failed to create backup: %v", err)
		return
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		log.Printf("[Backup] ❌ Failed to copy DB: %v", err)
		return
	}

	log.Printf("[Backup] ✅ Backup created: %s", backupPath)

	// Clean old backups — keep last 7
	cleanOldBackups(backupDir, 7)
}

func cleanOldBackups(backupDir string, keep int) {
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		return
	}

	var backups []string
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".db" {
			backups = append(backups, filepath.Join(backupDir, e.Name()))
		}
	}

	if len(backups) <= keep {
		return
	}

	sort.Strings(backups) // oldest first (timestamp in name)
	toDelete := backups[:len(backups)-keep]
	for _, f := range toDelete {
		if err := os.Remove(f); err == nil {
			log.Printf("[Backup] 🗑️ Removed old backup: %s", filepath.Base(f))
		}
	}
}
