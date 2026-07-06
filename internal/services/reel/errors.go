package reel

import (
	"database/sql"
	"errors"
)

// Domain errors. Callers of this service should never need to know about
// database/sql — GetReel/GetLatestScript's sql.ErrNoRows is translated to
// one of these at the service boundary via notFoundAs.
var (
	ErrReelNotFound      = errors.New("reel: not found")
	ErrNoScript          = errors.New("reel: no script exists")
	ErrScriptNotApproved = errors.New("reel: script must be approved before rendering")
)

// notFoundAs translates a store sql.ErrNoRows into domainErr; any other
// error passes through unchanged. Shared by every method that reads a reel
// or script before acting on it.
func notFoundAs(err, domainErr error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return domainErr
	}
	return err
}

// Reel lifecycle states. Matches the comment on reel.Reel.Status
// (internal/store/reel/models.go): draft|scripting|approved|rendering|
// done|failed. "rendering" is not used yet — RenderFake is synchronous, so
// there is no in-flight state to persist until an async provider lands.
const (
	StatusDraft     = "draft"
	StatusScripting = "scripting"
	StatusApproved  = "approved"
	StatusDone      = "done"
	StatusFailed    = "failed"
)
