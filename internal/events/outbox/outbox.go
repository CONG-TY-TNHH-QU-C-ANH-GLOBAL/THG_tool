// Package outbox defines the transactional-outbox event contract: the durable
// event envelope + the critical event type names + relay status values
// (docs/architecture/TRANSACTIONAL_OUTBOX.md).
//
// SCAFFOLD (Phase E): TYPES ONLY. There is NO outbox table, NO relay loop, and NO
// producer wiring in this commit — adding the schema is explicitly gated (the
// sprint forbids silent migrations). These pure types let producers/consumers be
// written against a stable contract first; the table + relay land in a later,
// explicitly-approved additive migration.
//
// Pure: imports stdlib only (time). No store, no service, no driver — events carry
// data, not behavior. A "when X then Y" reaction is a process manager in the owning
// SERVICE that subscribes to these events, never logic in this package.
package outbox

import "time"

// EventType is a stable contract string for a critical cross-module event. Changing
// an existing value breaks consumers; add a new suffixed type (e.g. ".v2") instead.
type EventType string

// Critical events that MUST flow through the durable outbox (never an ad-hoc
// in-memory callback). See TRANSACTIONAL_OUTBOX.md §8.
const (
	FacebookLeadCreated        EventType = "FacebookLeadCreated"
	FacebookLeadScored         EventType = "FacebookLeadScored"
	FacebookPostImported       EventType = "FacebookPostImported"
	CommentActionPlanned       EventType = "CommentActionPlanned"
	CommentActionPosted        EventType = "CommentActionPosted"
	ConnectorChallengeRequired EventType = "ConnectorChallengeRequired"
	ConnectorReadyChanged      EventType = "ConnectorReadyChanged"
)

// Status is the relay delivery state of an outbox row.
type Status string

const (
	StatusPending   Status = "pending"   // awaiting delivery (or backoff)
	StatusPublished Status = "published" // delivered to subscribers
	StatusFailed    Status = "failed"    // transient failure, will retry
	StatusDead      Status = "dead"      // exceeded max attempts; alert, never silently dropped
)

// Envelope is the durable event written to the outbox table IN THE SAME
// TRANSACTION as the state change that produced it. `Data` carries ids/references
// only — NEVER secrets (cookies/tokens/session) or unnecessary PII.
type Envelope struct {
	EventID     string         `json:"event_id"`  // ULID/UUID — the idempotency key
	Type        EventType      `json:"type"`      // stable contract name
	OrgID       int64          `json:"org_id"`    // tenant scope (mandatory); a consumer acts only within it
	Aggregate   string         `json:"aggregate"` // e.g. "lead", "outbound_message"
	AggregateID string         `json:"aggregate_id"`
	OccurredAt  time.Time      `json:"occurred_at"`
	TraceID     string         `json:"trace_id,omitempty"`
	Data        map[string]any `json:"data"` // ids/references only — NO secrets
}
