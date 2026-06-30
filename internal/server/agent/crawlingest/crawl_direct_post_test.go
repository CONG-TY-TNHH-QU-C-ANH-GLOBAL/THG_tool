package crawlingest

import (
	"testing"

	"github.com/thg/scraper/internal/directpost"
	"github.com/thg/scraper/internal/store/coordination"
)

const (
	dpPostID    = "4506703172989187"
	dpGroup     = "ship.viet.my"
	dpCanonical = "https://www.facebook.com/groups/ship.viet.my/permalink/4506703172989187/"
	dpLossy     = "https://www.facebook.com/permalink.php?story_fbid=4506703172989187"
	dpForeignGr = "https://www.facebook.com/groups/1112083256270739/user/100029/"
	dpShipping  = "Em ở Q7 HCM cần gửi hàng đông lạnh tầm 22kg đi Texas ạ."
	dpJobs      = "Backend & Frontend Developer Vietnam (Jobs) [CLUEGA] TUYỂN DỤNG BACKEND ENGINEER (AI SAAS)"
)

func dpWF() *coordination.DirectPostCommentWorkflow {
	return &coordination.DirectPostCommentWorkflow{ID: 4, PostFBID: dpPostID, GroupRef: dpGroup, CanonicalPostURL: dpCanonical}
}

// E — a valid observed item (lossy URL, matching id, normal author, real shipping text)
// passes and the canonical URL is stamped (context preserved).
func TestValidateDirectPostObservedItem_ValidStampsCanonical(t *testing.T) {
	id, v := validateDirectPostObservedItem(dpWF(), directpost.ObservedItem{
		PostFBID: dpPostID, SourceURL: dpLossy,
		AuthorName: "Nhii Tran", AuthorProfileURL: "https://www.facebook.com/nhii.tran", Content: dpShipping,
	})
	if !v.Valid {
		t.Fatalf("valid item must pass, got %+v", v)
	}
	if id.primaryURL != dpCanonical || id.postFBID != dpPostID || id.groupRef != dpGroup {
		t.Errorf("valid item must stamp the canonical identity, got %+v", id)
	}
}

// A — no extractable post id → not the requested post, no override, no canonical stamp.
func TestValidateDirectPostObservedItem_MissingPostID(t *testing.T) {
	id, v := validateDirectPostObservedItem(dpWF(), directpost.ObservedItem{
		PostFBID: "", SourceURL: "https://www.facebook.com/groups/ship.viet.my/", Content: dpShipping,
	})
	if v.Valid || v.IdentityMatched {
		t.Fatalf("missing post id must not validate/identity-match, got %+v", v)
	}
	if id.primaryURL != "" {
		t.Errorf("no canonical stamp on unverified item, got %q", id.primaryURL)
	}
}

// B — different post id → not the requested post.
func TestValidateDirectPostObservedItem_DifferentPostID(t *testing.T) {
	_, v := validateDirectPostObservedItem(dpWF(), directpost.ObservedItem{
		PostFBID: "9999999999999999", SourceURL: dpLossy, Content: dpShipping,
	})
	if v.Valid || v.IdentityMatched || v.Reason != directpost.ReasonPostIDMismatch {
		t.Fatalf("different post id must be a non-matching rejection, got %+v", v)
	}
}

// C — matching id but a foreign-group author + jobs content → the requested post is
// poisoned: IdentityMatched, invalid, group conflict, no canonical stamp.
func TestValidateDirectPostObservedItem_GroupConflict(t *testing.T) {
	id, v := validateDirectPostObservedItem(dpWF(), directpost.ObservedItem{
		PostFBID: dpPostID, SourceURL: dpCanonical, GroupFBID: dpGroup,
		AuthorName: "Backend & Frontend Developer Vietnam (Jobs)", AuthorProfileURL: dpForeignGr, Content: dpJobs,
	})
	if !v.IdentityMatched || v.Valid || v.Reason != directpost.ReasonGroupConflict {
		t.Fatalf("foreign-group requested post must be identity-matched + invalid (group conflict), got %+v", v)
	}
	if id.primaryURL != "" {
		t.Errorf("no canonical stamp on a poisoned item, got %q", id.primaryURL)
	}
	if got := importContextMismatchCode(v.Reason); got != coordination.DPErrImportGroupMismatch {
		t.Errorf("group conflict must map to direct_post_import_group_mismatch, got %q", got)
	}
}

// TestImportContextMismatchCode and TestDirectPostImportFailureCode moved to
// crawl_direct_post_outcome_test.go alongside the helpers they prove.

// D — matching id but boilerplate content → poisoned (content invalid).
func TestValidateDirectPostObservedItem_Boilerplate(t *testing.T) {
	_, v := validateDirectPostObservedItem(dpWF(), directpost.ObservedItem{
		PostFBID: dpPostID, SourceURL: dpCanonical,
		AuthorName: "x", AuthorProfileURL: "https://www.facebook.com/nhii.tran", Content: "Facebook Facebook Facebook Facebook",
	})
	if !v.IdentityMatched || v.Valid || v.Reason != directpost.ReasonContentInvalid {
		t.Fatalf("boilerplate requested post must be identity-matched + content-invalid, got %+v", v)
	}
	if got := importContextMismatchCode(v.Reason); got != coordination.DPErrImportBoilerplateContent {
		t.Errorf("content invalid must map to direct_post_import_boilerplate_content, got %q", got)
	}
}
