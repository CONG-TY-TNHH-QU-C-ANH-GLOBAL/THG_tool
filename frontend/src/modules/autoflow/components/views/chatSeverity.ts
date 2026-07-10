// Severity mapping for SYSTEM messages in the workspace chat. Pure and
// self-contained (tested by chatSeverity.test.mjs via the TS-transpile harness).
//
// Root cause this replaces: WorkspaceChatView rendered EVERY system message in
// var(--hot) (error red), so a normal crawl-progress heartbeat looked like a
// failure. Severity is derived from fields the history API already returns
// (action_taken + action_args JSON + success) — no wire change.

export type SystemSeverity = 'info' | 'success' | 'warning' | 'error';

// Crawl codes that pause the run for a human — warning, never auto-handled.
const RISK_CODES = new Set(['checkpoint_suspected', 'login_required', 'risk_blocked']);

// Non-risk stops where the crawl ended without reaching the target: the run is
// fine (not an error) but worth a glance — neutral warning.
const STALLED_EXITS = new Set([
  'duplicate_heavy',
  'no_progress',
  'no_new_items_after_scroll',
  'scroll_not_moving',
  'pass_exhausted',
]);

function parseArgs(actionArgs: string): Record<string, unknown> {
  try {
    const parsed: unknown = JSON.parse(actionArgs);
    return parsed && typeof parsed === 'object' ? (parsed as Record<string, unknown>) : {};
  } catch {
    return {};
  }
}

function argString(args: Record<string, unknown>, key: string): string {
  const value = args[key];
  return typeof value === 'string' ? value : '';
}

export function systemMessageSeverity(actionTaken: string, actionArgs: string, success: boolean): SystemSeverity {
  if (actionTaken === 'system_crawl_failure') return 'error';
  if (actionTaken === 'system_crawl_progress') {
    const code = argString(parseArgs(actionArgs), 'safe_reason_code');
    return RISK_CODES.has(code) ? 'warning' : 'info';
  }
  if (actionTaken === 'system_crawl_summary') {
    const reason = argString(parseArgs(actionArgs), 'exit_reason');
    if (RISK_CODES.has(reason) || STALLED_EXITS.has(reason)) return 'warning';
    return 'success';
  }
  // Every other system event keeps the generic semantic: red only for failures.
  return success ? 'info' : 'error';
}

// Design-token colors per severity (text + avatar chip). Tokens already exist
// and are used semantically elsewhere (browserHelpers.ts).
export const SEVERITY_COLOR: Record<SystemSeverity, string> = {
  info: 'var(--info)',
  success: 'var(--ok)',
  warning: 'var(--warn)',
  error: 'var(--hot)',
};
