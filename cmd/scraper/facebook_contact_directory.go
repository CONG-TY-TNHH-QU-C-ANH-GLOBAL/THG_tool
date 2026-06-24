package main

import (
	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/services/facebook"
	"github.com/thg/scraper/internal/store"
)

// fbContactDirectory is the composition-root adapter that satisfies the consumer-owned
// facebook.ContactDirectory port from the real *store.Store. It lives here (the wiring
// boundary) so services/facebook stays free of internal/store. Thin pass-throughs only —
// no logic, no behavior change.
type fbContactDirectory struct{ db *store.Store }

func (d fbContactDirectory) StaffContactProfile(orgID, userID int64) (*models.StaffContactProfile, error) {
	return d.db.GetStaffContactProfile(orgID, userID)
}

func (d fbContactDirectory) AccountForOrg(accountID, orgID int64) (*models.Account, error) {
	return d.db.Identities().GetAccountForOrg(accountID, orgID)
}

func (d fbContactDirectory) LeadContext(key string) (string, error) {
	return d.db.Leads().GetContext(key)
}

// Compile-time check: the adapter satisfies the consumer-owned port.
var _ facebook.ContactDirectory = fbContactDirectory{}
