// Package brand owns brand/company identity, contact profiles, and personas — the
// verified facts (company identity, CTA assets) that outbound copy is grounded by.
//
// Architecture role: BRAND (domain) — see MODULE_BOUNDARIES.md (brand).
//
//   - Allowed imports (conceptual): models, its store domain, stdlib.
//   - Forbidden imports (conceptual): services/*, outbound, internal/server,
//     connectors. Message generation is ai's job, not brand's.
//
// SCAFFOLD ONLY (Phase A): boundary marker. Brand/contact data currently lives in
// internal/server/contactprofile and the staff_contact_profiles table. See
// MODULE_OWNERSHIP.yml.
package brand
