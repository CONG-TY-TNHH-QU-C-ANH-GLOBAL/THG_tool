// Tenant-isolation regression tests for the crawl subpackage.
//
// These lock in the security fix that scoped DeletePost / DeleteAllPostsForOrg
// / ToggleGroup / DeleteGroup by org_id. Before the fix these store methods
// operated on a bare id and let one tenant mutate another tenant's data.
package crawl_test

import (
	"testing"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store"
	"github.com/thg/scraper/internal/store/crawl"
	"github.com/thg/scraper/internal/store/storetest"
)

func bootstrapStore(path string) error {
	db, err := store.New(path)
	if err != nil {
		return err
	}
	return db.Close()
}

func newCrawlStore(t *testing.T, name string) *crawl.Store {
	t.Helper()
	dst := storetest.CopyTemplate(t, bootstrapStore, name)
	db, err := store.New(dst)
	if err != nil {
		t.Fatalf("open from template: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db.Crawl()
}

func addGroup(t *testing.T, s *crawl.Store, orgID int64, url string) int64 {
	t.Helper()
	id, err := s.AddGroup(&models.Group{
		OrgID:     orgID,
		Platform:  models.Platform("facebook"),
		Name:      "g-" + url,
		URL:       url,
		Active:    true,
		JoinState: "none",
	})
	if err != nil {
		t.Fatalf("AddGroup(org=%d): %v", orgID, err)
	}
	return id
}

func addPost(t *testing.T, s *crawl.Store, groupID int64, dedup string) int64 {
	t.Helper()
	id, err := s.InsertPost(&models.Post{
		Platform:  models.Platform("facebook"),
		GroupID:   groupID,
		URL:       "https://example.com/p/" + dedup,
		Content:   "post " + dedup,
		DedupHash: dedup,
	})
	if err != nil {
		t.Fatalf("InsertPost(group=%d): %v", groupID, err)
	}
	return id
}

func TestDeleteGroup_IsOrgScoped(t *testing.T) {
	s := newCrawlStore(t, "crawl-delgroup.db")
	const orgA, orgB int64 = 1, 2
	groupB := addGroup(t, s, orgB, "https://fb.com/groups/b")

	// orgA must not be able to delete orgB's group.
	if err := s.DeleteGroup(orgA, groupB); err != nil {
		t.Fatalf("DeleteGroup cross-org returned error: %v", err)
	}
	if groups, _ := s.GetAllGroups(orgB); len(groups) != 1 {
		t.Fatalf("cross-org DeleteGroup leaked: orgB groups = %d, want 1", len(groups))
	}

	// The owner can delete it.
	if err := s.DeleteGroup(orgB, groupB); err != nil {
		t.Fatalf("DeleteGroup owner: %v", err)
	}
	if groups, _ := s.GetAllGroups(orgB); len(groups) != 0 {
		t.Fatalf("owner DeleteGroup failed: orgB groups = %d, want 0", len(groups))
	}
}

func TestToggleGroup_IsOrgScoped(t *testing.T) {
	s := newCrawlStore(t, "crawl-toggle.db")
	const orgA, orgB int64 = 1, 2
	groupB := addGroup(t, s, orgB, "https://fb.com/groups/b")

	if err := s.ToggleGroup(orgA, groupB, false); err != nil {
		t.Fatalf("ToggleGroup cross-org returned error: %v", err)
	}
	groups, _ := s.GetAllGroups(orgB)
	if len(groups) != 1 || !groups[0].Active {
		t.Fatalf("cross-org ToggleGroup mutated orgB group active=%v, want true", groups[0].Active)
	}

	if err := s.ToggleGroup(orgB, groupB, false); err != nil {
		t.Fatalf("ToggleGroup owner: %v", err)
	}
	groups, _ = s.GetAllGroups(orgB)
	if groups[0].Active {
		t.Fatalf("owner ToggleGroup failed: group still active")
	}
}

func TestDeletePost_IsOrgScoped(t *testing.T) {
	s := newCrawlStore(t, "crawl-delpost.db")
	const orgA, orgB int64 = 1, 2
	groupB := addGroup(t, s, orgB, "https://fb.com/groups/b")
	postB := addPost(t, s, groupB, "hashB")

	if err := s.DeletePost(orgA, postB); err != nil {
		t.Fatalf("DeletePost cross-org returned error: %v", err)
	}
	if posts, _ := s.GetRecentPosts(50, 0, orgB); len(posts) != 1 {
		t.Fatalf("cross-org DeletePost leaked: orgB posts = %d, want 1", len(posts))
	}

	if err := s.DeletePost(orgB, postB); err != nil {
		t.Fatalf("DeletePost owner: %v", err)
	}
	if posts, _ := s.GetRecentPosts(50, 0, orgB); len(posts) != 0 {
		t.Fatalf("owner DeletePost failed: orgB posts = %d, want 0", len(posts))
	}
}

func TestDeleteAllPostsForOrg_OnlyDeletesOwnTenant(t *testing.T) {
	s := newCrawlStore(t, "crawl-delall.db")
	const orgA, orgB int64 = 1, 2
	groupA := addGroup(t, s, orgA, "https://fb.com/groups/a")
	groupB := addGroup(t, s, orgB, "https://fb.com/groups/b")
	addPost(t, s, groupA, "hashA")
	addPost(t, s, groupB, "hashB")

	n, err := s.DeleteAllPostsForOrg(orgA)
	if err != nil {
		t.Fatalf("DeleteAllPostsForOrg: %v", err)
	}
	if n != 1 {
		t.Fatalf("DeleteAllPostsForOrg removed %d, want 1", n)
	}
	if posts, _ := s.GetRecentPosts(50, 0, orgA); len(posts) != 0 {
		t.Fatalf("orgA posts not cleared: %d", len(posts))
	}
	if posts, _ := s.GetRecentPosts(50, 0, orgB); len(posts) != 1 {
		t.Fatalf("orgB posts wrongly affected: %d, want 1", len(posts))
	}
}
