// Package products is the vendor-neutral product-catalog domain for
// the Workspace Knowledge OS.
//
// A "product catalog" is one of several knowledge surfaces a tenant
// may connect — alongside FAQ databases, shipping policies, sales
// playbooks, etc. The platform supports many catalog backends per
// tenant (Shopify Storefront, WooCommerce REST, BigCommerce, CSV,
// Google Sheets, generic REST/JSON, JSON-LD scraped from HTML, …),
// and the contract here is the single shape all of them flow into
// before they reach the rest of the system.
//
// # What this package is
//
// This package owns three contracts:
//
//   1. [CanonicalProduct] — the vendor-neutral product struct. Every
//      adapter, regardless of upstream shape, MUST emit values of this
//      type. Downstream code (retrieval, governance, agent grounding)
//      reads this contract, never an adapter-specific shape.
//
//   2. [Adapter] — a type alias of [ingestion.Ingestor]. There is no
//      parallel dispatcher for product catalogs; product adapters
//      register into the existing [ingestion.Registry] under their
//      own [sources.SourceType] value. The marker that distinguishes
//      product-catalog assets downstream is the asset Type field
//      ([assets.AssetPODProduct]), not the source type.
//
//   3. [Writer] — a thin wrapper around [ingestion.AssetWriter] that
//      maps CanonicalProduct → assets.Asset internally so adapters
//      never construct an Asset directly. This keeps the mapping rule
//      (Title/Tags/Payload/ExternalID derivation) in exactly one
//      place — change the mapping here and every adapter benefits.
//
// # What this package is NOT
//
// It is not a hardcoded vendor adapter. There is no "THG" or any other
// tenant name in this package. Specific vendor wire formats live in
// sibling packages — e.g. internal/workspace_knowledge/ingestion/rest_json,
// internal/workspace_knowledge/ingestion/shopify — and import this
// package to produce CanonicalProduct values.
//
// # Replay stability
//
// Every CanonicalProduct carries three lineage fields that downstream
// retrieval and operator-replay surfaces rely on:
//
//   - SourceURL         — canonical URL of the product on the upstream
//                         system. Survives re-ingest unchanged unless
//                         the source actually moved.
//   - SourceUpdatedAt   — upstream's own updated_at; used by the
//                         scheduler to skip unchanged rows and by
//                         operators to answer "when was this last
//                         fresh from the source?".
//   - RawPayloadHash    — sha256 of the raw upstream payload that
//                         produced this canonical row. Two re-ingests
//                         with the same upstream bytes yield the same
//                         hash; an extractor bug shows up as the same
//                         hash producing different CanonicalProduct
//                         values, which is the signature for a real
//                         regression.
//   - ExtractorVersion  — semver-ish identifier of the adapter version
//                         that produced this row. Bumped whenever the
//                         field-extraction logic changes so historical
//                         assets are re-extractable for replay.
//
// These four fields together let an operator answer "why did the AI
// quote this price?" by reading the asset payload — no separate trace
// store is needed for the ingestion side.
package products
