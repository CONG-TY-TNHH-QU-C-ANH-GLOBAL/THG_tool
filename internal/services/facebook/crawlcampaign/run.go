package crawlcampaign

// RunStatus is the lifecycle status of a facebook_crawl_runs row (migration
// 0115). The open statuses back the one-open-run-per-source invariant.
type RunStatus string

const (
	RunQueued                     RunStatus = "queued"
	RunWaitingForConnectorUpgrade RunStatus = "waiting_for_connector_upgrade"
	RunRunning                    RunStatus = "running"
	RunSucceeded                  RunStatus = "succeeded"
	RunStoppedSafe                RunStatus = "stopped_safe"
	RunFailed                     RunStatus = "failed"
	RunAbandoned                  RunStatus = "abandoned"
	RunCancelled                  RunStatus = "cancelled"
)

// IsOpen reports whether the status occupies a source's single open-run slot,
// matching the ux_fb_crawl_runs_one_open_source partial-index predicate.
func (s RunStatus) IsOpen() bool {
	switch s {
	case RunQueued, RunWaitingForConnectorUpgrade, RunRunning:
		return true
	default:
		return false
	}
}

// IsTerminal reports whether the status is a final, immutable run outcome.
func (s RunStatus) IsTerminal() bool {
	switch s {
	case RunSucceeded, RunStoppedSafe, RunFailed, RunAbandoned, RunCancelled:
		return true
	default:
		return false
	}
}

// RunFence identifies one append-only run attempt for stale-worker fencing;
// every progress/result write carries it (blueprint §9).
type RunFence struct {
	OrgID   int64
	RunID   int64
	Attempt int
}
