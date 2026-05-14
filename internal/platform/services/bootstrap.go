package services

import "github.com/thg/scraper/internal/platform/services/resolver"

// DefaultRegistry builds the platform service registry with every known
// service. Called once at server boot — the backend mirror of the frontend
// bootstrapServices(). Adding a service is one line here.
func DefaultRegistry() *Registry {
	r := NewRegistry()
	r.Register(resolver.NewFacebookResolver())
	// Future: r.Register(resolver.NewTaobaoResolver())
	return r
}
