# Facebook Multi-Group Fresh-Lead Crawl: PostgreSQL Schema Implementation Blueprint

**Target:** `specs/facebook/FACEBOOK_CRAWL_CAMPAIGN_POSTGRES_SCHEMA_IMPLEMENTATION.md`
**Status:** Preflight-gated implementation blueprint for PR-M2B
**Track:** Facebook Automation Reliability
**Authoritative parent:** `specs/facebook/MULTI_GROUP_FRESH_LEAD_CRAWL_SPEC.md`

## 1. Purpose and scope

PR-M2B creates the durable PostgreSQL platform schema required by the multi-group fresh-lead crawl architecture:

- crawl campaigns;
- campaign account pools;
- campaign sources;
- append-only crawl-run attempts;
- fresh-lead identity and idempotency.

PR-M2B is **runtime-inert**. It must not add or change:

- scheduler behavior;
- HTTP/API/UI behavior;
- store/service consumers;
- extension or connector behavior;
- freshness filtering or temporal-frontier logic;
- result-ingest behavior;
- account-safety policy;
- session, cookie, authentication, or concurrency budgets;
- SQLite schema.

PostgreSQL platform is the SaaS source of truth. SQLite remains local runtime/cache/outbox storage only. RAG is not involved.

## 2. Authority and preflight gate

The merged architecture spec is authoritative for product invariants. The current repository is authoritative for migration numbering, SQL naming conventions, existing table types, migration-runner behavior, and test harnesses.

Claude must stop and report instead of inventing when:

- the working tree is not clean;
- main or baseline PostgreSQL migration validation is already broken;
- the latest migration number or naming convention is unclear;
- the canonical account or lead owner cannot be identified;
- required column types conflict with the merged architecture spec;
- an existing equivalent constraint or index already exists;
- a proposed down migration would remove a shared pre-existing anchor.

Before writing SQL, record the verified result of:

1. `git status --short`.
2. Listing `internal/store/migrations/platform/`.
3. The exact latest migration version and next available contiguous versions.
4. The actual up/down filename convention.
5. The migration runner's transaction behavior.
6. Whether `CREATE INDEX CONCURRENTLY` is supported by that runner.
7. The custom migration advisory-lock owner and code path.
8. The real PostgreSQL migration test entrypoint and `POSTGRES_PLATFORM_TEST_DSN` convention.
9. The canonical `accounts` table name, `id` type, `org_id` type, and existing keys/indexes.
10. The canonical PostgreSQL lead table name, `id` type, `org_id` type, and existing keys/indexes.
11. Existing platform constraints equivalent to `UNIQUE (org_id, id)` on accounts, leads, or runs.

Do not write an assumed migration number into implementation files.

## 3. Migration ownership and split

Prefer three dependency-ordered, domain-owned migration units using the verified next versions `N`, `N+1`, and `N+2`.

### Migration A — campaign foundation

Proposed logical name:

```text
[N]_facebook_crawl_campaign_foundation
```

Creates:

- the accounts tenant anchor only when missing and owned safely by this migration;
- `facebook_crawl_campaigns`;
- `facebook_crawl_campaign_accounts`;
- `facebook_crawl_campaign_sources`.

### Migration B — append-only run ledger

Proposed logical name:

```text
[N+1]_facebook_crawl_runs_ledger
```

Creates:

- `facebook_crawl_runs`;
- lifecycle checks;
- tenant-safe foreign keys;
- active/open/retry/task idempotency indexes.

### Migration C — fresh-lead identity

Proposed logical name:

```text
[N+2]_facebook_crawl_lead_index
```

Creates:

- `facebook_crawl_lead_index`;
- org-scoped run provenance;
- optional org-scoped lead provenance only after the canonical lead owner is verified;
- fresh-lead identity uniqueness.

Adapt only the numeric prefix and filename suffixes to the verified repository convention. Do not edit, renumber, or append objects to already-applied migration files.

Down migrations must execute in reverse dependency order:

1. lead index;
2. runs;
3. sources, campaign accounts, campaigns;
4. an account/lead anchor only when this train created it and no remaining object depends on it.

Do not drop a pre-existing or shared anchor.

## 4. Canonical tenant anchors

Composite foreign keys require non-partial unique anchors.

For `accounts`, add the equivalent of the following only when no equivalent key/index exists:

```sql
CREATE UNIQUE INDEX uq_accounts_org_id_id
    ON accounts (org_id, id);
```

Before creating it:

