package reel

import "time"

// Reel is one video task: a brief, its lifecycle status, and ownership.
type Reel struct {
	ID        int64     `json:"id"`
	OrgID     int64     `json:"org_id"`
	Title     string    `json:"title"`
	Brief     string    `json:"brief"`
	Status    string    `json:"status"` // draft|scripting|approved|rendering|done|failed
	CreatedBy int64     `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Script is one versioned draft of a reel's dialogue/shot-list/caption.
// Content is opaque JSON — the script engine (PR-R2+) owns its shape.
type Script struct {
	ID        int64     `json:"id"`
	OrgID     int64     `json:"org_id"`
	ReelID    int64     `json:"reel_id"`
	Version   int       `json:"version"`
	Content   string    `json:"content"`
	Approved  bool      `json:"approved"`
	CreatedAt time.Time `json:"created_at"`
}
