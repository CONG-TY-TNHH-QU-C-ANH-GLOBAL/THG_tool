package main

import (
	"testing"

	"github.com/thg/scraper/internal/directpost"
	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store/coordination"
)

// F — the pre-comment guard blocks an already-poisoned DB lead even though its source_url
// equals the workflow canonical (so the P1.1 strict lookup matched it). This reproduces
// the production incident lead #313: ship.viet.my canonical URL stamped onto Backend-Jobs
// content with a foreign-group author.
func TestDirectPostLeadTargetMismatch(t *testing.T) {
	wf := &coordination.DirectPostCommentWorkflow{
		ID: 4, OrgID: 5, PostFBID: "4506703172989187", GroupRef: "ship.viet.my",
		CanonicalPostURL: "https://www.facebook.com/groups/ship.viet.my/permalink/4506703172989187/",
	}

	// Poisoned: foreign-group author_url (the incident).
	poisoned := &models.Lead{
		SourceType: "post",
		SourceURL:  wf.CanonicalPostURL, // matches canonical → P1.1 lookup would accept it
		Author:     "Backend & Frontend Developer Vietnam (Jobs)",
		AuthorURL:  "https://www.facebook.com/groups/1112083256270739/",
		Content:    "[CLUEGA] TUYỂN DỤNG BACKEND ENGINEER (AI SAAS / MCP ADS SYSTEM) cần backend mạnh",
	}
	if reason, blocked := directPostLeadTargetMismatch(wf, poisoned); !blocked || reason != directpost.ReasonGroupConflict {
		t.Fatalf("foreign-group lead must be blocked with group conflict, got blocked=%v reason=%q", blocked, reason)
	}

	// Poisoned: boilerplate content.
	boiler := &models.Lead{
		SourceType: "post", SourceURL: wf.CanonicalPostURL,
		Author: "x", AuthorURL: "https://www.facebook.com/nhii.tran",
		Content: "Facebook Facebook Facebook Facebook",
	}
	if reason, blocked := directPostLeadTargetMismatch(wf, boiler); !blocked || reason != directpost.ReasonContentInvalid {
		t.Fatalf("boilerplate lead must be blocked as content invalid, got blocked=%v reason=%q", blocked, reason)
	}

	// Clean: real shipping post from a normal user → NOT blocked.
	clean := &models.Lead{
		SourceType: "post", SourceURL: wf.CanonicalPostURL,
		Author: "Nhii Tran", AuthorURL: "https://www.facebook.com/nhii.tran",
		Content: "Em ở Q7 HCM cần gửi hàng đông lạnh tầm 22kg đi Texas ạ.",
	}
	if _, blocked := directPostLeadTargetMismatch(wf, clean); blocked {
		t.Fatal("a clean shipping lead must not be blocked")
	}
}
