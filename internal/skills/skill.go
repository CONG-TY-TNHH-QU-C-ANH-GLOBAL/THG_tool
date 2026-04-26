package skills

import "context"

// SkillResult is returned by every skill execution.
type SkillResult struct {
	Summary string `json:"summary"`
	Data    any    `json:"data,omitempty"`
}

// Skill is the interface every Facebook automation skill must implement.
// Skills are registered in the Registry at startup and dispatched by the TaskExecutor.
type Skill interface {
	// Name is the canonical machine-readable identifier (snake_case).
	Name() string
	// Description is shown to GPT-4o as the function description for routing.
	Description() string
	// ParamSchema returns a JSON Schema map used by OpenAI function calling.
	// Keys are param names; values are {"type": "string", "description": "..."}.
	ParamSchema() map[string]any
	// Run executes the skill for the given account with validated params.
	// ctx is already attached to a live chromedp workspace for accountID.
	Run(ctx context.Context, accountID int64, params map[string]any) (SkillResult, error)
}

// Registry is the canonical map of skill name → Skill implementation.
type Registry struct {
	skills map[string]Skill
}

func NewRegistry() *Registry {
	return &Registry{skills: make(map[string]Skill)}
}

// Register adds a skill. Panics on duplicate name (caught at startup).
func (r *Registry) Register(s Skill) {
	if _, exists := r.skills[s.Name()]; exists {
		panic("skills: duplicate registration: " + s.Name())
	}
	r.skills[s.Name()] = s
}

// Get returns a skill by name, nil if not found.
func (r *Registry) Get(name string) Skill {
	return r.skills[name]
}

// All returns all registered skills (for building the OpenAI function list).
func (r *Registry) All() []Skill {
	out := make([]Skill, 0, len(r.skills))
	for _, s := range r.skills {
		out = append(out, s)
	}
	return out
}
