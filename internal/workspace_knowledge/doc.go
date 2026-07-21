// Package workspace_knowledge is the domain layer for the Workspace
// Knowledge OS. It contains no database/sql imports — these are the
// cross-boundary types that handlers, ingestors, the retrieval engine,
// and the agent runtime all consume.
//
// Layout:
//
//	workspace_knowledge/
//	  sources/        Layer 1 domain: Source, SourceType, SyncPolicy, Health
//	  assets/         Layer 3 domain: Asset, AssetType, AssetState, Metrics
//	  ingestion/      Layer 2 port:   Ingestor + AssetWriter interfaces
//	  retrieval/      Layer 4 port:   Searcher interface
//	  observability/  Layer 7 port:   Metrics interface
//
// Persistence (the SQLite-backed repository) lives in internal/store/.
// The store package depends on these domain types; the domain types do
// not depend on the store. This direction matters — when the team
// later replaces SQLite with Postgres + pgvector, only the store needs
// to change.
//
// Design document: specs/domains/knowledge-platform/features/knowledge-os/technical.md. Read it before
// adding fields, methods, or new ports.
package workspace_knowledge
