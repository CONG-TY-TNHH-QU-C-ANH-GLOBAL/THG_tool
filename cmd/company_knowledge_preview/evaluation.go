package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type evaluationCase struct {
	ID        string   `json:"id"`
	Query     string   `json:"query"`
	Public    bool     `json:"public_facts_only"`
	Expected  []string `json:"expected_any"`
	Top       int      `json:"top"`
	NoResults bool     `json:"expect_no_results"`
}

type evaluationResult struct {
	Case     evaluationCase `json:"case"`
	Before   []string       `json:"before_ids"`
	After    []string       `json:"after_ids"`
	BeforeOK bool           `json:"before_passed"`
	AfterOK  bool           `json:"after_passed"`
}

type evaluationReport struct {
	Scope          string             `json:"scope"`
	RuntimeEnabled bool               `json:"runtime_enabled"`
	PackHash       string             `json:"knowledge_sha256"`
	CasesHash      string             `json:"cases_sha256"`
	BeforeImpl     string             `json:"before_impl"`
	AfterImpl      string             `json:"after_impl"`
	BeforePassed   int                `json:"before_passed"`
	AfterPassed    int                `json:"after_passed"`
	Results        []evaluationResult `json:"results"`
}

func evaluateRetrieval(dir, file string, review bool) error {
	if !review {
		return fmt.Errorf("evaluation requires -review-draft; it never approves or publishes knowledge")
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	var cases struct {
		Version int              `json:"schema_version"`
		Items   []evaluationCase `json:"cases"`
	}
	if err := json.Unmarshal(data, &cases); err != nil {
		return err
	}
	if cases.Version != 1 || len(cases.Items) == 0 {
		return fmt.Errorf("invalid evaluation dataset")
	}
	pack, err := os.ReadFile(filepath.Join(dir, "knowledge.json"))
	if err != nil {
		return err
	}
	packHash, casesHash := sha256.Sum256(pack), sha256.Sum256(data)
	report := evaluationReport{Scope: "Synthetic offline retrieval only; no LLM, vision, live pricing or production calls",
		PackHash: hex.EncodeToString(packHash[:]), CasesHash: hex.EncodeToString(casesHash[:])}
	seen := map[string]bool{}
	for _, item := range cases.Items {
		if err := validateEvaluationCase(dir, item, seen); err != nil {
			return err
		}
		row, beforeImpl, afterImpl, err := compareEvaluationCase(dir, item)
		if err != nil {
			return err
		}
		report.addOutcome(row)
		report.BeforeImpl, report.AfterImpl = beforeImpl, afterImpl
		report.Results = append(report.Results, row)
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(report)
}

func hitIDs(hits []previewHit) []string {
	out := make([]string, 0, len(hits))
	for _, hit := range hits {
		out = append(out, hit.ID)
	}
	return out
}

func evaluationPass(item evaluationCase, ids []string) bool {
	if item.NoResults {
		return len(ids) == 0
	}
	for _, id := range ids {
		for _, expected := range item.Expected {
			if id == expected {
				return true
			}
		}
	}
	return false
}

func validateEvaluationCase(dir string, item evaluationCase, seen map[string]bool) error {
	if seen[item.ID] || item.ID == "" || item.Query == "" || item.Top < 1 || item.Top > 20 || (!item.NoResults && len(item.Expected) == 0) {
		return fmt.Errorf("invalid evaluation case %q", item.ID)
	}
	seen[item.ID] = true
	store, _, _, err := loadDraft(dir, item.Public)
	if err != nil {
		return err
	}
	eligible := map[string]bool{}
	for _, asset := range store.items {
		eligible[asset.ExternalID] = asset.Type != "banned_claim"
	}
	for _, id := range item.Expected {
		if !eligible[id] {
			return fmt.Errorf("case %q expects ineligible record %q", item.ID, id)
		}
	}
	return nil
}

func compareEvaluationCase(dir string, item evaluationCase) (evaluationResult, string, string, error) {
	before, err := previewWithLexical(context.Background(), dir, item.Query, true, item.Public, item.Top, true)
	if err != nil {
		return evaluationResult{}, "", "", err
	}
	after, err := preview(context.Background(), dir, item.Query, true, item.Public, item.Top)
	if err != nil {
		return evaluationResult{}, "", "", err
	}
	row := evaluationResult{Case: item, Before: hitIDs(before.Hits), After: hitIDs(after.Hits)}
	row.BeforeOK, row.AfterOK = evaluationPass(item, row.Before), evaluationPass(item, row.After)
	return row, before.SearcherImpl, after.SearcherImpl, nil
}

func (report *evaluationReport) addOutcome(row evaluationResult) {
	if row.BeforeOK {
		report.BeforePassed++
	}
	if row.AfterOK {
		report.AfterPassed++
	}
}
