package leadoutreach

// State accumulates one queueLeadOutreach pass's counters + diagnostics. Pure
// (store-free) — colocated with the outcome formatters that read it. Queued and
// Scanned are exported because the caller (cmd) reads/bumps them directly; all
// other counters/diagnostics stay package-private.
type State struct {
	Queued        int
	skipped       int
	approvedCount int
	Scanned       int
	skipReasons   map[string]int
	// skipSamples keeps up to 5 sample lead IDs per skip reason (diagnosability).
	skipSamples map[string][]int64
	lastGenErr  error
	// riskBlock* capture the last risk_ceiling_exceeded deny for the response.
	riskBlockSeen    bool
	riskBlockRisk    float64
	riskBlockCeiling float64
}

func NewState() *State {
	return &State{
		skipReasons: map[string]int{},
		skipSamples: map[string][]int64{},
	}
}

func (s *State) recordSkip(reason string, leadID int64) {
	s.skipped++
	s.skipReasons[reason]++
	if leadID > 0 && len(s.skipSamples[reason]) < 5 {
		s.skipSamples[reason] = append(s.skipSamples[reason], leadID)
	}
}
