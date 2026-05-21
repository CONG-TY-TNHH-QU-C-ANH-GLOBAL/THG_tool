package skills

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/thg/scraper/internal/store"
)

func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	db, err := store.New(filepath.Join(t.TempDir(), "skills.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func sampleSkill(id string, def bool) *Skill {
	return &Skill{
		ID:             id,
		Title:          id,
		Description:    "test skill",
		Category:       CategoryAdmin,
		DefaultEnabled: def,
		Parameters: []SkillParam{
			{Name: "msg", Type: "string", Required: true, MaxLen: 100},
		},
		Run: func(ctx context.Context, env Env, args map[string]any) (SkillResult, error) {
			return SkillResult{Summary: "ok " + args["msg"].(string)}, nil
		},
	}
}

func TestRegistryRegisterAndGet(t *testing.T) {
	r := NewRegistry()
	r.Register(sampleSkill("alpha", true))
	r.Register(sampleSkill("beta", false))

	if got := r.Get("alpha"); got == nil || got.ID != "alpha" {
		t.Fatalf("expected alpha, got %+v", got)
	}
	if got := r.Get("missing"); got != nil {
		t.Fatalf("expected nil for missing, got %+v", got)
	}
	if all := r.All(); len(all) != 2 {
		t.Fatalf("expected 2 skills, got %d", len(all))
	}
}

func TestRegistryDefaultBlueprintWhenNoOverrides(t *testing.T) {
	db := newTestStore(t)
	r := NewRegistry()
	r.Register(sampleSkill("alpha", true))
	r.Register(sampleSkill("beta", false)) // not default
	r.Register(sampleSkill("gamma", true))

	enabled := r.EnabledFor(context.Background(), db, 1)
	if len(enabled) != 2 {
		t.Fatalf("expected 2 default-enabled, got %d", len(enabled))
	}
	for _, s := range enabled {
		if s.ID == "beta" {
			t.Fatalf("beta should not be in default blueprint")
		}
	}
}

func TestRegistryRespectsOrgOverrides(t *testing.T) {
	db := newTestStore(t)
	r := NewRegistry()
	r.Register(sampleSkill("alpha", true))
	r.Register(sampleSkill("beta", false))

	// Org 1: explicitly enable beta and disable alpha.
	if err := db.Prompts().SetOrgSkillEnabled(context.Background(), 1, "beta", true, 0); err != nil {
		t.Fatal(err)
	}
	if err := db.Prompts().SetOrgSkillEnabled(context.Background(), 1, "alpha", false, 0); err != nil {
		t.Fatal(err)
	}

	enabled := r.EnabledFor(context.Background(), db, 1)
	if len(enabled) != 1 || enabled[0].ID != "beta" {
		t.Fatalf("expected only beta enabled for org 1, got %+v", enabled)
	}
}

func TestRegistryExecuteRecordsAudit(t *testing.T) {
	db := newTestStore(t)
	r := NewRegistry()
	r.Register(sampleSkill("alpha", true))

	env := Env{DB: db, OrgID: 1, UserID: 7, Source: "test"}
	skill, res, err := r.Execute(context.Background(), env, "alpha", map[string]any{"msg": "hi"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if skill == nil || res.Summary != "ok hi" {
		t.Fatalf("unexpected result: %+v", res)
	}

	rows, err := db.Prompts().ListRecentSkillExecutions(context.Background(), 1, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 audit row, got %d", len(rows))
	}
	if rows[0].SkillID != "alpha" || !rows[0].Success || rows[0].UserID != 7 {
		t.Fatalf("audit row mismatch: %+v", rows[0])
	}
}

func TestRegistryExecuteRefusesDisabledSkill(t *testing.T) {
	db := newTestStore(t)
	r := NewRegistry()
	r.Register(sampleSkill("alpha", true))

	if err := db.Prompts().SetOrgSkillEnabled(context.Background(), 5, "alpha", false, 0); err != nil {
		t.Fatal(err)
	}
	env := Env{DB: db, OrgID: 5, Source: "test"}
	_, _, err := r.Execute(context.Background(), env, "alpha", map[string]any{"msg": "hi"})
	if err == nil {
		t.Fatal("expected error when running a disabled skill")
	}
	rows, _ := db.Prompts().ListRecentSkillExecutions(context.Background(), 5, 10)
	if len(rows) != 1 || rows[0].Success {
		t.Fatalf("expected one failed audit row, got %+v", rows)
	}
}

func TestSkillValidateRejectsMissingRequired(t *testing.T) {
	s := sampleSkill("alpha", true)
	if _, err := s.Validate(map[string]any{}); err == nil {
		t.Fatal("expected validation error for missing required param")
	}
}

func TestSkillValidateAppliesMaxLen(t *testing.T) {
	s := &Skill{
		ID: "x", Title: "x", Description: "x", Category: CategoryAdmin,
		Parameters: []SkillParam{
			{Name: "txt", Type: "string", Required: true, MaxLen: 5},
		},
		Run: func(ctx context.Context, env Env, args map[string]any) (SkillResult, error) {
			return SkillResult{Summary: args["txt"].(string)}, nil
		},
	}
	out, err := s.Validate(map[string]any{"txt": "hello world"})
	if err != nil {
		t.Fatal(err)
	}
	got := out["txt"].(string)
	// runes truncated to 5 + ellipsis appended by sanitizeText
	if got != "hello…" {
		t.Fatalf("expected truncated string, got %q", got)
	}
}

func TestOpenAIToolsShape(t *testing.T) {
	r := NewRegistry()
	r.Register(sampleSkill("alpha", true))
	tools := OpenAITools(r.All())
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}
	fn, _ := tools[0]["function"].(map[string]any)
	if fn["name"] != "alpha" {
		t.Fatalf("unexpected name: %v", fn["name"])
	}
	params, _ := fn["parameters"].(map[string]any)
	if params["type"] != "object" {
		t.Fatalf("unexpected schema: %v", params)
	}
}
