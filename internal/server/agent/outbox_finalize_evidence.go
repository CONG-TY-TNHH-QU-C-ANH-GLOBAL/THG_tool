package agent

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/thg/scraper/internal/runtime"
	"github.com/thg/scraper/internal/store/coordination"
)

// Evidence + proof adapters for outbound finalization: decode/persist the
// extension's out-of-band failure screenshot and translate the runtime verifier
// proof onto the coordination evidence shape. Pure helpers — no Handler state.

// maxEvidenceScreenshotBytes bounds a decoded evidence screenshot before it
// touches disk. A q40 JPEG of a 1080p tab is ~80–160 KB; 1 MB is a generous
// ceiling that still rejects an extension shipping a runaway payload.
const maxEvidenceScreenshotBytes = 1 << 20 // 1 MiB

// persistEvidenceScreenshot decodes the extension's out-of-band base64 JPEG to
// an ORG-SCOPED file under data/evidence/<orgID>/ and returns the relative path
// to record in NavDiagnostic.ScreenshotPath. The bytes never enter evidence_json.
//
// Tenant safety: every path component is server-derived (orgID, outboundID,
// attemptID are internal ids issued in org-scoped txs) — no extension-supplied
// string reaches the filename, so this cannot be steered out of the org dir.
// Best-effort: any failure returns "" with a logged warning; evidence capture
// must never block the terminal callback.
func persistEvidenceScreenshot(orgID, outboundID, attemptID int64, b64 string) (string, error) {
	b64 = strings.TrimSpace(b64)
	// Accept a data: URL prefix or raw base64.
	if i := strings.Index(b64, ","); strings.HasPrefix(b64, "data:") && i >= 0 {
		b64 = b64[i+1:]
	}
	if b64 == "" {
		return "", nil
	}
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return "", fmt.Errorf("decode evidence screenshot: %w", err)
	}
	if len(raw) == 0 || len(raw) > maxEvidenceScreenshotBytes {
		return "", fmt.Errorf("evidence screenshot size out of bounds: %d bytes", len(raw))
	}
	dir := filepath.Join("data", "evidence", strconv.FormatInt(orgID, 10))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("mkdir evidence dir: %w", err)
	}
	name := fmt.Sprintf("ob%d-att%d-%d.jpg", outboundID, attemptID, time.Now().UTC().Unix())
	rel := filepath.ToSlash(filepath.Join(dir, name))
	if err := os.WriteFile(filepath.FromSlash(rel), raw, 0o644); err != nil {
		return "", fmt.Errorf("write evidence screenshot: %w", err)
	}
	return rel, nil
}

// proofToEvidence adapts the runtime verifier's proof shape onto the
// coordination domain's evidence shape. Two types exist (instead of
// one shared) to avoid an import cycle: runtime cannot import store,
// and coordination cannot import runtime. The fields are 1:1 today;
// if they diverge, this is the seam to translate.
func proofToEvidence(p runtime.VerifierProof) coordination.VerificationEvidence {
	return coordination.VerificationEvidence{
		CommentPermalink: p.CommentPermalink,
		MessageBubbleID:  p.MessageBubbleID,
		DOMSnippet:       p.DOMSnippet,
		PageURLAfter:     p.PageURLAfter,
		ObservedAt:       p.ObservedAt,
		Notes:            p.Notes,
		NavDiagnostic:    p.NavDiagnostic, // PR8A: structured landing telemetry → evidence_json
	}
}
