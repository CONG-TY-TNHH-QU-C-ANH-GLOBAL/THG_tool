package assets

import (
	"crypto/sha1"
	"encoding/hex"
	"sort"
	"strings"
)

// NormalizeTags canonicalizes a slice of free-form tags into the form
// the store + retrieval engine both rely on:
//
//   - whitespace trimmed
//   - lowercased
//   - deduplicated (case-insensitive)
//   - empty strings dropped
//   - sorted (stable order so JSON-compare equality works across re-ingests)
//
// Called by every ingestor before constructing an Asset, and by the
// Validate path as a defense-in-depth. The cost is O(n log n) which
// is negligible for the asset-tag scale (single-digit tags per asset).
func NormalizeTags(raw []string) []string {
	if len(raw) == 0 {
		return []string{}
	}
	seen := make(map[string]struct{}, len(raw))
	out := make([]string, 0, len(raw))
	for _, t := range raw {
		clean := strings.ToLower(strings.TrimSpace(t))
		if clean == "" {
			continue
		}
		if _, ok := seen[clean]; ok {
			continue
		}
		seen[clean] = struct{}{}
		out = append(out, clean)
	}
	sort.Strings(out)
	return out
}

// ContentFingerprint returns a stable hash of an asset's content
// fields — title + description + sorted tags + payload. Used by
// ingestors that read sources without stable external IDs (e.g. CSV
// uploads where row order is not guaranteed) to dedupe on content.
//
// Two assets with the same fingerprint are the same asset for
// idempotent-ingest purposes. Two assets with different fingerprints
// are different assets even if their titles match.
//
// SHA-1 is intentional: not for cryptographic strength but for a
// short, well-distributed hex string that fits in a TEXT column
// without compression headaches. If a future ingestor needs collision
// resistance against an adversary, switch to SHA-256 here — callers
// treat the return value as opaque.
func ContentFingerprint(title, description string, tags []string, payload []byte) string {
	h := sha1.New()
	_, _ = h.Write([]byte(strings.TrimSpace(title)))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(strings.TrimSpace(description)))
	_, _ = h.Write([]byte{0})
	for _, t := range NormalizeTags(tags) {
		_, _ = h.Write([]byte(t))
		_, _ = h.Write([]byte{0})
	}
	_, _ = h.Write(payload)
	return hex.EncodeToString(h.Sum(nil))
}
