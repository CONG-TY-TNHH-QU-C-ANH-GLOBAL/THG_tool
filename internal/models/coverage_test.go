package models

import (
	"testing"
	"time"
)

func TestEvaluateCoverage(t *testing.T) {
	pol := DefaultCoveragePolicy()
	now := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)

	// Multi-actor coverage allowed by default: a fresh actor on an untouched lead is eligible.
	if ok, r := EvaluateCoverage(LeadCoverageState{}, pol, 7, now); !ok || r != CoverageOK {
		t.Errorf("fresh actor should be eligible, got ok=%v r=%s", ok, r)
	}
	// A SECOND, different actor (1 prior touch, gap satisfied) is eligible (brand coverage).
	st := LeadCoverageState{ActorsTouched: []int64{1}, OrgTouchCount: 1, LastTouchAt: now.Add(-time.Hour)}
	if ok, r := EvaluateCoverage(st, pol, 2, now); !ok || r != CoverageOK {
		t.Errorf("second actor within policy should be eligible, got ok=%v r=%s", ok, r)
	}
	// Same actor again → skip.
	if ok, r := EvaluateCoverage(st, pol, 1, now); ok || r != CoverageAlreadyThisActor {
		t.Errorf("same actor must skip, got ok=%v r=%s", ok, r)
	}
	// Lead replied → stop.
	if _, r := EvaluateCoverage(LeadCoverageState{LeadReplied: true}, pol, 9, now); r != CoverageLeadReplied {
		t.Errorf("replied lead must stop, got %s", r)
	}
	// Coverage full (2 accounts already).
	full := LeadCoverageState{ActorsTouched: []int64{1, 2}, OrgTouchCount: 2, LastTouchAt: now.Add(-time.Hour)}
	if _, r := EvaluateCoverage(full, pol, 3, now); r != CoverageFull {
		t.Errorf("full coverage must skip, got %s", r)
	}
	// Too soon after the previous actor.
	soon := LeadCoverageState{ActorsTouched: []int64{1}, OrgTouchCount: 1, LastTouchAt: now.Add(-5 * time.Minute)}
	if _, r := EvaluateCoverage(soon, pol, 2, now); r != CoverageGapTooSoon {
		t.Errorf("too-soon must skip, got %s", r)
	}
	// Single-actor policy: another touch blocks a second actor.
	single := pol
	single.AllowMultiActorCoverage = false
	if _, r := EvaluateCoverage(LeadCoverageState{OrgTouchCount: 1}, single, 2, now); r != CoverageSingleActorPolicy {
		t.Errorf("single-actor policy must skip a second actor, got %s", r)
	}
}

func TestDeriveActorPersona(t *testing.T) {
	pol := DefaultCoveragePolicy()
	// Fresh lead: the actor may use the website + a direct CTA.
	p := DeriveActorPersona(LeadCoverageState{}, pol, "sourcing specialist", "warm")
	if p.LinkPolicy != LinkMayIncludeWebsite || p.AllowedCTAStyle != CTADirectInbox {
		t.Errorf("fresh persona should allow website + direct CTA, got %+v", p)
	}
	// Website + CTA already used → next actor: no link, experience-share, avoid used angles.
	st := LeadCoverageState{WebsiteAlreadyUsed: true, DirectCTAAlreadyUsed: true, UsedAngles: []string{"price_focus"}}
	p2 := DeriveActorPersona(st, pol, "fulfillment advisor", "consultative")
	if p2.LinkPolicy != LinkNoLink || p2.AllowedCTAStyle != CTAExperienceShare {
		t.Errorf("covered lead should force no_link + experience_share, got %+v", p2)
	}
	if len(p2.ForbiddenRepeatedPhrases) != 1 || p2.ForbiddenRepeatedPhrases[0] != "price_focus" {
		t.Errorf("persona must forbid used angles, got %v", p2.ForbiddenRepeatedPhrases)
	}
}
