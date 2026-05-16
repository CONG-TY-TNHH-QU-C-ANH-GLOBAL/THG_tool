/**
 * Step 4a — Verified Execution Reality view.
 *
 * Three observation panels over the verified execution substrate:
 *
 *  1. Outcome distribution — counts grouped by outcome × action_type for
 *     the window (default 24h). Answers "what fraction of the last 24h
 *     reached dom_verified vs shadow_rejected vs rate_limited?"
 *  2. Recent attempts — newest-first table with outcome, account, target,
 *     and parsed evidence (comment permalink / message bubble id / notes).
 *     The "what just happened" feed.
 *  3. Account health — per-account risk_score, recent_failures, cooldown,
 *     trust_level. Sorted by risk_score DESC so poisoned accounts surface
 *     immediately.
 *
 * This view is purely observational — no buttons that mutate state, no
 * "retry" / "force send" controls. The user's directive: stop inventing
 * intelligence, start observing reality.
 */

import { useMemo, useState } from 'react';
import { RefreshCw } from 'lucide-react';
import { theme, cardStyle, primaryBtn } from '../../constants/styles';
import { useExecutionReality } from '../../hooks/useExecutionReality';
import type { ExecutionAttemptRow, OutcomeBucket } from '../../services/executionService';

interface ExecutionRealityViewProps { orgId: string; isAdmin: boolean }

const WINDOW_OPTIONS = [1, 6, 24, 72, 168] as const;

// Outcome → semantic color. Maps onto the existing CSS-var palette
// (theme.green / theme.red / theme.yellow / theme.info / theme.textFaint)
// so badges stay consistent with other status tags across the app.
function outcomeColor(outcome: string): string {
  switch (outcome) {
    case 'dom_verified':
    case 'duplicate_blocked':
      return theme.green;
    case 'optimistic_success':
      return theme.info;
    case 'shadow_rejected':
    case 'blocked':
    case 'rate_limited':
      return theme.red;
    case 'redirected_feed':
    case 'context_drift':
    case 'captcha':
      return theme.yellow;
    case 'soft_fail':
    case 'verification_timeout':
    case 'composer_failed':
    case 'hard_fail':
    case 'retry_exhausted':
      return theme.textMuted;
    default:
      return theme.textFaint;
  }
}

// Risk score → label + color. Mirrors the policy resolver buckets so the
// dashboard doesn't invent its own band thresholds.
function riskBadge(score: number): { label: string; color: string } {
  if (score >= 0.7) return { label: 'critical', color: theme.red };
  if (score >= 0.4) return { label: 'elevated', color: theme.yellow };
  if (score >= 0.2) return { label: 'warm', color: theme.info };
  return { label: 'healthy', color: theme.green };
}

function formatRelative(iso: string): string {
  if (!iso) return '—';
  const t = new Date(iso).getTime();
  if (!Number.isFinite(t)) return '—';
  const diff = Date.now() - t;
  if (diff < 0) return 'in the future';
  const s = Math.floor(diff / 1000);
  if (s < 60) return `${s}s ago`;
  const m = Math.floor(s / 60);
  if (m < 60) return `${m}m ago`;
  const h = Math.floor(m / 60);
  if (h < 24) return `${h}h ago`;
  return `${Math.floor(h / 24)}d ago`;
}

function shorten(url: string, max = 56): string {
  if (!url) return '';
  if (url.length <= max) return url;
  return url.slice(0, max - 1) + '…';
}

