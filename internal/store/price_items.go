// Domain: app (see internal/store/DOMAINS.md)
package store

import (
	"fmt"
	"strings"

	"github.com/thg/scraper/internal/models"
)

// InsertPriceItems saves multiple price items.
func (s *Store) InsertPriceItems(items []models.PriceItem, source string) (int, error) {
	saved := 0
	for _, item := range items {
		if item.ServiceName == "" || item.Price == "" {
			continue
		}
		_, err := s.db.Exec(
			`INSERT INTO price_items (service_name, price, unit, notes, source) VALUES (?, ?, ?, ?, ?)`,
			item.ServiceName, item.Price, item.Unit, item.Notes, source,
		)
		if err == nil {
			saved++
		}
	}
	return saved, nil
}

// GetAllPriceItems returns all stored price items.
func (s *Store) GetAllPriceItems() ([]models.PriceItem, error) {
	rows, err := s.db.Query(
		`SELECT id, service_name, price, unit, notes, source, created_at FROM price_items ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []models.PriceItem
	for rows.Next() {
		var p models.PriceItem
		if err := rows.Scan(&p.ID, &p.ServiceName, &p.Price, &p.Unit, &p.Notes, &p.Source, &p.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, p)
	}
	return items, nil
}

// ClearPriceItems deletes all stored price items.
func (s *Store) ClearPriceItems() error {
	_, err := s.db.Exec(`DELETE FROM price_items`)
	return err
}

// GetPriceListText returns a compact price list for AI prompt context.
func (s *Store) GetPriceListText() string {
	items, err := s.GetAllPriceItems()
	if err != nil || len(items) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("BANG GIA DICH VU:\n")
	for _, item := range items {
		line := fmt.Sprintf("- %s: %s", item.ServiceName, item.Price)
		if item.Unit != "" {
			line += "/" + item.Unit
		}
		if item.Notes != "" {
			line += " (" + item.Notes + ")"
		}
		sb.WriteString(line + "\n")
	}
	return sb.String()
}
