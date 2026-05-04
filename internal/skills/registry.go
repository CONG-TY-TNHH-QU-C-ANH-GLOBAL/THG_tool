package skills

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/thg/scraper/internal/store"
)

// Registry is the runtime catalog of every skill the system knows
// about. It is populated once at process boot from cmd/scraper and
// then read-only for the lifetime of the process. Per-org enablement
// is layered on top via the org_skills table — the registry itself
// stays global.
type Registry struct {
	mu     sync.RWMutex
	skills map[string]*Skill
	order  []string // preserve registration order for stable catalog API
}

// NewRegistry returns an empty registry. Call Register for each skill
// during boot, then hand the registry to the agent and the API server.
func NewRegistry() *Registry {
	return &Registry{
		skills: make(map[string]*Skill),
		order:  make([]string, 0, 32),
	}
}

// Register adds a skill. Panics on duplicate ID — duplicates are
// always a programmer bug caught immediately at boot, never a runtime
// surprise.
func (r *Registry) Register(s *Skill) {
	if s == nil || s.ID == "" {
		panic("skills: Register called with nil or empty-id skill")
	}
	if s.Run == nil {
		panic("skills: Register called for " + s.ID + " with nil Run")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.skills[s.ID]; exists {
		panic("skills: duplicate registration: " + s.ID)
	}
	r.skills[s.ID] = s
	r.order = append(r.order, s.ID)
}

// Get returns the skill by ID, or nil when the ID is not registered.
func (r *Registry) Get(id string) *Skill {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.skills[id]
}

// All returns every registered skill in registration order. Used by
// the admin /api/skills/all endpoint.
func (r *Registry) All() []*Skill {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*Skill, 0, len(r.order))
	for _, id := range r.order {
		out = append(out, r.skills[id])
	}
	return out
}

// EnabledFor returns the subset of skills enabled for orgID.
//
// When the org has zero rows in org_skills, the full default
// blueprint applies (every skill with DefaultEnabled=true). This means
// new orgs work immediately without admin setup but admins can still
// opt out of any individual skill.
func (r *Registry) EnabledFor(ctx context.Context, db *store.Store, orgID int64) []*Skill {
	all := r.All()
	if db == nil || orgID <= 0 {
		return defaultBlueprint(all)
	}
	overrides, err := store.LoadOrgSkillOverrides(ctx, db, orgID)
	if err != nil || len(overrides) == 0 {
		return defaultBlueprint(all)
	}
	out := make([]*Skill, 0, len(all))
	for _, s := range all {
		state, hasOverride := overrides[s.ID]
		if hasOverride {
			if state {
				out = append(out, s)
			}
			continue
		}
		if s.DefaultEnabled {
			out = append(out, s)
		}
	}
	return out
}

func defaultBlueprint(all []*Skill) []*Skill {
	out := make([]*Skill, 0, len(all))
	for _, s := range all {
		if s.DefaultEnabled {
			out = append(out, s)
		}
	}
	return out
}

// OpenAITools projects a slice of skills into the OpenAI function
// calling tool list shape. Used by the resolver when calling the LLM
// — the LLM only sees skills the org has actually enabled.
func OpenAITools(enabled []*Skill) []map[string]any {
	out := make([]map[string]any, 0, len(enabled))
	for _, s := range enabled {
		out = append(out, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        s.ID,
				"description": s.Description,
				"parameters":  s.ParamSchema(),
			},
		})
	}
	return out
}

// CatalogEntry is the JSON shape returned by /api/skills.
type CatalogEntry struct {
	ID             string         `json:"id"`
	Title          string         `json:"title"`
	Description    string         `json:"description"`
	Category       SkillCategory  `json:"category"`
	Outbound       bool           `json:"outbound"`
	NeedsAccount   bool           `json:"needs_account"`
	DefaultEnabled bool           `json:"default_enabled"`
	Enabled        bool           `json:"enabled"`
	Parameters     []SkillParam   `json:"parameters"`
}

// Catalog returns every skill plus whether it is enabled for orgID.
// Admin UI uses this to render the toggle list.
func (r *Registry) Catalog(ctx context.Context, db *store.Store, orgID int64) []CatalogEntry {
	all := r.All()
	enabled := map[string]bool{}
	for _, s := range r.EnabledFor(ctx, db, orgID) {
		enabled[s.ID] = true
	}
	out := make([]CatalogEntry, 0, len(all))
	for _, s := range all {
		out = append(out, CatalogEntry{
			ID:             s.ID,
			Title:          s.Title,
			Description:    s.Description,
			Category:       s.Category,
			Outbound:       s.Outbound,
			NeedsAccount:   s.NeedsAccount,
			DefaultEnabled: s.DefaultEnabled,
			Enabled:        enabled[s.ID],
			Parameters:     s.Parameters,
		})
	}
	// Stable sort by category then ID for predictable UI rendering.
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Category != out[j].Category {
			return out[i].Category < out[j].Category
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// Run resolves the skill, validates params, runs it, and writes one
// row to skill_executions for the audit trail. Returns the
// SkillResult plus the canonicalised skill ID actually invoked.
//
// Errors from validation, missing-skill, or disabled-skill are
// surfaced before Run so the audit row records why nothing happened.
func (r *Registry) Execute(ctx context.Context, env Env, skillID string, args map[string]any) (*Skill, SkillResult, error) {
	skill := r.Get(skillID)
	if skill == nil {
		_ = recordExecution(ctx, env, skillID, args, SkillResult{}, fmt.Errorf("unknown skill"))
		return nil, SkillResult{}, fmt.Errorf("skill %q is not registered", skillID)
	}
	if env.DB != nil && env.OrgID > 0 {
		enabled := r.EnabledFor(ctx, env.DB, env.OrgID)
		if !containsSkill(enabled, skillID) {
			err := fmt.Errorf("skill %q is not enabled for this org", skillID)
			_ = recordExecution(ctx, env, skillID, args, SkillResult{}, err)
			return skill, SkillResult{}, err
		}
	}
	validated, err := skill.Validate(args)
	if err != nil {
		_ = recordExecution(ctx, env, skillID, args, SkillResult{}, err)
		return skill, SkillResult{}, err
	}
	res, runErr := skill.Run(ctx, env, validated)
	_ = recordExecution(ctx, env, skillID, validated, res, runErr)
	return skill, res, runErr
}

func containsSkill(set []*Skill, id string) bool {
	for _, s := range set {
		if s.ID == id {
			return true
		}
	}
	return false
}

func recordExecution(ctx context.Context, env Env, skillID string, args map[string]any, res SkillResult, runErr error) error {
	if env.DB == nil {
		return nil
	}
	argsJSON, _ := json.Marshal(args)
	errMsg := ""
	success := runErr == nil
	if runErr != nil {
		errMsg = runErr.Error()
	}
	return store.RecordSkillExecution(ctx, env.DB, store.SkillExecution{
		OrgID:    env.OrgID,
		UserID:   env.UserID,
		Source:   env.Source,
		SkillID:  skillID,
		ArgsJSON: string(argsJSON),
		Summary:  res.Summary,
		Success:  success,
		Error:    errMsg,
		At:       time.Now().UTC(),
	})
}
