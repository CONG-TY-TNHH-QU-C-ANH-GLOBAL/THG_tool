package crawl

import (
	"crypto/sha256"
	"encoding/hex"

	"github.com/thg/scraper/internal/models"
)

// InsertPost inserts a post if it does not already exist by dedup_hash.
func (s *Store) InsertPost(p *models.Post) (int64, error) {
	res, err := s.db.Exec(
		`INSERT OR IGNORE INTO posts (platform, group_id, group_name, url, author, author_url, author_avatar, content, images, reactions, comments, posted_at, dedup_hash)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.Platform, p.GroupID, p.GroupName, p.URL, p.Author, p.AuthorURL, p.AuthorAvatar,
		p.Content, p.Images, p.Reactions, p.Comments, p.PostedAt, p.DedupHash,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// GetRecentPosts returns recent posts with pagination. orgID=0 returns
// all (admin-only callers; HTTP handlers always pass a positive org).
// Intra-domain JOIN (posts ↔ groups, both crawl-owned).
func (s *Store) GetRecentPosts(limit, offset int, orgID int64) ([]models.Post, error) {
	q := `SELECT p.id, p.platform, p.group_id, p.group_name, p.url, p.author, p.author_url, p.author_avatar, p.content, p.images, p.reactions, p.comments, p.posted_at, p.scraped_at, p.dedup_hash
		 FROM posts p`
	var args []any
	if orgID > 0 {
		q += ` JOIN groups g ON p.group_id = g.id WHERE g.org_id = ?`
		args = append(args, orgID)
	}
	q += ` ORDER BY p.scraped_at DESC LIMIT ? OFFSET ?`
	args = append(args, limit, offset)

	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var posts []models.Post
	for rows.Next() {
		var p models.Post
		if err := rows.Scan(&p.ID, &p.Platform, &p.GroupID, &p.GroupName, &p.URL, &p.Author, &p.AuthorURL, &p.AuthorAvatar, &p.Content, &p.Images, &p.Reactions, &p.Comments, &p.PostedAt, &p.ScrapedAt, &p.DedupHash); err != nil {
			return nil, err
		}
		posts = append(posts, p)
	}
	return posts, nil
}

// DeletePost removes a post by ID, scoped to the owning org.
//
// Posts have no org_id column; ownership is derived from the parent group
// (posts.group_id → groups.org_id). A postID belonging to another tenant is
// a silent no-op.
func (s *Store) DeletePost(orgID, postID int64) error {
	_, err := s.db.Exec(
		`DELETE FROM posts WHERE id = ? AND group_id IN (SELECT id FROM groups WHERE org_id = ?)`,
		postID, orgID,
	)
	return err
}

// DeleteAllPostsForOrg removes all posts owned by a single tenant and keeps
// groups. Scoped via the parent group (posts.group_id → groups.org_id) so it
// never wipes another tenant's posts.
func (s *Store) DeleteAllPostsForOrg(orgID int64) (int64, error) {
	result, err := s.db.Exec(
		`DELETE FROM posts WHERE group_id IN (SELECT id FROM groups WHERE org_id = ?)`,
		orgID,
	)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// InsertComment inserts a comment if it does not already exist.
func (s *Store) InsertComment(c *models.Comment) (int64, error) {
	res, err := s.db.Exec(
		`INSERT OR IGNORE INTO comments (post_id, platform, author, author_url, content, posted_at, dedup_hash)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		c.PostID, c.Platform, c.Author, c.AuthorURL, c.Content, c.PostedAt, c.DedupHash,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// InsertPostsBatch inserts a slice of posts from an agent result,
// skipping duplicates.
func (s *Store) InsertPostsBatch(posts []models.Post) (int, error) {
	saved := 0
	for i := range posts {
		p := &posts[i]
		if p.DedupHash == "" {
			h := sha256.Sum256([]byte(p.Content + p.AuthorURL))
			p.DedupHash = hex.EncodeToString(h[:])
		}
		_, err := s.db.Exec(
			`INSERT OR IGNORE INTO posts (platform, group_id, group_name, url, author, author_url, content, images, reactions, comments, posted_at, scraped_at, dedup_hash)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, ?)`,
			p.Platform, p.GroupID, p.GroupName, p.URL, p.Author, p.AuthorURL,
			p.Content, p.Images, p.Reactions, p.Comments, p.PostedAt, p.DedupHash,
		)
		if err == nil {
			saved++
		}
	}
	return saved, nil
}
