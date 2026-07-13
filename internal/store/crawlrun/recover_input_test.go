package crawlrun_test

import (
	"context"
	"errors"
	"testing"

	"github.com/thg/scraper/internal/store/crawlrun"
	"github.com/thg/scraper/internal/store/dbutil"
)

// Structurally invalid input is rejected before any database work, so these
// cases need no live Postgres — a postgres-dialect store over a nil handle
// exercises the pure validation branch.
func TestRecoverDispatchFailure_InvalidInputIsCallerError(t *testing.T) {
	st := crawlrun.NewStore(nil, dbutil.NewPostgresDialect())
	ctx := context.Background()

	fenceCases := []struct {
		name  string
		fence crawlrun.Fence
	}{
		{"zero org", crawlrun.Fence{OrgID: 0, RunID: 1, Attempt: 1}},
		{"zero run", crawlrun.Fence{OrgID: 1, RunID: 0, Attempt: 1}},
		{"zero attempt", crawlrun.Fence{OrgID: 1, RunID: 1, Attempt: 0}},
		{"negative run", crawlrun.Fence{OrgID: 1, RunID: -5, Attempt: 1}},
	}
	for _, tc := range fenceCases {
		t.Run("invalid fence/"+tc.name, func(t *testing.T) {
			out, err := st.RecoverDispatchFailure(ctx,
				crawlrun.RecoverDispatchFailureInput{Fence: tc.fence, ExpectedAccountID: 7})
			if !errors.Is(err, crawlrun.ErrInvalidFence) {
				t.Fatalf("err = %v, want ErrInvalidFence", err)
			}
			if out != (crawlrun.RecoverDispatchFailureOutcome{}) {
				t.Fatalf("outcome = %+v, want zero value", out)
			}
		})
	}

	t.Run("invalid account", func(t *testing.T) {
		out, err := st.RecoverDispatchFailure(ctx, crawlrun.RecoverDispatchFailureInput{
			Fence:             crawlrun.Fence{OrgID: 1, RunID: 1, Attempt: 1},
			ExpectedAccountID: 0,
		})
		if !errors.Is(err, crawlrun.ErrInvalidAccountID) {
			t.Fatalf("err = %v, want ErrInvalidAccountID", err)
		}
		if out != (crawlrun.RecoverDispatchFailureOutcome{}) {
			t.Fatalf("outcome = %+v, want zero value", out)
		}
	})
}