- verify exact table and column types;
- run a duplicate preflight;
- verify it is not redundant with an existing primary/unique key;
- assess lock/scan cost on the existing production table.

Preflight query:

```sql
SELECT org_id, id, COUNT(*)
FROM accounts
GROUP BY org_id, id
HAVING COUNT(*) > 1;
```

### Accounts-anchor production apply gate

The anchor is the one non-empty-table build in this train; recency of the
platform baseline does **not** make it operationally safe. Do not apply `0112`
to a production `accounts` table until all of the following are recorded:

1. actual `accounts` row count;
2. the duplicate preflight above returns zero rows;
3. an acceptable write-lock-window assessment for a plain transactional
   `CREATE UNIQUE INDEX` (it takes a `SHARE` lock and blocks writes for the
   build duration);
4. a stop-and-split decision rule: if the table is too large for that window,
   do **not** apply `0112` as-is;
5. in that case a separately reviewed migration using `-- migrate:notx` +
   `CREATE UNIQUE INDEX CONCURRENTLY` (which owns its own atomicity), planned
   and approved before apply.

The same rule applies if the canonical lead table needs an `(org_id, id)` anchor. Do not assume a `leads` table or add a lead foreign key until ownership and types are verified.

## 5. `facebook_crawl_campaigns`

The PR-M0 contract defines campaign configuration. Do not replace these fields with speculative alternatives such as `max_runs_per_source`.

Normative shape:

```sql
CREATE TABLE facebook_crawl_campaigns (
    id                       BIGSERIAL PRIMARY KEY,
    org_id                   BIGINT NOT NULL,
    name                     TEXT NOT NULL,
    status                   TEXT NOT NULL DEFAULT 'active',
    freshness_window_minutes INTEGER NOT NULL DEFAULT 1440,
    cadence_minutes          INTEGER NOT NULL DEFAULT 240,
    max_items_per_run        INTEGER NOT NULL DEFAULT 50,
    created_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_fb_crawl_campaigns_org_id_id
        UNIQUE (org_id, id),
    CONSTRAINT ck_fb_crawl_campaigns_status
        CHECK (status IN ('active', 'paused', 'archived')),
    CONSTRAINT ck_fb_crawl_campaigns_freshness_window
        CHECK (freshness_window_minutes > 0),
    CONSTRAINT ck_fb_crawl_campaigns_cadence
        CHECK (cadence_minutes > 0),
    CONSTRAINT ck_fb_crawl_campaigns_max_items
        CHECK (max_items_per_run > 0)
);

CREATE INDEX ix_fb_crawl_campaigns_org_status
    ON facebook_crawl_campaigns (org_id, status);
```

If the merged parent spec on main differs, the parent spec wins. Report the conflict before implementation rather than silently changing the model.

## 6. `facebook_crawl_campaign_accounts`

This table defines which canonical Facebook accounts may serve a campaign.

```sql
CREATE TABLE facebook_crawl_campaign_accounts (
    org_id      BIGINT NOT NULL,
    campaign_id BIGINT NOT NULL,
    account_id  BIGINT NOT NULL,
    assigned_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT pk_fb_crawl_campaign_accounts
        PRIMARY KEY (org_id, campaign_id, account_id),
    CONSTRAINT fk_fb_crawl_campaign_accounts_campaign
        FOREIGN KEY (org_id, campaign_id)
        REFERENCES facebook_crawl_campaigns (org_id, id),
    CONSTRAINT fk_fb_crawl_campaign_accounts_account
        FOREIGN KEY (org_id, account_id)
        REFERENCES accounts (org_id, id)
);
```

Use default `NO ACTION`/`RESTRICT` semantics. A campaign/account relationship must not be silently deleted while sources or run history depend on it.

The database must reject:

- nonexistent accounts;
- accounts owned by another org;
- cross-org campaign/account relationships.

## 7. `facebook_crawl_campaign_sources`

A source is a normalized Facebook group target owned by one campaign.

