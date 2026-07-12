-- migrate:notx
-- SaaS Platform plane — Facebook crawl: accounts (org_id, id) tenant anchor
-- (PR-M2B). The campaign account pool's (org_id, account_id) FK needs a
-- non-partial unique key on accounts(org_id, id); accounts has only a
-- single-column PK today.
--
-- Built CONCURRENTLY — hence `-- migrate:notx`, which runs this migration
-- outside a transaction while the boot advisory lock is still held — so a
-- non-empty production accounts table is not write-blocked for the whole index
-- build. Deterministic (no IF NOT EXISTS) so a same-named index — including a
-- leftover INVALID index from a failed concurrent build — fails the migration
-- visibly instead of masking drift. Production apply and the invalid-index
-- recovery procedure are gated in
-- specs/facebook/FACEBOOK_CRAWL_CAMPAIGN_POSTGRES_SCHEMA_IMPLEMENTATION.md §4.
CREATE UNIQUE INDEX CONCURRENTLY uq_accounts_org_id_id
    ON accounts (org_id, id);
