package session

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/thg/scraper/internal/store/sessions"
)

// RegistryEntry is the in-memory view of a browser session.
type RegistryEntry struct {
	Session    sessions.BrowserSession
	ActiveJobs int32 // atomically incremented by worker goroutines
	HealthOK   bool
	LastCheck  time.Time
}

// RegistryStats is returned by Registry.Stats() for the /api/sessions/stats endpoint.
type RegistryStats struct {
	Total       int `json:"total"`
	Idle        int `json:"idle"`
	Active      int `json:"active"`
	Recovering  int `json:"recovering"`
	Terminated  int `json:"terminated"`
	Initializing int `json:"initializing"`
}

// Registry is an in-memory mirror of browser_sessions for fast reads.
// It is NOT used for coordination (workers use the DB-level Allocator).
// The primary use is the /api/browser/workspaces endpoint and health checks.
type Registry struct {
	mu      sync.RWMutex
	entries map[int64]*RegistryEntry // keyed by account_id
	store   *sessions.Store
}

// NewRegistry creates an empty registry.
func NewRegistry(sessionsStore *sessions.Store) *Registry {
	return &Registry{
		entries: make(map[int64]*RegistryEntry),
		store:   sessionsStore,
	}
}

// LoadAll syncs the registry from the database. Call once on startup.
func (r *Registry) LoadAll(ctx context.Context) error {
	sessions, err := r.store.ListAllActiveSessions(ctx)
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, s := range sessions {
		s := s
		r.entries[s.AccountID] = &RegistryEntry{Session: s, HealthOK: true}
	}
	return nil
}

// Upsert inserts or updates the registry entry for a session.
// Called by the workspace manager after start/stop/health events.
func (r *Registry) Upsert(s sessions.BrowserSession) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.entries[s.AccountID]
	if !ok {
		e = &RegistryEntry{HealthOK: true}
		r.entries[s.AccountID] = e
	}
	e.Session = s
}

// SetHealth marks the health status for a given account's session.
func (r *Registry) SetHealth(accountID int64, healthy bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if e, ok := r.entries[accountID]; ok {
		e.HealthOK = healthy
		e.LastCheck = time.Now()
	}
}

// Remove deletes the registry entry for accountID.
func (r *Registry) Remove(accountID int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.entries, accountID)
}

// Get returns the registry entry for accountID, or nil.
func (r *Registry) Get(accountID int64) *RegistryEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.entries[accountID]
}

// List returns a snapshot of all entries.
func (r *Registry) List() []*RegistryEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*RegistryEntry, 0, len(r.entries))
	for _, e := range r.entries {
		out = append(out, e)
	}
	return out
}

// Stats returns aggregated counts for the /api/sessions/stats endpoint.
func (r *Registry) Stats() RegistryStats {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var s RegistryStats
	s.Total = len(r.entries)
	for _, e := range r.entries {
		switch e.Session.Status {
		case "idle":
			s.Idle++
		case "active":
			s.Active++
		case "recovering":
			s.Recovering++
		case "terminated":
			s.Terminated++
		case "initializing":
			s.Initializing++
		}
	}
	return s
}

// IncrementActive atomically increments the active job counter for an account.
func (r *Registry) IncrementActive(accountID int64) {
	r.mu.RLock()
	e := r.entries[accountID]
	r.mu.RUnlock()
	if e != nil {
		atomic.AddInt32(&e.ActiveJobs, 1)
	}
}

// DecrementActive atomically decrements the active job counter for an account.
func (r *Registry) DecrementActive(accountID int64) {
	r.mu.RLock()
	e := r.entries[accountID]
	r.mu.RUnlock()
	if e != nil {
		atomic.AddInt32(&e.ActiveJobs, -1)
	}
}