```sql
CREATE TABLE facebook_crawl_campaign_sources (
    id                       BIGSERIAL PRIMARY KEY,
    org_id                   BIGINT NOT NULL,
    campaign_id              BIGINT NOT NULL,
    source_url               TEXT NOT NULL,
    normalized_source_key    TEXT NOT NULL,
    source_label             TEXT NOT NULL DEFAULT '',
    priority                 INTEGER NOT NULL DEFAULT 0,
    preferred_account_id     BIGINT,
    cursor_last_post_at      TIMESTAMPTZ,
    last_run_at              TIMESTAMPTZ,
    status                   TEXT NOT NULL DEFAULT 'active',
    created_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_fb_crawl_sources_org_campaign_id
        UNIQUE (org_id, campaign_id, id),
    CONSTRAINT uq_fb_crawl_sources_org_campaign_key
        UNIQUE (org_id, campaign_id, normalized_source_key),
    CONSTRAINT ck_fb_crawl_sources_status
        CHECK (status IN ('active', 'paused', 'archived')),
    CONSTRAINT fk_fb_crawl_sources_campaign
        FOREIGN KEY (org_id, campaign_id)
        REFERENCES facebook_crawl_campaigns (org_id, id),
    CONSTRAINT fk_fb_crawl_sources_preferred_account
        FOREIGN KEY (org_id, campaign_id, preferred_account_id)
        REFERENCES facebook_crawl_campaign_accounts
            (org_id, campaign_id, account_id)
);

CREATE INDEX ix_fb_crawl_sources_org_campaign_status
    ON facebook_crawl_campaign_sources (org_id, campaign_id, status);
```

`preferred_account_id IS NULL` means no sticky affinity. PostgreSQL `MATCH SIMPLE` semantics make the nullable composite foreign key valid.

Do **not** use `ON DELETE SET NULL` on the composite key as a shorthand. Removing an account from the pool must first clear or reassign the source affinity explicitly, then delete the pool row. This avoids accidentally nulling tenant/campaign columns or hiding an ownership transition.

`normalized_source_key` must be produced by one deterministic Facebook group URL normalization contract in a later runtime PR. PR-M2B only persists and constrains the key; it does not invent runtime normalization.

## 8. `facebook_crawl_runs`

Runs are append-only execution attempts. Retrying creates a new row. Previous terminal rows remain immutable history.

```sql
CREATE TABLE facebook_crawl_runs (
    id                   BIGSERIAL PRIMARY KEY,
    org_id               BIGINT NOT NULL,
    campaign_id          BIGINT NOT NULL,
    source_id            BIGINT NOT NULL,
    account_id           BIGINT,
    status               TEXT NOT NULL DEFAULT 'queued',
    exit_reason_code     TEXT NOT NULL DEFAULT '',
    fresh_cutoff_at      TIMESTAMPTZ,
    attempt              INTEGER NOT NULL DEFAULT 1,
    retry_of_run_id      BIGINT,
    task_id              TEXT,

    posts_seen           INTEGER NOT NULL DEFAULT 0,
    fresh_lead_count     INTEGER NOT NULL DEFAULT 0,
    stale_skipped        INTEGER NOT NULL DEFAULT 0,
    duplicate_count      INTEGER NOT NULL DEFAULT 0,
    unparsed_count       INTEGER NOT NULL DEFAULT 0,

    queued_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at           TIMESTAMPTZ,
    heartbeat_at         TIMESTAMPTZ,
    finished_at          TIMESTAMPTZ,

    CONSTRAINT uq_fb_crawl_runs_org_id_id
        UNIQUE (org_id, id),
    CONSTRAINT ck_fb_crawl_runs_status
        CHECK (
            status IN (
                'queued',
                'waiting_for_connector_upgrade',
                'running',
                'succeeded',
                'stopped_safe',
                'failed',
                'abandoned',
                'cancelled'
            )
        ),
    CONSTRAINT ck_fb_crawl_runs_running_requires_account
        CHECK (status <> 'running' OR account_id IS NOT NULL),
    CONSTRAINT ck_fb_crawl_runs_attempt
        CHECK (attempt > 0),
    CONSTRAINT ck_fb_crawl_runs_nonnegative_counters
        CHECK (
            posts_seen >= 0
            AND fresh_lead_count >= 0
            AND stale_skipped >= 0
            AND duplicate_count >= 0
            AND unparsed_count >= 0
        ),
    CONSTRAINT fk_fb_crawl_runs_campaign
        FOREIGN KEY (org_id, campaign_id)
        REFERENCES facebook_crawl_campaigns (org_id, id),
    CONSTRAINT fk_fb_crawl_runs_source
        FOREIGN KEY (org_id, campaign_id, source_id)
        REFERENCES facebook_crawl_campaign_sources
            (org_id, campaign_id, id),
    CONSTRAINT fk_fb_crawl_runs_account
        FOREIGN KEY (org_id, campaign_id, account_id)
        REFERENCES facebook_crawl_campaign_accounts
            (org_id, campaign_id, account_id),
    CONSTRAINT fk_fb_crawl_runs_retry_parent
        FOREIGN KEY (org_id, retry_of_run_id)
        REFERENCES facebook_crawl_runs (org_id, id)
);
```

