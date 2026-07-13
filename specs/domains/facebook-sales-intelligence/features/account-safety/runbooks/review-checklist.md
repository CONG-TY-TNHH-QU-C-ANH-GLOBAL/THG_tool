# Account Safety — Review Checklist (every PR-C* runtime PR)

Layer: **runbook** for the `account-safety` feature.
Extracted from the PR-C0.5 spec (§10; authority: [technical.md](../technical.md)).
Apply to every PR-C* runtime PR before merge.

- [ ] Does this PR **increase automation speed**? (Must be "no" unless C4 with telemetry.)
- [ ] Does it **increase concurrency**? (Must be "no"; defaults only tighten.)
- [ ] Does it **alter checkpoint behavior** beyond *detect → stop → report*?
- [ ] Does it attempt any **bypass/evasion/rotation/solving**? (Must be "no".)
- [ ] Does it **preserve the per-account lease** (one active workflow/account)?
- [ ] Does it **preserve data-plane doctrine** (no browser secrets server-side, no plane move without a migration PR)?
- [ ] Does it **avoid raw checkpoint/page-text leakage** (typed reason codes only)?
- [ ] Are **state/budget decisions testable as pure policy** (no browser/DB/clock in the unit)?
- [ ] Are **Sonar / CodeRabbit expected clean** (no suppressions, no config change)?
