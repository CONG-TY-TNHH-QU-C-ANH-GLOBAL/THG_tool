package services

import "github.com/thg/scraper/internal/platform/services/resolver"

// DefaultRegistry builds the platform service registry with every known
// service. Called once at server boot — the backend mirror of the frontend
// bootstrapServices(). Adding a service is one line here.
//
// Taobao and 1688 are registered as stubs (ResolveStatus → "unavailable")
// so the /services page reflects the multi-service platform from day one.
// When those backends land, only the resolver implementation changes —
// no registry surgery required.
func DefaultRegistry() *Registry {
	r := NewRegistry()
	r.Register(resolver.NewFacebookResolver())
	r.Register(resolver.NewTaobaoResolver())
	r.Register(resolver.NewAlibaba1688Resolver())
	return r
}