Required partial unique indexes:

```sql
CREATE UNIQUE INDEX ux_fb_crawl_runs_one_active_account
    ON facebook_crawl_runs (org_id, account_id)
    WHERE status = 'running' AND account_id IS NOT NULL;

CREATE UNIQUE INDEX ux_fb_crawl_runs_one_open_source
    ON facebook_crawl_runs (org_id, source_id)
    WHERE status IN (
        'queued',
        'waiting_for_connector_upgrade',
        'running'
    );

CREATE UNIQUE INDEX ux_fb_crawl_runs_one_retry_per_parent
    ON facebook_crawl_runs (org_id, retry_of_run_id)
    WHERE retry_of_run_id IS NOT NULL;

CREATE UNIQUE INDEX ux_fb_crawl_runs_org_task
    ON facebook_crawl_runs (org_id, task_id)
    WHERE task_id IS NOT NULL;
```

Supporting read indexes:

```sql
CREATE INDEX ix_fb_crawl_runs_org_source_created
    ON facebook_crawl_runs (org_id, source_id, queued_at DESC);

CREATE INDEX ix_fb_crawl_runs_org_account_status
    ON facebook_crawl_runs (org_id, account_id, status);
```

The constraints prevent:

- `running` without an assigned account;
- two running runs for one org/account;
- two open runs for one org/source;
- two automatic retries for one parent;
- duplicate task ingestion within one org;
- cross-org or cross-campaign source/account relationships.

No cascade delete is permitted for run history.

## 9. Retry lineage and automatic retry creation

Original runs have `retry_of_run_id IS NULL`.

An automatic retry:

- creates a new append-only run row;
- increments `attempt`;
- sets `retry_of_run_id` to the abandoned parent;
- leaves the parent terminal and immutable;
- is created atomically with `INSERT ... ON CONFLICT DO NOTHING`, keyed by the unique retry-parent index.

Concurrent reapers observing the same abandoned parent must produce exactly one retry row. The losing transaction observes the existing retry and exits without error or a retry storm.

Manual retry behavior must be explicit in a later runtime spec. It must not silently bypass automatic retry lineage or the one-open-run invariant.

## 10. `facebook_crawl_lead_index`

This table reserves canonical post identity before a fresh lead is committed, preventing duplicate leads from concurrent or repeated crawls.

Normative shape:

```sql
CREATE TABLE facebook_crawl_lead_index (
    org_id           BIGINT NOT NULL,
    platform         TEXT NOT NULL DEFAULT 'facebook',
    post_dedup_hash  TEXT NOT NULL,
    run_id           BIGINT NOT NULL,
    lead_id          BIGINT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT pk_fb_crawl_lead_index
        PRIMARY KEY (org_id, platform, post_dedup_hash),
    CONSTRAINT fk_fb_crawl_lead_index_run
        FOREIGN KEY (org_id, run_id)
        REFERENCES facebook_crawl_runs (org_id, id)
);
```

`lead_id` is nullable during claim-first processing so the runtime transaction can:

1. reserve `(org_id, platform, post_dedup_hash)`;
2. create the canonical lead;
3. attach `lead_id`;
4. commit atomically.

If any step fails, the transaction rolls back and no orphan reservation remains.

Add a composite lead provenance foreign key only after preflight proves the canonical PostgreSQL lead table and an equivalent `(org_id, id)` anchor:

```sql
ALTER TABLE facebook_crawl_lead_index
    ADD CONSTRAINT fk_fb_crawl_lead_index_lead
    FOREIGN KEY (org_id, lead_id)
    REFERENCES <verified_lead_table> (org_id, id);
```

Do not assume the table is named `leads`. Do not modify the canonical lead table except for a separately reviewed additive tenant anchor when it is truly required.

## 11. PostgreSQL test matrix

Use the real PostgreSQL migration harness and repository-supported DSN. Do not simulate PostgreSQL constraints in SQLite.

At minimum, pin:

### Migration application

1. All platform migrations apply from a clean PostgreSQL database.
2. Up migrations apply in order.
3. Down migrations remove only objects owned by this train, in reverse order, when the repository supports down validation.
4. Re-applying the normal migration runner is stable.

### Canonical account ownership

