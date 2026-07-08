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

// Enriched is the object-storage-key + cost view of a reel's "enriched"
// pipeline state (source clip -> HeyGen avatar -> Remotion compose). Its
// columns live on the reels row (migration 0112); this struct exists so the
// enriched accessors can read/write them without widening the base Reel
// scan used by the PR-R1 CRUD. All *Key fields are R2 object keys, never
// blobs. RenderIdempotencyKey is empty until a render is claimed.
type Enriched struct {
	ReelType             string  `json:"reel_type"`
	SourceKey            string  `json:"source_key"`
	InputBranch          string  `json:"input_branch"` // 'audio' | 'vision'
	AvatarKey            string  `json:"avatar_key"`
	FinalOutputKey       string  `json:"final_output_key"`
	TotalCostUSD         float64 `json:"total_cost_usd"`
	RenderIdempotencyKey string  `json:"render_idempotency_key"`
}

// TranscriptInput is the field bundle for CreateTranscript — grouped into a
// struct (rather than positional args) to keep the call under the max-param
// limit and make call sites self-documenting.
type TranscriptInput struct {
	Segments string
	LangSrc  string
	LangTgt  string
	Source   string
	CostUSD  float64
}

// Transcript is the understood content of a reel's source video plus the
// timing cues that let Remotion sync subtitles to speech. Segments is
// opaque JSON ([{text, from_ms, to_ms}]); the transcriber adapter owns its
// exact shape. Source is 'whisper' (audio branch) or 'vision' (silent).
type Transcript struct {
	ID        int64     `json:"id"`
	OrgID     int64     `json:"org_id"`
	ReelID    int64     `json:"reel_id"`
	Segments  string    `json:"segments"`
	LangSrc   string    `json:"lang_src"`
	LangTgt   string    `json:"lang_tgt"`
	Source    string    `json:"source"`
	CostUSD   float64   `json:"cost_usd"`
	CreatedAt time.Time `json:"created_at"`
}
