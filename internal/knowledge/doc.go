// Package knowledge is the KnowledgeOS domain: assets, sources, feedback, and
// retrieval — the grounding substrate every concrete outbound claim (price,
// website, proof) must cite. Missing grounding degrades honestly to a typed
// knowledge_gap; it is never invented.
//
// Architecture role: KNOWLEDGE (domain) — see MODULE_BOUNDARIES.md (knowledge).
//
//   - Allowed imports (conceptual): models, its store domain, ai (via ports for
//     embedding/classification), stdlib.
//   - Forbidden imports (conceptual): outbound, services/* workflows,
//     internal/server, connectors.
//
// SCAFFOLD ONLY (Phase A): boundary marker. Knowledge code currently lives at
// internal/store/knowledge and internal/workspace_knowledge. See MODULE_OWNERSHIP.yml.
package knowledge
