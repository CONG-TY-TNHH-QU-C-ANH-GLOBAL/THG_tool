# Feature: data-platform

Cross-domain data-plane strategy: SQLite for local runtime/cache/outbox,
PostgreSQL for durable tenant-scoped SaaS state, explicit events/outbox
between planes. The current authority for plane ownership is
`docs/architecture/DATABASE_OWNERSHIP.md`; the realized dual-target slice is
KnowledgeOS ([postgres-compat](../../../knowledge-platform/features/knowledge-os/implementation/postgres-compat.md)).

- [implementation/production-database-migration.md](implementation/production-database-migration.md)
  — draft aspirational all-domains SQLite→Postgres migration plan (proposed;
  only the KnowledgeOS slice is realized).
