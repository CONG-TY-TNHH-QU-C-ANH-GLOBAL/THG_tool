# Feature: sales-copilot

Natural-language copilot over the workspace's Facebook capabilities: intent
pipeline (normalize → entities → classify → route → workflow), open-prompt
orchestration, and the founder-directed omnichannel Telegram track
(`internal/drivers/copilot`, `agent_action_router` pipeline).

- [technical.md](technical.md) — ACTIVE_BINDING intent/routing architecture.
  Implementation state: **backed** (pipeline characterization-tested; package
  moved from `internal/ai/` to `internal/drivers/copilot`).
- [implementation/telegram-track.md](implementation/telegram-track.md) —
  omnichannel track (Telegram as peer interface); comment-quality
  prerequisites shipped, later phases staged (partial).
- [decisions/open-prompt-agent.md](decisions/open-prompt-agent.md) — Phase 6
  open-prompt agent design, DRAFT awaiting approval; partly realized
  (`Agent.ProcessPromptForOrg`).
- [implementation/voice-enterprise-plan.md](implementation/voice-enterprise-plan.md)
  — draft forward vision (voice + private-data enterprise); proposed, not
  implementation guidance.
