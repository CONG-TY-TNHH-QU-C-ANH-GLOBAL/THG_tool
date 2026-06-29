package leadoutreach

import (
	"context"
	"errors"
	"testing"

	"github.com/thg/scraper/internal/models"
)

// fakeCoverage is a store-free LeadCoverageReader stub. The seam (ARCHCM2c Seam 2)
// is what makes coverageGate's store-read behavior testable without a real store.
type fakeCoverage struct {
	state   *models.LeadCoverageState
	err     error
	calls   int
	gotOrg  int64
	gotLead int64
	gotSite string
}

func (f *fakeCoverage) GetLeadCoverageState(_ context.Context, orgID, leadID int64, website string) (*models.LeadCoverageState, error) {
	f.calls++
	f.gotOrg, f.gotLead, f.gotSite = orgID, leadID, website
	return f.state, f.err
}

func newCoverageCtx(cov LeadCoverageReader) *Context {
	return &Context{
		coverage:        cov,
		orgID:           7,
		accountID:       3,
		msgType:         "comment",
		commentIdentity: models.CompanyIdentity{Website: "shop.example"},
		coveragePolicy:  models.DefaultCoveragePolicy(),
	}
}

// TestCoverageGate_NonCommentShortCircuits: a non-comment msgType proceeds without
// touching the coverage reader at all (preserves the original guard).
func TestCoverageGate_NonCommentShortCircuits(t *testing.T) {
	cov := &fakeCoverage{}
	c := newCoverageCtx(cov)
	c.msgType = "inbox"

	_, skip := c.coverageGate(context.Background(), models.Lead{ID: 1})
	if skip != "" {
		t.Fatalf("skip=%q, want empty (proceed)", skip)
	}
	if cov.calls != 0 {
		t.Fatalf("reader called %d times, want 0 for non-comment", cov.calls)
	}
}

// TestCoverageGate_ReadErrorIsNonFatal: a coverage lookup error proceeds with a zero
// persona and no skip (the original error-tolerant behavior), and does not deref nil.
func TestCoverageGate_ReadErrorIsNonFatal(t *testing.T) {
	cov := &fakeCoverage{err: errors.New("boom")}
	c := newCoverageCtx(cov)

	_, skip := c.coverageGate(context.Background(), models.Lead{ID: 5})
	if skip != "" {
		t.Fatalf("skip=%q, want empty (error is non-fatal)", skip)
	}
	if cov.calls != 1 {
		t.Fatalf("reader called %d times, want 1", cov.calls)
	}
}

// TestCoverageGate_PassesExactArgs: the port receives the org/lead/website the gate
// resolved (orgID from context, lead.ID, commentIdentity.Website) — pins the seam wiring.
func TestCoverageGate_PassesExactArgs(t *testing.T) {
	cov := &fakeCoverage{err: errors.New("stop-before-eval")}
	c := newCoverageCtx(cov)

	c.coverageGate(context.Background(), models.Lead{ID: 42})
	if cov.gotOrg != 7 || cov.gotLead != 42 || cov.gotSite != "shop.example" {
		t.Fatalf("reader got org=%d lead=%d site=%q, want 7/42/shop.example", cov.gotOrg, cov.gotLead, cov.gotSite)
	}
}
