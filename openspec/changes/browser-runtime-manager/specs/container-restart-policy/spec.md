## ADDED Requirements

### Requirement: Configurable restart policy
The system SHALL support three restart policies configured via `CONTAINER_RESTART_POLICY` environment variable: `never` (no automatic restart), `on-failure:<N>` (restart up to N times on failure or unhealthy), `always` (always restart). The policy SHALL apply to containers that reach `stopped` state from an unexpected path (crash, OOM, health failure) or that are found exited during reconciliation.

#### Scenario: `never` policy — no restart
- **WHEN** a container transitions to `stopped` after an unexpected failure and the policy is `never`
- **THEN** the container row stays in `stopped` state; no `browser_start` job is submitted; no further action is taken

#### Scenario: `on-failure:<N>` policy — restart within limit
- **WHEN** a container transitions to `stopped` unexpectedly, the policy is `on-failure:3`, and `restart_count < 3`
- **THEN** `restart_count` is incremented in `browser_containers`; a new `browser_start` job is submitted via `jobs.Submit` with `run_after = now`; the container row transitions to `pending`

#### Scenario: `on-failure:<N>` policy — limit exhausted
- **WHEN** a container transitions to `stopped` unexpectedly, the policy is `on-failure:3`, and `restart_count >= 3`
- **THEN** no restart job is submitted; the row stays in `stopped` state; a warning is logged with the account ID and final restart count

#### Scenario: `always` policy — unconditional restart
- **WHEN** a container transitions to `stopped` for any reason and the policy is `always`
- **THEN** `restart_count` is incremented; a new `browser_start` job is submitted; no upper bound on restart count

### Requirement: Restart count persistence
The system SHALL persist `restart_count` in `browser_containers` and increment it atomically as part of the state transition that triggers a restart. `restart_count` is NOT reset between restarts — it accumulates for the lifetime of the account's row.

#### Scenario: Restart count incremented atomically
- **WHEN** the restart policy triggers a restart
- **THEN** `UPDATE browser_containers SET state='pending', restart_count=restart_count+1, updated_at=? WHERE account_id=? AND state='stopped'` is executed in a single statement

### Requirement: Clean stop does not trigger restart
The system SHALL distinguish between an intentional stop (via `POST /browser/stop`) and an unintentional stop (crash, OOM, health failure). Only unintentional stops evaluate the restart policy.

#### Scenario: Intentional stop via API
- **WHEN** `POST /browser/stop` is called for a running container
- **THEN** the container transitions through `stopping → stopped`; the restart policy is NOT evaluated; `restart_count` is NOT incremented

#### Scenario: Health-failure stop triggers restart evaluation
- **WHEN** a container transitions `running → unhealthy → stopping → stopped` due to health probe failure
- **THEN** the restart policy IS evaluated after the `stopped` state is reached
