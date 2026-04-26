## ADDED Requirements

### Requirement: Pool exhaustion alert
The system SHALL fire an alert when the ratio of allocated ports to total port capacity exceeds `ALERT_POOL_EXHAUSTION_THRESHOLD` (default 0.9). The alert SHALL be emitted as a WARN log line and optionally sent to the Telegram admin chat.

#### Scenario: Pool exhaustion alert fires at threshold
- **WHEN** allocated ports reach 90% of total capacity (with default threshold) and the last alert for this type was more than 5 minutes ago
- **THEN** a WARN log line is emitted with `"event": "alert_pool_exhaustion"` and (if `ALERT_TELEGRAM_ENABLED=true`) a Telegram message is sent to the admin chat

#### Scenario: Pool exhaustion alert suppressed within rate-limit window
- **WHEN** a pool exhaustion alert was fired less than 5 minutes ago and the condition still holds
- **THEN** no additional log alert or Telegram message is sent; condition is logged at DEBUG only

#### Scenario: Alert clears when pool recovers
- **WHEN** allocated ports drop below the threshold after an alert was fired
- **THEN** a recovery INFO log line is emitted with `"event": "alert_pool_exhaustion_resolved"`

### Requirement: Queue backlog spike alert
The system SHALL fire an alert when `queued_jobs` exceeds `ALERT_QUEUE_BACKLOG_THRESHOLD` (default 100).

#### Scenario: Queue backlog alert fires
- **WHEN** the number of pending jobs in the scheduler queue exceeds 100 (default) and rate-limit allows
- **THEN** a WARN log line is emitted with `"event": "alert_queue_backlog"`, including current queue depth

#### Scenario: Queue backlog alert uses configured threshold
- **WHEN** `ALERT_QUEUE_BACKLOG_THRESHOLD=50` is set and queue depth reaches 51
- **THEN** the alert fires at 51 pending jobs, not 100

### Requirement: Container crash-loop alert
The system SHALL detect when the same `account_id` experiences repeated container start failures within a sliding time window and fire a crash-loop alert.

#### Scenario: Crash-loop detected at threshold
- **WHEN** account 42 has `ALERT_CRASH_LOOP_COUNT` (default 3) start failures within `ALERT_CRASH_LOOP_WINDOW` (default 5 minutes) and rate-limit allows
- **THEN** a WARN log line is emitted with `"event": "alert_crash_loop"`, `"account_id": 42`, and the failure count; (if enabled) a Telegram alert is sent

#### Scenario: Failures spread across window boundary do not trigger alert
- **WHEN** account 42 has 2 failures before the window and 2 failures after the window boundary (4 total but only 2 within the window)
- **THEN** the crash-loop alert does NOT fire

#### Scenario: Crash-loop window resets after recovery
- **WHEN** account 42 has 2 failures followed by a successful start
- **THEN** the failure ring buffer for account 42 is NOT cleared (ring buffer tracks raw timestamps); the alert fires only if the window count reaches the threshold regardless of intervening successes

### Requirement: Alert rate limiting
The system SHALL suppress repeated alerts of the same type within a 5-minute suppression window to prevent Telegram spam and log flooding. Each alert type SHALL have its own independent suppression timer.

#### Scenario: Same alert type suppressed within window
- **WHEN** an alert of type `alert_queue_backlog` fires and the same condition persists 2 minutes later
- **THEN** no second alert is emitted; condition is noted at DEBUG level only

#### Scenario: Different alert types are independent
- **WHEN** a pool exhaustion alert fires, and 1 minute later a queue backlog alert condition is detected
- **THEN** the queue backlog alert fires immediately (its own suppression timer has not started)

### Requirement: Telegram alert delivery
When `ALERT_TELEGRAM_ENABLED=true`, the system SHALL send alert messages to `TELEGRAM_ADMIN_CHAT` using the existing Telegram bot instance. Alert messages SHALL include the alert type, current metric values, and a timestamp.

#### Scenario: Telegram message sent on alert
- **WHEN** `ALERT_TELEGRAM_ENABLED=true` and a pool exhaustion alert fires
- **THEN** a message is sent to `TELEGRAM_ADMIN_CHAT` containing the alert name, current utilization percentage, and UTC timestamp

#### Scenario: Telegram disabled — no message sent
- **WHEN** `ALERT_TELEGRAM_ENABLED=false` (default) and an alert fires
- **THEN** only the WARN log line is emitted; no Telegram API call is made
