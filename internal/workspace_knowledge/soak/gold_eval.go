package soak

import (
	"strconv"

	"github.com/thg/scraper/internal/workspace_knowledge/assets"
)

// promptField is the shared diagnostic-string fragment used when
// building blocking-incident messages. Defined once.
const promptField = " prompt="

// CIGateResult is the structured outcome of the CI gate. Used by the
// test that runs the gate; CI also reads the JSON form when posting to
// PR reviews.
type CIGateResult struct {
	TotalPrompts      int                  `json:"total_prompts"`
	GovernanceLeaks   int                  `json:"governance_leaks"`
	TenantLeaks       int                  `json:"tenant_leaks"`
	BannedRetrieved   int                  `json:"banned_retrieved"`
	ByCategory        map[GoldCategory]int `json:"by_category"`
	Verdict           string               `json:"verdict"` // "PASS" | "FAIL"
	BlockingIncidents []string             `json:"blocking_incidents"`
}

// EvaluateGoldDataset runs every gold prompt through `runQuery`,
// collects leaks, and returns the gate verdict.
//
// HARD-FAIL CONDITIONS (per goal G1):
//
//   - any governance leak (banned_claim returned for any prompt)
//   - any tenant leak (asset from a different org returned)
//   - any banned_claim retrieved at all
//
// Soft-pass conditions: intent precision dropping below threshold
// produces a warning but not a FAIL. The hard gate is correctness +
// safety; quality regressions are tracked separately.
//
// `isolationOrgIDs` lets the caller seed isolation probes: each gold
// prompt in GoldTenantIsolation category runs against EACH org in turn;
// the function asserts no org sees another org's titles. Pass an empty
// slice to skip the isolation check entirely.
func EvaluateGoldDataset(
	runQuery func(orgID int64, prompt GoldPrompt) ([]string, []*assets.Asset, error),
	prompts []GoldPrompt,
	isolationOrgIDs []int64,
) *CIGateResult {
	r := &CIGateResult{ByCategory: map[GoldCategory]int{}}
	if len(prompts) == 0 {
		prompts = GoldDataset()
	}
	primaryOrg := int64(7777)
	if len(isolationOrgIDs) > 0 {
		primaryOrg = isolationOrgIDs[0]
	}

	for _, p := range prompts {
		r.TotalPrompts++

		// Non-isolation prompts run once against the primary org.
		// Isolation prompts run against every supplied org.
		orgsToRun := []int64{primaryOrg}
		if p.Category == GoldTenantIsolation && len(isolationOrgIDs) > 0 {
			orgsToRun = isolationOrgIDs
		}

		evaluatePrompt(r, runQuery, p, orgsToRun)
		r.ByCategory[p.Category]++
	}

	if r.GovernanceLeaks > 0 || r.TenantLeaks > 0 || r.BannedRetrieved > 0 {
		r.Verdict = "FAIL"
	} else {
		r.Verdict = "PASS"
	}
	return r
}

// evaluatePrompt runs one gold prompt against every org it targets,
// recording any query error, leak, or forbidden-title incident into r.
func evaluatePrompt(
	r *CIGateResult,
	runQuery func(orgID int64, prompt GoldPrompt) ([]string, []*assets.Asset, error),
	p GoldPrompt,
	orgsToRun []int64,
) {
	for _, orgID := range orgsToRun {
		titles, hitAssets, err := runQuery(orgID, p)
		if err != nil {
			r.BlockingIncidents = append(r.BlockingIncidents,
				"query error org="+strconv.FormatInt(orgID, 10)+promptField+p.Text+": "+err.Error())
			continue
		}
		checkRetrievedAssets(r, orgID, p, hitAssets)
		checkForbiddenTitles(r, orgID, p, titles)
	}
}

// checkRetrievedAssets enforces HARD CHECK 1 (banned-claim asset
// surfaced) and HARD CHECK 2 (tenant leak — asset from a different org)
// over the retrieved assets.
func checkRetrievedAssets(r *CIGateResult, orgID int64, p GoldPrompt, hitAssets []*assets.Asset) {
	for _, a := range hitAssets {
		if a == nil {
			continue
		}
		if a.Type == assets.AssetBannedClaim {
			r.BannedRetrieved++
			r.GovernanceLeaks++
			r.BlockingIncidents = append(r.BlockingIncidents,
				"BANNED CLAIM in retrieval: org="+strconv.FormatInt(orgID, 10)+promptField+p.Text+" title="+a.Title)
		}
		if a.OrgID != orgID {
			r.TenantLeaks++
			r.BlockingIncidents = append(r.BlockingIncidents,
				"TENANT LEAK: query org="+strconv.FormatInt(orgID, 10)+" returned asset from org="+strconv.FormatInt(a.OrgID, 10))
		}
	}
}

// checkForbiddenTitles enforces HARD CHECK 3: titles a prompt marks
// MustNotSurface must never appear in the returned titles.
func checkForbiddenTitles(r *CIGateResult, orgID int64, p GoldPrompt, titles []string) {
	for _, forbidden := range p.MustNotSurfaceTitles {
		for _, title := range titles {
			if title == forbidden {
				r.GovernanceLeaks++
				r.BlockingIncidents = append(r.BlockingIncidents,
					"FORBIDDEN TITLE surfaced: org="+strconv.FormatInt(orgID, 10)+promptField+p.Text+" title="+title)
			}
		}
	}
}
