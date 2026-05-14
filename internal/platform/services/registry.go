// Package services hosts the platform service registry — the backend authority
// for which services exist. The frontend registry is a bootstrap layer; this is
// the source of truth (see frontend/src/platform/BOUNDARIES.md § Registry authority).
package services

import (
	"sort"
	"sync"

	"github.com/thg/scraper/internal/platform/services/contracts"
)

// Resolver is what a service exposes to the platform. Every method is a pure
// function over a UserContext — deterministic, side-effect free, no IO, no DB
// writes. See specs/DOMAIN_MODEL.md and BOUNDARIES.md § Resolver purity rule.
type Resolver interface {
	Descriptor() contracts.ServiceDescriptor
	ResolveStatus(contracts.UserContext) contracts.ServiceStatus
	ResolveWorkspace(contracts.UserContext) contracts.WorkspaceResolution
	ResolveCapabilities(contracts.UserContext) contracts.ServiceCapabilities
	ResolveAccess(contracts.UserContext) contracts.AccessResolution
}

// Registry holds the set of registered services. Thread-safe.
type Registry struct {
	mu       sync.RWMutex
	services map[string]Resolver
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{services: make(map[string]Resolver)}
}

// Register adds a service. Slugs are immutable and unique — a duplicate panics
// at boot, never silently overwrites.
func (r *Registry) Register(svc Resolver) {
	r.mu.Lock()
	defer r.mu.Unlock()
	slug := svc.Descriptor().Slug
	if slug == "" {
		panic("platform/services: service has empty slug")
	}
	if _, exists := r.services[slug]; exists {
		panic("platform/services: service already registered: " + slug)
	}
	r.services[slug] = svc
}

// Get returns the resolver for a slug.
func (r *Registry) Get(slug string) (Resolver, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	svc, ok := r.services[slug]
	return svc, ok
}

// All returns every registered service, ordered deterministically by
// descriptor.DisplayOrder then slug. No reliance on map iteration order.
func (r *Registry) All() []Resolver {
	r.mu.RLock()
	out := make([]Resolver, 0, len(r.services))
	for _, svc := range r.services {
		out = append(out, svc)
	}
	r.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool {
		di, dj := out[i].Descriptor(), out[j].Descriptor()
		if di.DisplayOrder != dj.DisplayOrder {
			return di.DisplayOrder < dj.DisplayOrder
		}
		return di.Slug < dj.Slug
	})
	return out
}
