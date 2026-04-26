package learning

import (
	"context"
	"sync"
	"time"
)

const emaAlpha = 0.15 // smoothing factor for exponential moving average

// OutcomeSignal represents feedback on a scored lead.
type OutcomeSignal struct {
	OrgID    int64
	LeadID   int64
	Category string  // "hot", "warm", "cold"
	Outcome  string  // "converted", "rejected", "ignored"
	Score    float64 // original lead score at time of outcome
}

// Weights mirrors scoring.Weights but lives here to avoid import cycles.
type Weights struct {
	KeywordRelevance float64 `json:"keyword_relevance"`
	Engagement       float64 `json:"engagement"`
	ContentQuality   float64 `json:"content_quality"`
}

func defaultWeights() Weights {
	return Weights{KeywordRelevance: 0.40, Engagement: 0.30, ContentQuality: 0.30}
}

// orgState holds learned state for one organisation.
type orgState struct {
	weights      Weights
	convertedN   int
	rejectedN    int
	ignoredN     int
	lastUpdated  time.Time
}

// Engine maintains per-org adaptive scoring weights.
// It uses an exponential moving average to adjust weights based on outcome signals.
// Thread-safe; all methods may be called concurrently.
type Engine struct {
	mu     sync.RWMutex
	orgs   map[int64]*orgState
	persist Persister
}

// Persister is an optional callback to save weight snapshots to the DB.
// If nil, weights live in-memory only.
type Persister interface {
	SaveWeights(ctx context.Context, orgID int64, w Weights, updatedAt time.Time) error
	LoadWeights(ctx context.Context, orgID int64) (Weights, bool, error)
}

func New(persist Persister) *Engine {
	return &Engine{
		orgs:    make(map[int64]*orgState),
		persist: persist,
	}
}

// GetCurrentWeights returns the current adaptive weights for the given org.
// Falls back to defaults if no data has been collected yet.
func (e *Engine) GetCurrentWeights(ctx context.Context, orgID int64) Weights {
	e.mu.RLock()
	st, ok := e.orgs[orgID]
	e.mu.RUnlock()
	if ok {
		return st.weights
	}

	// Try loading persisted weights
	if e.persist != nil {
		if w, found, err := e.persist.LoadWeights(ctx, orgID); err == nil && found {
			e.mu.Lock()
			if _, still := e.orgs[orgID]; !still {
				e.orgs[orgID] = &orgState{weights: w}
			}
			e.mu.Unlock()
			return w
		}
	}
	return defaultWeights()
}

// ProcessOutcome ingests a conversion / rejection signal and adjusts weights via EMA.
//
// Conversion signals (outcome == "converted") for hot leads indicate that
// keyword_relevance and engagement weights should increase — the model is finding
// real buyers. Rejection signals suggest over-weighting; we nudge down.
func (e *Engine) ProcessOutcome(ctx context.Context, sig OutcomeSignal) error {
	e.mu.Lock()
	st := e.getOrCreate(sig.OrgID)

	switch sig.Outcome {
	case "converted":
		st.convertedN++
		// Reinforce whichever dimension is currently highest (the scoring model was right)
		st.weights = adjustWeights(st.weights, sig.Category, +1)
	case "rejected":
		st.rejectedN++
		st.weights = adjustWeights(st.weights, sig.Category, -1)
	default:
		st.ignoredN++
	}
	st.weights = normalise(st.weights)
	st.lastUpdated = time.Now()
	w := st.weights
	e.mu.Unlock()

	if e.persist != nil {
		return e.persist.SaveWeights(ctx, sig.OrgID, w, time.Now())
	}
	return nil
}

// Stats returns outcome counts for one org (for the learning dashboard page).
func (e *Engine) Stats(orgID int64) (converted, rejected, ignored int, lastUpdated time.Time) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if st, ok := e.orgs[orgID]; ok {
		return st.convertedN, st.rejectedN, st.ignoredN, st.lastUpdated
	}
	return 0, 0, 0, time.Time{}
}

func (e *Engine) getOrCreate(orgID int64) *orgState {
	if st, ok := e.orgs[orgID]; ok {
		return st
	}
	st := &orgState{weights: defaultWeights()}
	e.orgs[orgID] = st
	return st
}

// adjustWeights nudges individual dimension weights using EMA.
// direction > 0 = reinforce, direction < 0 = penalise.
func adjustWeights(w Weights, category string, direction float64) Weights {
	delta := emaAlpha * direction

	// Hot leads are keyword + engagement driven; cold/warm are content quality signals
	switch category {
	case "hot":
		w.KeywordRelevance = clamp(w.KeywordRelevance+delta*0.6, 0.15, 0.70)
		w.Engagement = clamp(w.Engagement+delta*0.4, 0.10, 0.60)
	case "warm":
		w.KeywordRelevance = clamp(w.KeywordRelevance+delta*0.4, 0.15, 0.70)
		w.ContentQuality = clamp(w.ContentQuality+delta*0.4, 0.10, 0.60)
	case "cold":
		w.ContentQuality = clamp(w.ContentQuality+delta*0.5, 0.10, 0.60)
		w.Engagement = clamp(w.Engagement+delta*0.3, 0.10, 0.60)
	}
	return w
}

// normalise ensures weights sum to exactly 1.0.
func normalise(w Weights) Weights {
	total := w.KeywordRelevance + w.Engagement + w.ContentQuality
	if total == 0 {
		return defaultWeights()
	}
	return Weights{
		KeywordRelevance: w.KeywordRelevance / total,
		Engagement:       w.Engagement / total,
		ContentQuality:   w.ContentQuality / total,
	}
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
