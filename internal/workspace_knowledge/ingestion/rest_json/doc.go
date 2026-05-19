// Package rest_json is the generic REST/JSON product-catalog adapter.
//
// A tenant configures one [sources.Source] of type [sources.SourceRESTJSON]
// per catalog backend they want to connect. The connection_config JSON
// describes:
//
//   - the HTTP endpoint and pagination scheme,
//   - the auth model (none / bearer / custom header),
//   - the field_map that translates upstream JSON keys into
//     [products.CanonicalProduct] fields,
//   - an availability mapping (upstream value → canonical Availability),
//   - request-level timeouts and a User-Agent.
//
// This adapter has zero hardcoded tenant knowledge. It is the same code
// path whether a workspace points it at a Shopify-style endpoint, a
// custom internal hub, or any other JSON-returning catalog. The
// product platform ships one tenant-agnostic adapter; tenant-specific
// "presets" (saved field_maps that operators can pick instead of
// filling each key by hand) live as data, not code.
//
// # Responsibilities
//
//   - Paginate through the endpoint until the configured limit is hit
//     or the upstream signals exhaustion.
//   - For each item, apply the field_map, map availability, compute a
//     per-item SHA-256 hash of the raw bytes (replay lineage), and
//     produce one [products.CanonicalProduct].
//   - Normalise + validate via [products.Writer]; failed rows surface
//     as [ingestion.SyncError] entries while the rest of the sync
//     continues.
//   - Classify HTTP errors into recoverable (5xx, network blips, 429)
//     vs permanent (auth, schema drift, 4xx other than 429) using the
//     existing [ingestion.WrapRecoverable] / [ingestion.WrapPermanent]
//     helpers so the dispatcher's health mapping works unchanged.
//
// # Not in scope (v1)
//
//   - Cursor or offset pagination. Add when a tenant needs it. v1
//     supports "page" (page/limit query params) and "none" (single
//     fetch).
//   - JSONPath wildcards (`*`, `[]`). Field map values are dot-paths
//     up to a few levels deep — sufficient for every catalog we have
//     seen. Real JSONPath belongs in a later config tier.
//   - Per-item detail fetch (list endpoint + detail endpoint per row).
//     v1 reads everything from the list endpoint; if a tenant needs
//     detail data (e.g. variants), the upstream is responsible for
//     including it in the list payload. PR-3+ may add an opt-in
//     two-stage fetch.
package rest_json
