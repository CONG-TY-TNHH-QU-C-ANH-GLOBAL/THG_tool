package crawlrun

import "context"

// OrgPool is one org's distinct campaign-pool account set across its active
// campaigns — the candidate accounts the PR-M4 orchestrator may try to claim a
// run for. Account selection (safety, readiness, budget, fairness) is the
// orchestrator's job; this read only reports pool membership.
type OrgPool struct {
	OrgID      int64
	AccountIDs []int64
}

// activePoolsQuery lists every (org, account) pair belonging to an active
// campaign's pool. Ordered so a row scan groups deterministically by org.
const activePoolsQuery = `
SELECT ca.org_id, ca.account_id
FROM facebook_crawl_campaign_accounts ca
JOIN facebook_crawl_campaigns c
  ON c.org_id = ca.org_id AND c.id = ca.campaign_id
WHERE c.status = 'active'
GROUP BY ca.org_id, ca.account_id
ORDER BY ca.org_id, ca.account_id`

// ActiveCampaignPools returns, per org, the distinct pool accounts of its active
// campaigns. It is a platform-global scheduler read (every org with active
// campaigns), so it is deliberately not org-scoped; each returned OrgPool.OrgID
// scopes the downstream enqueue/claim/dispatch. An org with active campaigns but
// no due work yields an idempotent no-op downstream, so this read never filters
// on due-ness.
func (s *Store) ActiveCampaignPools(ctx context.Context) ([]OrgPool, error) {
	if err := s.requirePostgres(); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, activePoolsQuery)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var pools []OrgPool
	for rows.Next() {
		var org, account int64
		if err := rows.Scan(&org, &account); err != nil {
			return nil, err
		}
		if n := len(pools); n > 0 && pools[n-1].OrgID == org {
			pools[n-1].AccountIDs = append(pools[n-1].AccountIDs, account)
			continue
		}
		pools = append(pools, OrgPool{OrgID: org, AccountIDs: []int64{account}})
	}
	return pools, rows.Err()
}