export default function ExecutionRealityView(_: ExecutionRealityViewProps) {
  const [hours, setHours] = useState<number>(24);
  const { distribution, attempts, accounts, isLoading, error, refetch, setWindowHours } = useExecutionReality(hours);

  // Pivot the flat bucket list into outcome → action_type → count for the
  // grid renderer below. Keeps the API surface flat (easier to extend) and
  // the rendering logic local.
  const grid = useMemo(() => {
    const out: Record<string, Record<string, number>> = {};
    const outcomes = new Set<string>();
    const actionTypes = new Set<string>();
    (distribution?.buckets ?? []).forEach((b: OutcomeBucket) => {
      outcomes.add(b.outcome);
      actionTypes.add(b.action_type || 'unknown');
      out[b.outcome] = out[b.outcome] ?? {};
      out[b.outcome][b.action_type || 'unknown'] = b.count;
    });
    return {
      matrix: out,
      outcomes: Array.from(outcomes).sort(),
      actionTypes: Array.from(actionTypes).sort(),
    };
  }, [distribution]);

  const onWindowChange = (h: number) => {
    setHours(h);
    setWindowHours(h);
  };

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
      {/* Header: window selector + manual refresh */}
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 10 }}>
        <div>
          <h2 style={{ margin: 0, fontSize: 16, color: theme.text }}>Execution Reality</h2>
          <p style={{ margin: '2px 0 0', fontSize: 11, color: theme.textFaint }}>
            Verified outcomes over the last {hours}h — no intelligence, no decisions, just what the platform did.
          </p>
        </div>
        <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
          <div style={{ display: 'flex', gap: 4 }}>
            {WINDOW_OPTIONS.map(h => (
              <button
                key={h}
                type="button"
                onClick={() => onWindowChange(h)}
                style={{
                  ...primaryBtn(),
                  background: hours === h ? theme.primary : 'transparent',
                  color: hours === h ? 'var(--accent-ink)' : theme.text,
                  border: `1px solid ${hours === h ? theme.primary : theme.border}`,
                  fontSize: 11,
                  padding: '4px 10px',
                }}
              >
                {h < 24 ? `${h}h` : `${h / 24}d`}
              </button>
            ))}
          </div>
          <button type="button" onClick={refetch} disabled={isLoading} style={{
            ...primaryBtn(),
            background: 'transparent',
            color: theme.text,
            border: `1px solid ${theme.border}`,
            display: 'flex',
            alignItems: 'center',
            gap: 4,
            fontSize: 11,
            padding: '4px 10px',
          }}>
            <RefreshCw size={12} />
            Refresh
          </button>
        </div>
      </div>

      {error && (
        <div style={{ ...cardStyle(), borderColor: theme.red, color: theme.red, fontSize: 12 }}>
          {error.message}
        </div>
      )}

      {/* Panel 1 — Outcome distribution grid */}
      <section style={cardStyle()}>
        <header style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'baseline', marginBottom: 10 }}>
          <h3 style={{ margin: 0, fontSize: 13, color: theme.text }}>Outcome distribution</h3>
          <span style={{ fontSize: 11, color: theme.textFaint }}>
            {distribution?.total ?? 0} attempts
          </span>
        </header>
        {grid.outcomes.length === 0 ? (
          <p style={{ margin: 0, fontSize: 12, color: theme.textFaint }}>
            No verified outcomes in this window yet. {isLoading ? 'Loading…' : 'Once the extension ships rich proof or actions are queued, this fills in.'}
          </p>
        ) : (
          <div style={{ overflowX: 'auto' }}>
            <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 12 }}>
              <thead>
                <tr style={{ borderBottom: `1px solid ${theme.border}` }}>
                  <th style={{ textAlign: 'left', padding: '6px 8px', color: theme.textMuted, fontWeight: 600 }}>outcome</th>
                  {grid.actionTypes.map(at => (
                    <th key={at} style={{ textAlign: 'right', padding: '6px 8px', color: theme.textMuted, fontWeight: 600 }}>{at}</th>
                  ))}
                  <th style={{ textAlign: 'right', padding: '6px 8px', color: theme.textMuted, fontWeight: 600 }}>total</th>
                </tr>
              </thead>
              <tbody>
                {grid.outcomes.map(outcome => {
                  const row = grid.matrix[outcome] ?? {};
                  const rowTotal = Object.values(row).reduce((s, n) => s + n, 0);
                  return (
                    <tr key={outcome} style={{ borderBottom: `1px solid ${theme.borderAlt}` }}>
                      <td style={{ padding: '6px 8px' }}>
                        <span style={{
                          display: 'inline-block',
                          width: 8,
                          height: 8,
                          borderRadius: 99,
                          background: outcomeColor(outcome),
                          marginRight: 6,
                          verticalAlign: 'middle',
                        }} />
                        <span style={{ color: theme.text }}>{outcome}</span>
                      </td>
                      {grid.actionTypes.map(at => (
                        <td key={at} style={{ textAlign: 'right', padding: '6px 8px', color: row[at] ? theme.text : theme.textFaint }}>
                          {row[at] ?? '—'}
                        </td>
                      ))}
                      <td style={{ textAlign: 'right', padding: '6px 8px', color: theme.text, fontWeight: 600 }}>
                        {rowTotal}
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}
      </section>

      {/* Panel 2 — Recent attempts */}
      <section style={cardStyle()}>
        <header style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'baseline', marginBottom: 10 }}>
          <h3 style={{ margin: 0, fontSize: 13, color: theme.text }}>Recent attempts</h3>
          <span style={{ fontSize: 11, color: theme.textFaint }}>
            {attempts.length} rows
          </span>
        </header>
        {attempts.length === 0 ? (
          <p style={{ margin: 0, fontSize: 12, color: theme.textFaint }}>
            {isLoading ? 'Loading…' : 'No attempts in this window.'}
          </p>
        ) : (
          <div style={{ overflowX: 'auto' }}>
            <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 11 }}>
              <thead>
                <tr style={{ borderBottom: `1px solid ${theme.border}` }}>
                  {['when', 'action', 'outcome', 'account', 'target', 'evidence'].map(h => (
                    <th key={h} style={{ textAlign: 'left', padding: '6px 8px', color: theme.textMuted, fontWeight: 600 }}>{h}</th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {attempts.map(a => (
                  <AttemptRow key={a.id} attempt={a} />
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>

      {/* Panel 3 — Account health snapshot */}
      <section style={cardStyle()}>
        <header style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'baseline', marginBottom: 10 }}>
          <h3 style={{ margin: 0, fontSize: 13, color: theme.text }}>Account health</h3>
          <span style={{ fontSize: 11, color: theme.textFaint }}>
            {accounts.length} accounts
          </span>
        </header>
        {accounts.length === 0 ? (
          <p style={{ margin: 0, fontSize: 12, color: theme.textFaint }}>
            {isLoading ? 'Loading…' : 'No runtime state rows yet. Will populate as accounts queue actions.'}
          </p>
        ) : (
          <div style={{ overflowX: 'auto' }}>
            <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 12 }}>
              <thead>
                <tr style={{ borderBottom: `1px solid ${theme.border}` }}>
                  {['account', 'trust', 'risk', 'failures', 'cooldown', 'last action', 'comments today', 'inbox today'].map(h => (
                    <th key={h} style={{ textAlign: 'left', padding: '6px 8px', color: theme.textMuted, fontWeight: 600 }}>{h}</th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {accounts.map(acc => {
                  const badge = riskBadge(acc.risk_score);
                  const inCooldown = acc.cooldown_until && new Date(acc.cooldown_until).getTime() > Date.now();
                  return (
                    <tr key={acc.account_id} style={{ borderBottom: `1px solid ${theme.borderAlt}` }}>
                      <td style={{ padding: '6px 8px', color: theme.text, fontFamily: 'var(--font-mono)' }}>#{acc.account_id}</td>
                      <td style={{ padding: '6px 8px', color: theme.text }}>{acc.trust_level}</td>
                      <td style={{ padding: '6px 8px' }}>
                        <span style={{
                          display: 'inline-block',
                          padding: '2px 8px',
                          borderRadius: 99,
                          background: badge.color,
                          color: 'var(--accent-ink)',
                          fontSize: 10,
                          fontWeight: 600,
                          marginRight: 6,
                        }}>{badge.label}</span>
                        <span style={{ color: theme.textMuted }}>{acc.risk_score.toFixed(2)}</span>
                      </td>
                      <td style={{ padding: '6px 8px', color: acc.recent_failures > 0 ? theme.red : theme.textFaint }}>{acc.recent_failures}</td>
                      <td style={{ padding: '6px 8px', color: inCooldown ? theme.yellow : theme.textFaint }}>
                        {inCooldown ? `until ${new Date(acc.cooldown_until).toLocaleString()}` : '—'}
                      </td>
                      <td style={{ padding: '6px 8px', color: theme.textMuted }}>{formatRelative(acc.last_action_at)}</td>
                      <td style={{ padding: '6px 8px', color: theme.text, textAlign: 'right' }}>{acc.comments_today}</td>
                      <td style={{ padding: '6px 8px', color: theme.text, textAlign: 'right' }}>{acc.inbox_today}</td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}
      </section>
    </div>
  );
}

// AttemptRow renders one execution_attempts row, with the evidence
// expanded into the most-relevant proof fields. Kept inline because it's
// only used here.
function AttemptRow({ attempt }: { attempt: ExecutionAttemptRow }) {
  const ev = attempt.evidence ?? {};
  const permalink = typeof ev.comment_permalink === 'string' ? ev.comment_permalink : '';
  const bubbleID = typeof ev.message_bubble_id === 'string' ? ev.message_bubble_id : '';
  const pageURL = typeof ev.page_url_after === 'string' ? ev.page_url_after : '';
  const notes = typeof ev.notes === 'string' ? ev.notes : '';

  const evidenceBits: Array<{ label: string; value: string; isLink?: boolean }> = [];
  if (permalink) evidenceBits.push({ label: 'comment', value: shorten(permalink), isLink: true });
  if (bubbleID) evidenceBits.push({ label: 'bubble', value: bubbleID });
  if (pageURL && pageURL !== attempt.target_url) evidenceBits.push({ label: 'after', value: shorten(pageURL) });
  if (notes) evidenceBits.push({ label: 'note', value: notes });

  return (
    <tr style={{ borderBottom: `1px solid ${theme.borderAlt}` }}>
      <td style={{ padding: '6px 8px', color: theme.textMuted, whiteSpace: 'nowrap' }}>
        {formatRelative(attempt.started_at)}
      </td>
      <td style={{ padding: '6px 8px', color: theme.text }}>{attempt.action_type}</td>
      <td style={{ padding: '6px 8px' }}>
        <span style={{
          display: 'inline-block',
          width: 6,
          height: 6,
          borderRadius: 99,
          background: outcomeColor(attempt.outcome),
          marginRight: 6,
          verticalAlign: 'middle',
        }} />
        <span style={{ color: theme.text }}>{attempt.outcome || attempt.status}</span>
        {attempt.failure_reason && attempt.failure_reason !== attempt.outcome && (
          <span style={{ color: theme.textFaint, marginLeft: 6 }}>· {attempt.failure_reason}</span>
        )}
        {attempt.attempt > 1 && (
          <span style={{ color: theme.yellow, marginLeft: 6 }}>×{attempt.attempt}</span>
        )}
      </td>
      <td style={{ padding: '6px 8px', color: theme.text, fontFamily: 'var(--font-mono)' }}>#{attempt.account_id}</td>
      <td style={{ padding: '6px 8px', color: theme.textMuted }}>
        {attempt.target_url ? (
          <a href={attempt.target_url} target="_blank" rel="noreferrer" style={{ color: theme.info, textDecoration: 'none' }}>
            {shorten(attempt.target_url)}
          </a>
        ) : '—'}
      </td>
      <td style={{ padding: '6px 8px', color: theme.textMuted }}>
        {evidenceBits.length === 0 ? '—' : (
          <span style={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
            {evidenceBits.map((bit, i) => (
              <span key={i}>
                <span style={{ color: theme.textFaint }}>{bit.label}:</span>{' '}
                {bit.isLink ? (
                  <a href={permalink} target="_blank" rel="noreferrer" style={{ color: theme.info, textDecoration: 'none' }}>{bit.value}</a>
                ) : (
                  <span style={{ color: theme.text }}>{bit.value}</span>
                )}
              </span>
            ))}
          </span>
        )}
      </td>
    </tr>
  );
}
