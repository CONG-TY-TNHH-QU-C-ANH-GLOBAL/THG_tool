// Domain: coordination (see internal/store/DOMAINS.md)
package coordination

import (
	"context"
	"sort"
	"time"
)

// ContributionRow is one member's derived contribution — Organic Sales Network
// Attribution Layer. It is a PROJECTION over the action_ledger (the append-only
// Interaction Event store), keyed by the IMMUTABLE created_by member, NEVER by
// the account's current owner. Rebuildable from the ledger at any time.
type ContributionRow struct {
	UserID   int64          `json:"user_id"`
	UserName string         `json:"user_name"`
	Total    int            `json:"total"`
	ByType   map[string]int `json:"by_type"` // InteractionType -> verified count
}

// ContributionLeaderboard derives the per-member contribution leaderboard from
// verified-success interactions in the action_ledger, most-contributions first.
// created_by=0 (system/unattributed) is excluded. since=zero means all-time.
// The top row is the org "champion" — but champion is ANALYTICS ONLY: it confers
// no routing/execution priority and no lead ownership (Ownership ⊥ Champion).
func (s *Store) ContributionLeaderboard(ctx context.Context, orgID int64, since time.Time, limit int) ([]ContributionRow, error) {
	if orgID <= 0 {
		return nil, nil
	}
	if limit <= 0 {
		limit = 50
	}
	q := `SELECT al.created_by, COALESCE(u.name, ''), al.action_type, COUNT(*)
	        FROM action_ledger al
	        LEFT JOIN users u ON u.id = al.created_by
	       WHERE al.org_id = ? AND al.created_by > 0 AND al.outcome = 'succeeded'`
	args := []any{orgID}
	if !since.IsZero() {
		q += ` AND al.performed_at >= ?`
		args = append(args, since.UTC().Format("2006-01-02 15:04:05"))
	}
	q += ` GROUP BY al.created_by, al.action_type`

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	byUser := make(map[int64]*ContributionRow)
	for rows.Next() {
		var (
			uid        int64
			name       string
			actionType string
			n          int
		)
		if err := rows.Scan(&uid, &name, &actionType, &n); err != nil {
			return nil, err
		}
		r := byUser[uid]
		if r == nil {
			r = &ContributionRow{UserID: uid, UserName: name, ByType: map[string]int{}}
			byUser[uid] = r
		}
		r.ByType[actionType] += n
		r.Total += n
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]ContributionRow, 0, len(byUser))
	for _, r := range byUser {
		out = append(out, *r)
	}
	// Deterministic order: total desc, then user_id asc as a stable tiebreak.
	sort.Slice(out, func(i, j int) bool {
		if out[i].Total != out[j].Total {
			return out[i].Total > out[j].Total
		}
		return out[i].UserID < out[j].UserID
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}
