package security

import (
	"regexp"
	"strings"
)

// Goal G8 second half: secret redaction in traces.
//
// The Operator Replay surface shows trace data including the lead
// content. If a lead body (or anything that flows into a trace)
// contains a secret-shaped string — API key, JWT, AWS access key,
// password — we MUST redact it before persisting or rendering.
//
// Production reality: operators paste real customer messages into
// our system. Customers occasionally paste API keys by mistake. The
// replay surface is shared (other operators view it); a leaked key
// in the replay table is a data-protection incident.
//
// Redaction is one-way: we replace matches with `[REDACTED:KIND]`
// where KIND identifies the secret class. Operators see WHICH kind
// of secret was redacted (helpful for "tell customer not to share
// production keys"), but not the value itself.

// secretPattern is a labelled detector.
type secretPattern struct {
	kind string
	re   *regexp.Regexp
}

// Conservative, false-positive-tolerating patterns. We prefer over-
// redacting (false positive = "REDACTED" on innocuous text) over
// under-redacting (false negative = a real secret survives).
//
// Patterns are simple enough to read; high-precision matchers (e.g.
// AWS-secret-key entropy detection) are not worth the complexity
// at this stage.
var secretPatterns = []secretPattern{
	{"openai", regexp.MustCompile(`sk-[A-Za-z0-9]{20,}`)},
	{"aws_access_key", regexp.MustCompile(`AKIA[0-9A-Z]{16}`)},
	{"aws_secret", regexp.MustCompile(`(?i)aws[-_]?secret[-_]?(access[-_]?)?key\s*[:=]\s*["']?[A-Za-z0-9/+=]{30,}["']?`)},
	{"jwt", regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{10,}\.eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\b`)},
	{"private_key_block", regexp.MustCompile(`(?s)-----BEGIN [A-Z ]+ PRIVATE KEY-----.*?-----END [A-Z ]+ PRIVATE KEY-----`)},
	{"bearer_token", regexp.MustCompile(`(?i)bearer\s+[A-Za-z0-9._\-]{20,}`)},
	{"github_pat", regexp.MustCompile(`gh[ps]_[A-Za-z0-9]{30,}`)},
	{"stripe_key", regexp.MustCompile(`(sk|pk)_(live|test)_[A-Za-z0-9]{20,}`)},
	{"slack_token", regexp.MustCompile(`xox[abprs]-[A-Za-z0-9-]{10,}`)},
}

// Redact replaces every secret-shaped substring with a
// `[REDACTED:kind]` placeholder. Safe to call on empty strings.
//
// Idempotent: calling Redact on already-redacted output is a no-op.
func Redact(s string) string {
	if s == "" {
		return s
	}
	out := s
	for _, p := range secretPatterns {
		out = p.re.ReplaceAllString(out, "[REDACTED:"+p.kind+"]")
	}
	return out
}

// RedactedAny reports whether Redact would CHANGE s. Useful for
// callers that want to add a finding to a trace without paying for
// the actual replacement when no secret is present.
func RedactedAny(s string) bool {
	if s == "" {
		return false
	}
	for _, p := range secretPatterns {
		if p.re.MatchString(s) {
			return true
		}
	}
	return false
}

// Compile-time assurance: the patterns compile. If a future
// contributor adds a malformed pattern, init order ensures the
// program fails to start rather than running with a broken matcher.
var _ = func() bool {
	for _, p := range secretPatterns {
		if p.re == nil {
			panic("security: nil secret pattern for " + p.kind)
		}
		_ = strings.TrimSpace(p.kind)
	}
	return true
}()
