# Feature: ai-cost-controls

Phase-1 LLM cost controls for classifier traffic: structured JSON usage
logs, bounded-TTL exact-result cache, real token capture
(`internal/ai/classifier_cache.go`, `classifier_usage_log.go`).

- [technical.md](technical.md) — the Phase-1 contract. Implementation state:
  **backed** (shipped on main with tests; the doc's own "not merged" header
  predates the merge).
