// Domain: connectors (see internal/store/DOMAINS.md)
package connectors

import (
	"database/sql"
	"strings"
)

// Extension version states (closed set — SaaS UX Hardening PR-4).
// latest + update_available allow automation; update_required +
// unsupported block it.
const (
	VersionStateLatest          = "latest"
	VersionStateUpdateAvailable = "update_available"
	VersionStateUpdateRequired  = "update_required"
	VersionStateUnsupported     = "unsupported"
)

// Connector-eligibility reasons emitted by the version gate.
const (
	ConnExtensionUpdateRequired = "extension_update_required"
	ConnExtensionUnsupported    = "extension_unsupported"
)

// VersionPolicy is the platform extension policy (extension_policies
// row id=1, falling back to DefaultVersionPolicy when absent/blank).
type VersionPolicy struct {
	LatestVersion       string `json:"latest_version"`
	MinSupportedVersion string `json:"min_supported_version"`
	MinRequiredVersion  string `json:"min_required_version"`
	ReleaseChannel      string `json:"release_channel"`
	ReleaseNotes        string `json:"release_notes"`
	UpdateURL           string `json:"update_url"`
	UpdateInstructions  string `json:"update_instructions"`
	ForceUpdateAfter    string `json:"force_update_after,omitempty"`
}

// DefaultVersionPolicy preserves the pre-policy behavior: the compiled
// MinExtensionVersion is both the supported and the required floor.
func DefaultVersionPolicy() VersionPolicy {
	return VersionPolicy{
		MinSupportedVersion: MinExtensionVersion,
		MinRequiredVersion:  MinExtensionVersion,
		ReleaseChannel:      "stable",
	}
}

// EvaluateVersionState classifies a reported extension version against
// the policy. PURE. Unknown/empty versions are unsupported (an
// incompatible or pre-reporting build must never run automation).
func EvaluateVersionState(version string, p VersionPolicy) string {
	v := strings.TrimSpace(version)
	if v == "" {
		return VersionStateUnsupported
	}
	if p.MinSupportedVersion != "" && !versionAtLeast(v, p.MinSupportedVersion) {
		return VersionStateUnsupported
	}
	if p.MinRequiredVersion != "" && !versionAtLeast(v, p.MinRequiredVersion) {
		return VersionStateUpdateRequired
	}
	if p.LatestVersion != "" && !versionAtLeast(v, p.LatestVersion) {
		return VersionStateUpdateAvailable
	}
	return VersionStateLatest
}

// VersionStateAllowsAutomation: latest/update_available run (the
// latter with a soft warning); update_required/unsupported are blocked.
func VersionStateAllowsAutomation(state string) bool {
	return state == VersionStateLatest || state == VersionStateUpdateAvailable
}

// VersionStateReason maps a blocking state to its connector reason
// code; empty for non-blocking states.
func VersionStateReason(state string) string {
	switch state {
	case VersionStateUpdateRequired:
		return ConnExtensionUpdateRequired
	case VersionStateUnsupported:
		return ConnExtensionUnsupported
	}
	return ""
}

// GetExtensionPolicy loads the platform policy row, blank fields and a
// missing row fall back to DefaultVersionPolicy values — the gate is
// never silently disabled by configuration absence.
func (s *Store) GetExtensionPolicy() (VersionPolicy, error) {
	p := DefaultVersionPolicy()
	var forceAfter sql.NullString
	err := s.db.QueryRow(
		`SELECT latest_version, min_supported_version, min_required_version,
		        release_channel, release_notes, update_url, update_instructions,
		        COALESCE(force_update_after, '')
		   FROM extension_policies WHERE id = 1`,
	).Scan(&p.LatestVersion, &p.MinSupportedVersion, &p.MinRequiredVersion,
		&p.ReleaseChannel, &p.ReleaseNotes, &p.UpdateURL, &p.UpdateInstructions, &forceAfter)
	if err == sql.ErrNoRows {
		return DefaultVersionPolicy(), nil
	}
	if err != nil {
		return DefaultVersionPolicy(), err
	}
	d := DefaultVersionPolicy()
	if strings.TrimSpace(p.MinSupportedVersion) == "" {
		p.MinSupportedVersion = d.MinSupportedVersion
	}
	if strings.TrimSpace(p.MinRequiredVersion) == "" {
		p.MinRequiredVersion = d.MinRequiredVersion
	}
	if strings.TrimSpace(p.ReleaseChannel) == "" {
		p.ReleaseChannel = d.ReleaseChannel
	}
	p.ForceUpdateAfter = forceAfter.String
	return p, nil
}

// UpsertExtensionPolicy writes the single platform policy row.
func (s *Store) UpsertExtensionPolicy(p VersionPolicy) error {
	var forceAfter any
	if strings.TrimSpace(p.ForceUpdateAfter) != "" {
		forceAfter = p.ForceUpdateAfter
	}
	_, err := s.db.Exec(
		`INSERT INTO extension_policies
		   (id, latest_version, min_supported_version, min_required_version,
		    release_channel, release_notes, update_url, update_instructions,
		    force_update_after, updated_at)
		 VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		 ON CONFLICT(id) DO UPDATE SET
		   latest_version = excluded.latest_version,
		   min_supported_version = excluded.min_supported_version,
		   min_required_version = excluded.min_required_version,
		   release_channel = excluded.release_channel,
		   release_notes = excluded.release_notes,
		   update_url = excluded.update_url,
		   update_instructions = excluded.update_instructions,
		   force_update_after = excluded.force_update_after,
		   updated_at = CURRENT_TIMESTAMP`,
		p.LatestVersion, p.MinSupportedVersion, p.MinRequiredVersion,
		p.ReleaseChannel, p.ReleaseNotes, p.UpdateURL, p.UpdateInstructions, forceAfter,
	)
	return err
}
