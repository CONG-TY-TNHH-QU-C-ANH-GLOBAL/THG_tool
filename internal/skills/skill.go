// Package skills is the open-prompt agent runtime — every Facebook
// capability the dashboard / Telegram chatbot can invoke is registered
// as a skill at process boot. The user types one open prompt and the
// resolver picks exactly one skill (deterministic fast-path or LLM
// function-calling fallback). No hardcoded vertical: HR, POD, sales,
// support are blueprints over the same primitives.
//
// Why a struct instead of an interface: every skill is data plus a
// single Run closure that captures dependencies at registration time
// (in cmd/scraper). Keeping skills as data lets the registry serialise
// them straight into the /api/skills catalog and lets the LLM tool
// list be a 1:1 projection of the registered set.
package skills

import (
	"context"
	"fmt"
	"strings"

	"github.com/thg/scraper/internal/jobs"
	"github.com/thg/scraper/internal/prompt"
	"github.com/thg/scraper/internal/store"
)

// SkillCategory groups skills in the dashboard catalog and lets the
// resolver hint to the LLM about scope (e.g. "the user is asking about
// inbox replies, prefer Category=inbox skills").
type SkillCategory string

const (
	CategoryPost    SkillCategory = "post"
	CategoryComment SkillCategory = "comment"
	CategoryInbox   SkillCategory = "inbox"
	CategoryScrape  SkillCategory = "scrape"
	CategoryCare    SkillCategory = "care"  // fanpage / profile maintenance
	CategoryAdmin   SkillCategory = "admin" // describe_business, get_stats
)

// Env carries the non-AI dependencies a skill needs at runtime. AI deps
// (MessageGenerator, classifier) are not exposed here on purpose —
// concrete builtin closures registered from cmd/scraper capture them so
// internal/skills stays free of an internal/ai import cycle.
type Env struct {
	DB       *store.Store
	AppStore *store.AppStore
	JobStore *jobs.Store
	Notify   func(string)

	OrgID     int64
	UserID    int64  // operator who triggered the prompt; 0 = Telegram bot
	AccountID int64  // Facebook account already resolved, 0 if not picked
	Source    string // "dashboard" | "telegram" | "api"
	Prompt    string // original user text — for audit + skill heuristics
}

// SkillResult is the structured outcome of one skill execution.
//
// Summary is the line surfaced back to the chat / Telegram. The
// counters (Approved / Drafted / Skipped) flow into the audit log so
// operators can see at a glance whether a bulk run produced live
// outbound or queued drafts. Data carries arbitrary structured payload
// for the dashboard (e.g. created_job_id, lead_count, screenshot_url).
type SkillResult struct {
	Summary     string
	Data        any
	Approved    int
	Drafted     int
	Skipped     int
	SkipReasons map[string]int
}

// SkillRun is the executor function. Receives validated params after
// the registry type-checks them via Skill.Validate.
type SkillRun func(ctx context.Context, env Env, params map[string]any) (SkillResult, error)

// SkillParam describes one input the skill accepts. Used to render the
// JSON Schema for OpenAI function calling AND to validate incoming
// arguments before the Run closure is invoked.
type SkillParam struct {
	Name        string
	Type        string // "string", "int", "bool", "url", "enum"
	Description string
	Required    bool
	Enum        []string // when Type == "enum"
	MaxLen      int      // 0 = no limit; applied to string/url
	Default     any      // fallback when not Required and value is missing
}

// Skill is the canonical capability descriptor — exposed by the
// registry, registered at boot, and serialised straight into the
// catalog API.
type Skill struct {
	ID             string // snake_case, stable across versions
	Title          string // human-readable label (Vietnamese OK)
	Description    string // LLM-facing description
	Category       SkillCategory
	Outbound       bool // true → must use store.QueueOutboundForOrg
	NeedsAccount   bool // true → resolver must pass AccountID
	DefaultEnabled bool // true → enabled out-of-the-box for new orgs
	Parameters     []SkillParam
	Run            SkillRun
}

// ParamSchema renders the parameter list as an OpenAI-compatible JSON
// Schema {"type":"object","properties":{...},"required":[...]}.
func (s *Skill) ParamSchema() map[string]any {
	props := map[string]any{}
	required := make([]string, 0, len(s.Parameters))
	for _, p := range s.Parameters {
		schema := map[string]any{"description": p.Description}
		switch p.Type {
		case "int":
			schema["type"] = "integer"
		case "bool":
			schema["type"] = "boolean"
		case "url":
			schema["type"] = "string"
			schema["format"] = "uri"
		case "enum":
			schema["type"] = "string"
			if len(p.Enum) > 0 {
				schema["enum"] = p.Enum
			}
		default:
			schema["type"] = "string"
		}
		props[p.Name] = schema
		if p.Required {
			required = append(required, p.Name)
		}
	}
	out := map[string]any{
		"type":       "object",
		"properties": props,
	}
	if len(required) > 0 {
		out["required"] = required
	}
	return out
}

// Validate type-checks an incoming arg map against the skill's
// parameter list, applies MaxLen truncation, fills defaults, and
// rejects unknown enum values. Returns a normalised map suitable for
// passing into Run.
//
// Treats user-supplied strings as untrusted: control characters are
// stripped (mirrors ai.sanitizeForPrompt) so a malicious prompt cannot
// smuggle prompt-injection markers through into downstream LLM calls.
func (s *Skill) Validate(in map[string]any) (map[string]any, error) {
	out := make(map[string]any, len(s.Parameters))
	for _, p := range s.Parameters {
		raw, present := in[p.Name]
		if !present {
			if p.Required {
				return nil, fmt.Errorf("skill %q: missing required param %q", s.ID, p.Name)
			}
			if p.Default != nil {
				out[p.Name] = p.Default
			}
			continue
		}
		switch p.Type {
		case "int":
			n, err := coerceInt(raw)
			if err != nil {
				return nil, fmt.Errorf("skill %q param %q: %w", s.ID, p.Name, err)
			}
			out[p.Name] = n
		case "bool":
			out[p.Name] = coerceBool(raw)
		case "enum":
			str := strings.TrimSpace(fmt.Sprint(raw))
			if !containsString(p.Enum, str) {
				return nil, fmt.Errorf("skill %q param %q: %q not in %v", s.ID, p.Name, str, p.Enum)
			}
			out[p.Name] = str
		default: // string, url
			str := prompt.SanitizeText(fmt.Sprint(raw), p.MaxLen)
			out[p.Name] = str
		}
	}
	return out, nil
}

func coerceInt(raw any) (int64, error) {
	switch v := raw.(type) {
	case int:
		return int64(v), nil
	case int64:
		return v, nil
	case float64:
		return int64(v), nil
	case string:
		n := int64(0)
		if _, err := fmt.Sscanf(v, "%d", &n); err != nil {
			return 0, fmt.Errorf("not an integer: %q", v)
		}
		return n, nil
	default:
		return 0, fmt.Errorf("unsupported integer type %T", raw)
	}
}

func coerceBool(raw any) bool {
	switch v := raw.(type) {
	case bool:
		return v
	case string:
		s := strings.ToLower(strings.TrimSpace(v))
		return s == "true" || s == "1" || s == "yes" || s == "y" || s == "auto"
	case int, int64, float64:
		return fmt.Sprint(v) != "0"
	default:
		return false
	}
}

func containsString(set []string, value string) bool {
	for _, s := range set {
		if s == value {
			return true
		}
	}
	return false
}