5. A same-org account can join a campaign pool.
6. A nonexistent account is rejected.
7. A cross-org account is rejected.
8. Duplicate campaign account membership is rejected.

### Campaign/source integrity

9. A source cannot reference a campaign from another org.
10. A duplicate normalized source key in one campaign is rejected.
11. The same normalized key in another campaign is accepted.
12. `preferred_account_id IS NULL` is accepted.
13. A preferred account outside the campaign pool is rejected.
14. Removing a pool account with active source affinity is rejected until affinity is cleared.

### Run integrity

15. A run cannot pair a source with another campaign or org.
16. A run account must belong to the campaign pool.
17. `queued` and `waiting_for_connector_upgrade` may have a null account.
18. `running` with a null account is rejected.
19. Negative counters or a non-positive attempt are rejected.
20. Duplicate non-null `(org_id, task_id)` is rejected.

### Active/open/history constraints

21. Two running rows for the same org/account are rejected.
22. Different accounts can each have a running row at the database layer.
23. One source cannot have multiple queued/waiting/running rows.
24. Multiple terminal history rows for a source are accepted.

### Retry lineage

25. A same-org retry parent is accepted.
26. Cross-org retry lineage is rejected.
27. A second automatic retry for the same parent is rejected.
28. Concurrent retry inserts produce exactly one child row.

### Lead identity

29. Duplicate `(org_id, platform, post_dedup_hash)` is rejected.
30. The same post identity in another org is accepted.
31. Cross-org run provenance is rejected.
32. A reservation can be created with `lead_id IS NULL`.
33. Canonical lead provenance is enforced when the verified lead FK is present.
34. A failed claim-first transaction leaves no committed reservation.

Prefer exact constraint-name assertions where the test harness exposes PostgreSQL error details.

## 12. Production migration impact

Most tables are new and empty, so they are runtime-inert until later consumers are enabled. The existing-table tenant anchors are the non-zero-risk part.

Before production apply:

- run duplicate preflight queries;
- inspect existing equivalent indexes;
- estimate accounts/lead table row counts;
- confirm migration transaction and lock behavior;
- confirm whether the runner permits concurrent index creation;
- confirm the schema owner can create tables, indexes, and constraints;
- verify no runtime consumer is enabled before all three migrations succeed.

Do not claim the account or lead anchor is lock-free. A standard unique index may scan and lock an existing table. If the table is large and the transactional runner cannot use `CREATE INDEX CONCURRENTLY`, stop and split the anchor into a separately reviewed production migration plan.

Production verification should include:

```sql
SELECT to_regclass('facebook_crawl_campaigns');
SELECT to_regclass('facebook_crawl_campaign_accounts');
SELECT to_regclass('facebook_crawl_campaign_sources');
SELECT to_regclass('facebook_crawl_runs');
SELECT to_regclass('facebook_crawl_lead_index');
```

Also verify expected constraints/indexes through `pg_constraint` and `pg_indexes`.

If migration apply fails, startup must fail closed according to the existing platform migration policy. Do not start a partially migrated campaign runtime.

Rollback before runtime consumers is schema-only in reverse order. After data is written, rollback is an operational data-migration decision; dropping the run ledger or lead identity table destroys audit/idempotency state and must not be automated casually.

## 13. Rejected designs

- Durable campaign/run state in SQLite.
- Editing or renumbering applied migrations.
- One giant platform schema file.
- Raw `source_url` as the only group identity.
- `account_id = 0` sentinel.
- Application-only tenant, active-run, open-run, or retry checks.
- Retry without durable parent lineage.
- Cascade deletion of append-only run history.
- Composite `ON DELETE SET NULL` that hides ownership transitions.
- Duplicate-lead checks without a database identity reservation.
- Modifying the existing lead table without verified ownership and review.
- Scheduler, API, extension, or result-ingest wiring inside PR-M2B.

## 14. PR-M2B acceptance criteria

PR-M2B is ready to merge only when:

- preflight findings are reported with verified paths and versions;
- migration files are modular and use the actual next versions;
- no existing applied migration is edited;
- all objects are PostgreSQL-only;
- tenant-safe composite relationships are enforced;
- active/open/retry/task invariants are database-backed;
- append-only history cannot be cascade-deleted;
- lead identity is org-scoped and transaction-ready;
- the real PostgreSQL test matrix passes;
- baseline and complete platform migration validation pass;
- no runtime consumer or SQLite schema is added;
- the diff contains no unrelated formatting or generated reports.
