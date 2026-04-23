package orchestrator

import (
	"testing"
	"time"

	"github.com/thg/scraper/internal/ai"
	"github.com/thg/scraper/internal/models"
)

// --- groupCooldown ---

func TestGroupCooldown_HighPriorityHighQuality(t *testing.T) {
	d := groupCooldown(0.85, "high")
	if d != 24*time.Hour {
		t.Errorf("expected 24h, got %v", d)
	}
}

func TestGroupCooldown_HighPriorityLowQuality(t *testing.T) {
	d := groupCooldown(0.3, "high")
	if d != 3*24*time.Hour {
		t.Errorf("expected 72h, got %v", d)
	}
}

func TestGroupCooldown_MediumPriorityHighQuality(t *testing.T) {
	d := groupCooldown(0.75, "medium")
	if d != 2*24*time.Hour {
		t.Errorf("expected 48h, got %v", d)
	}
}

func TestGroupCooldown_MediumPriorityLowQuality(t *testing.T) {
	d := groupCooldown(0.5, "medium")
	if d != 3*24*time.Hour {
		t.Errorf("expected 72h, got %v", d)
	}
}

func TestGroupCooldown_LowPriority(t *testing.T) {
	d := groupCooldown(0.9, "low")
	if d != 7*24*time.Hour {
		t.Errorf("expected 168h, got %v", d)
	}
}

func TestGroupCooldown_CaseInsensitive(t *testing.T) {
	d1 := groupCooldown(0.8, "HIGH")
	d2 := groupCooldown(0.8, "high")
	if d1 != d2 {
		t.Errorf("case sensitivity bug: HIGH=%v high=%v", d1, d2)
	}
}

// --- commentThreshold / inboxThreshold ---

func TestCommentThreshold(t *testing.T) {
	tests := []struct {
		priority string
		want     float64
	}{
		{"high", 0.35},
		{"medium", 0.55},
		{"low", 0.70},
		{"", 0.55},   // default
		{"HIGH", 0.35}, // case insensitive
	}
	for _, tt := range tests {
		got := commentThreshold(tt.priority)
		if got != tt.want {
			t.Errorf("commentThreshold(%q) = %.2f, want %.2f", tt.priority, got, tt.want)
		}
	}
}

func TestInboxThreshold(t *testing.T) {
	tests := []struct {
		priority string
		want     float64
	}{
		{"high", 0.55},
		{"medium", 0.67},
		{"low", 0.82},
	}
	for _, tt := range tests {
		got := inboxThreshold(tt.priority)
		if got != tt.want {
			t.Errorf("inboxThreshold(%q) = %.2f, want %.2f", tt.priority, got, tt.want)
		}
	}
}

// --- normalizeGroupCategory ---

func TestNormalizeGroupCategory(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"tech", "tech"},
		{"sales", "sales"},
		{"ops", "ops"},
		{"finance", "finance"},
		{"tuyển dụng", "general"},
		{"hr recruitment", "general"},
		{"unknown", "general"},
		{"TECH", "tech"}, // case insensitive
		{"e-commerce operations", "ops"},
		{"logistics & supply chain", "ops"},
	}
	for _, tt := range tests {
		got := normalizeGroupCategory(tt.input)
		if got != tt.want {
			t.Errorf("normalizeGroupCategory(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// --- dedupeGroups ---

func TestDedupeGroups(t *testing.T) {
	groups := []models.Group{
		{ID: 1, Name: "A"},
		{ID: 2, Name: "B"},
		{ID: 1, Name: "A dup"},
		{ID: 3, Name: "C"},
		{ID: 2, Name: "B dup"},
	}
	got := dedupeGroups(groups)
	if len(got) != 3 {
		t.Errorf("expected 3 unique groups, got %d", len(got))
	}
	seen := make(map[int64]bool)
	for _, g := range got {
		if seen[g.ID] {
			t.Errorf("duplicate ID %d in deduped result", g.ID)
		}
		seen[g.ID] = true
	}
}

func TestDedupeGroups_Empty(t *testing.T) {
	got := dedupeGroups(nil)
	if len(got) != 0 {
		t.Errorf("expected empty result, got %d", len(got))
	}
}

// --- ai.JobDomainCategory ---

func TestJobDomainCategory(t *testing.T) {
	tests := []struct {
		title string
		want  string
	}{
		{"Software Developer", "tech"},
		{"Backend Engineer", "tech"},
		{"Frontend Vietnam Intern", "tech"},
		{"Data Scientist HCM", "tech"},
		{"Accountant Senior", "finance"},
		{"Kế toán Tổng hợp", "finance"},
		{"Sales Executive", "sales"},
		{"POD / Dropship Sales Executive", "sales"},
		{"China Desk Executive", "sales"},
		{"Kinh doanh Quốc tế", "sales"},
		{"E-Commerce Operations Executive", "ops"},
		{"Logistics Coordinator", "ops"},
		{"Warehouse Supervisor", "ops"},
		{"International Shipping Sales Executive", "sales"}, // sales checked before ops
		{"HR Manager", "sales"},                            // no matching keyword → default sales
	}
	for _, tt := range tests {
		job := models.CareerJob{Title: tt.title}
		got := ai.JobDomainCategory(job)
		if got != tt.want {
			t.Errorf("JobDomainCategory(%q) = %q, want %q", tt.title, got, tt.want)
		}
	}
}

// --- domainSearchQuery ---

func TestDomainSearchQuery(t *testing.T) {
	domains := []string{"tech", "sales", "ops", "finance", "unknown"}
	for _, d := range domains {
		q := domainSearchQuery(d, "Test Job")
		if q == "" {
			t.Errorf("domainSearchQuery(%q) returned empty string", d)
		}
	}
}
