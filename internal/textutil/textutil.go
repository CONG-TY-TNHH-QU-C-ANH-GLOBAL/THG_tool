// Package textutil holds tiny string helpers that were copy-pasted across
// 7+ packages before consolidation. Keep this package a true leaf — no
// imports outside the standard library — so anyone (recovery, runtime,
// leadingest, ai, server/*, cmd/*) can depend on it without cycles.
package textutil

import "strings"

// ContainsAny reports whether s contains any of the given substrings. Matches
// the behaviour of the previous duplicates in recovery, runtime/antidetect,
// and server/workspace/watchers.
func ContainsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if sub == "" {
			continue
		}
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// FirstNonEmpty returns the first value whose TrimSpace is non-empty,
// already trimmed. Replaces the four copies of firstNonEmpty / firstNonEmptyBrain.
func FirstNonEmpty(values ...string) string {
	for _, v := range values {
		if t := strings.TrimSpace(v); t != "" {
			return t
		}
	}
	return ""
}
