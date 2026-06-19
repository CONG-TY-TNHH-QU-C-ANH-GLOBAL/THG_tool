/**
 * Watchpoint B — Prompt Routing Reality view.
 *
 * Four observation panels over prompt_logs.routing_decision_json:
 *
 *  1. Routing Distribution — counts grouped by (route, reason_code) so
 *     operators can see the deterministic / brain / LLM split at a glance.
 *  2. Ambiguous Prompt Surface — which signals (source/action/target/
 *     market/quantity) users keep forgetting. The "training opportunity"
 *     panel.
 *  3. Conflict Candidates — heuristic-flagged rows: false-positive
 *     deterministic routings (user followed up with retry/cancel) and
 *     false-negative ask-backs (prompt was actually self-sufficient).
 *  4. Recent Prompts — newest-first feed of every prompt + its route +
 *     reason. The orchestration debugger.
 *
 * Read-only — no action buttons, no retry, no route override. The whole
 * point of this view is to MAKE routing measurable before any further
 * intelligence is built on top.
 */

import { useMemo, useState } from 'react';
import { RefreshCw } from 'lucide-react';
import { theme, cardStyle, primaryBtn } from '../../constants/styles';
import { usePromptRouting } from '../../hooks/usePromptRouting';
import type {
  ConflictRow,
  PromptRoutingBucket,
  PromptRoutingRow,
} from '../../services/promptRoutingService';

interface PromptRoutingRealityViewProps { orgId: string; isAdmin: boolean }

const WINDOW_OPTIONS = [1, 6, 24, 72, 168] as const;

function routeColor(route: string): string {
  switch (route) {
    case 'deterministic': return theme.green;
    case 'brain': return theme.info;
    case 'llm_fallback': return theme.yellow;
    case 'scope_guard': return theme.textMuted;
    case 'preflight': return theme.textFaint;
    case 'legacy': return theme.textFaint;
    default: return theme.textFaint;
  }
}

function shortenPrompt(text: string, max = 120): string {
  const clean = (text || '').replace(/\s+/g, ' ').trim();
  if (clean.length <= max) return clean;
  return clean.slice(0, max - 1) + '…';
}

function formatRelative(iso: string): string {
  if (!iso) return '—';
  const t = new Date(iso).getTime();
  if (!Number.isFinite(t)) return '—';
  const diff = Date.now() - t;
  if (diff < 0) return 'future';
  const s = Math.floor(diff / 1000);
  if (s < 60) return `${s}s ago`;
  const m = Math.floor(s / 60);
  if (m < 60) return `${m}m ago`;
  const h = Math.floor(m / 60);
  if (h < 24) return `${h}h ago`;
  return `${Math.floor(h / 24)}d ago`;
}

export default function PromptRoutingRealityView(_: Readonly<PromptRoutingRealityViewProps>) {
  const [hours, setHours] = useState<number>(24);
  const {
    distribution, recent, conflicts, missing,
    isLoading, error, refetch, setWindowHours,
  } = usePromptRouting(hours);

  // Pivot the flat bucket list into a route → percentage summary +
  // a route → reason matrix for the two distribution panels.
  const summary = useMemo(() => {
    const buckets: PromptRoutingBucket[] = distribution?.buckets ?? [];
    const byRoute: Record<string, number> = {};
    const matrix: Record<string, Record<string, number>> = {};
    const reasons = new Set<string>();
    for (const b of buckets) {
      byRoute[b.route] = (byRoute[b.route] ?? 0) + b.count;
      matrix[b.route] = matrix[b.route] ?? {};
      matrix[b.route][b.reason_code] = (matrix[b.route][b.reason_code] ?? 0) + b.count;
      reasons.add(b.reason_code);
    }
    const total = distribution?.total ?? 0;
    return {
      byRoute,
      matrix,
      reasons: Array.from(reasons).sort(),
      routes: Object.keys(byRoute).sort(),
      total,
    };
  }, [distribution]);

  const onWindowChange = (h: number) => {
    setHours(h);
    setWindowHours(h);
  };

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
      {/* Header */}
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 10 }}>
        <div>
          <h2 style={{ margin: 0, fontSize: 16, color: theme.text }}>Prompt Routing Reality</h2>
          <p style={{ margin: '2px 0 0', fontSize: 11, color: theme.textFaint }}>
            Where every prompt went over the last {hours}h — deterministic vs brain vs LLM, with reason codes. Watchpoint B.
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
              >{h < 24 ? `${h}h` : `${h / 24}d`}</button>
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

      {/* Panel 1 — route % share + reason breakdown */}
      <section style={cardStyle()}>
        <header style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'baseline', marginBottom: 10 }}>
          <h3 style={{ margin: 0, fontSize: 13, color: theme.text }}>Routing distribution</h3>
          <span style={{ fontSize: 11, color: theme.textFaint }}>{summary.total} prompts</span>
        </header>
        {summary.total === 0 ? (
          <p style={{ margin: 0, fontSize: 12, color: theme.textFaint }}>
            {isLoading ? 'Loading…' : 'No routing decisions in this window. Once prompts arrive, this fills in.'}
          </p>
        ) : (
          <>
            {/* Route share — single horizontal bar */}
            <div style={{ display: 'flex', height: 10, borderRadius: 99, overflow: 'hidden', marginBottom: 12, border: `1px solid ${theme.borderAlt}` }}>
              {summary.routes.map(r => {
                const n = summary.byRoute[r];
                const pct = (n / summary.total) * 100;
                return (
                  <div key={r} title={`${r}: ${n} (${pct.toFixed(1)}%)`}
                    style={{ width: `${pct}%`, background: routeColor(r) }} />
                );
              })}
            </div>
            <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(140px, 1fr))', gap: 8, marginBottom: 16 }}>
              {summary.routes.map(r => {
                const n = summary.byRoute[r];
                const pct = (n / summary.total) * 100;
                return (
                  <div key={r} style={{ display: 'flex', alignItems: 'center', gap: 6, fontSize: 11 }}>
                    <span style={{ width: 8, height: 8, borderRadius: 99, background: routeColor(r) }} />
                    <span style={{ color: theme.text }}>{r}</span>
                    <span style={{ color: theme.textMuted, marginLeft: 'auto' }}>{pct.toFixed(1)}% · {n}</span>
                  </div>
                );
              })}
            </div>
            {/* Reason × route matrix */}
            <div style={{ overflowX: 'auto' }}>
              <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 12 }}>
                <thead>
                  <tr style={{ borderBottom: `1px solid ${theme.border}` }}>
                    <th style={{ textAlign: 'left', padding: '6px 8px', color: theme.textMuted, fontWeight: 600 }}>reason_code</th>
                    {summary.routes.map(r => (
                      <th key={r} style={{ textAlign: 'right', padding: '6px 8px', color: theme.textMuted, fontWeight: 600 }}>{r}</th>
                    ))}
                  </tr>
                </thead>
                <tbody>
                  {summary.reasons.map(reason => (
                    <tr key={reason} style={{ borderBottom: `1px solid ${theme.borderAlt}` }}>
                      <td style={{ padding: '6px 8px', color: theme.text }}>{reason}</td>
                      {summary.routes.map(r => {
                        const v = summary.matrix[r]?.[reason] ?? 0;
                        return (
                          <td key={r} style={{ textAlign: 'right', padding: '6px 8px', color: v ? theme.text : theme.textFaint }}>
                            {v || '—'}
                          </td>
                        );
                      })}
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </>
        )}
      </section>

      {/* Panel 2 — ambiguous prompt surface */}
      <section style={cardStyle()}>
        <header style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'baseline', marginBottom: 10 }}>
          <h3 style={{ margin: 0, fontSize: 13, color: theme.text }}>Ambiguous prompt surface — most-missing signals</h3>
          <span style={{ fontSize: 11, color: theme.textFaint }}>{(missing?.buckets ?? []).length} signal types</span>
        </header>
        {(missing?.buckets?.length ?? 0) === 0 ? (
          <p style={{ margin: 0, fontSize: 12, color: theme.textFaint }}>
            {isLoading ? 'Loading…' : 'No ask-backs in this window — either no ambiguous prompts, or no rows yet.'}
          </p>
        ) : (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
            {(missing!.buckets).map(b => {
              const max = Math.max(...missing!.buckets.map(x => x.count), 1);
              const pct = (b.count / max) * 100;
              return (
                <div key={b.signal} style={{ display: 'grid', gridTemplateColumns: '100px 1fr 60px', alignItems: 'center', gap: 8, fontSize: 12 }}>
                  <span style={{ color: theme.text, textTransform: 'capitalize' }}>{b.signal}</span>
                  <div style={{ height: 8, background: theme.surfaceAlt, borderRadius: 99, overflow: 'hidden' }}>
                    <div style={{ width: `${pct}%`, height: '100%', background: theme.yellow }} />
                  </div>
                  <span style={{ color: theme.textMuted, textAlign: 'right' }}>{b.count}</span>
                </div>
              );
            })}
          </div>
        )}
      </section>

      {/* Panel 3 — conflict candidates */}
      <section style={cardStyle()}>
        <header style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'baseline', marginBottom: 10 }}>
          <h3 style={{ margin: 0, fontSize: 13, color: theme.text }}>Conflict candidates (heuristic)</h3>
          <span style={{ fontSize: 11, color: theme.textFaint }}>
            FP {conflicts?.false_positive_count ?? 0} · FN {conflicts?.false_negative_count ?? 0}
          </span>
        </header>
        {(conflicts?.false_positive_count ?? 0) === 0 && (conflicts?.false_negative_count ?? 0) === 0 ? (
          <p style={{ margin: 0, fontSize: 12, color: theme.textFaint }}>
            {isLoading ? 'Loading…' : 'No flagged routings in this window.'}
          </p>
        ) : (
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(320px, 1fr))', gap: 12 }}>
            <ConflictPanel title="False-positive deterministic" subtitle="dispatched, then user followed up with retry/cancel" tone="red" items={conflicts?.false_positive_examples ?? []} />
            <ConflictPanel title="False-negative deterministic" subtitle="brain asked, but the prompt was actually self-sufficient" tone="yellow" items={conflicts?.false_negative_examples ?? []} />
          </div>
        )}
      </section>

      {/* Panel 4 — recent feed */}
      <section style={cardStyle()}>
        <header style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'baseline', marginBottom: 10 }}>
          <h3 style={{ margin: 0, fontSize: 13, color: theme.text }}>Recent prompts</h3>
          <span style={{ fontSize: 11, color: theme.textFaint }}>{recent.length} rows</span>
        </header>
        {recent.length === 0 ? (
          <p style={{ margin: 0, fontSize: 12, color: theme.textFaint }}>
            {isLoading ? 'Loading…' : 'No prompts in this window.'}
          </p>
        ) : (
          <div style={{ overflowX: 'auto' }}>
            <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 11 }}>
              <thead>
                <tr style={{ borderBottom: `1px solid ${theme.border}` }}>
                  {['when', 'route', 'reason', 'action', 'prompt', 'missing'].map(h => (
                    <th key={h} style={{ textAlign: 'left', padding: '6px 8px', color: theme.textMuted, fontWeight: 600 }}>{h}</th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {recent.map(r => <RecentRow key={r.id} row={r} />)}
              </tbody>
            </table>
          </div>
        )}
      </section>
    </div>
  );
}

function ConflictPanel({ title, subtitle, tone, items }: Readonly<{ title: string; subtitle: string; tone: 'red' | 'yellow'; items: ConflictRow[] }>) {
  const accent = tone === 'red' ? theme.red : theme.yellow;
  return (
    <div style={{ border: `1px solid ${theme.borderAlt}`, borderRadius: 8, padding: 10 }}>
      <div style={{ display: 'flex', alignItems: 'baseline', gap: 6, marginBottom: 6 }}>
        <span style={{ width: 8, height: 8, borderRadius: 99, background: accent }} />
        <strong style={{ fontSize: 12, color: theme.text }}>{title}</strong>
        <span style={{ fontSize: 11, color: theme.textFaint, marginLeft: 'auto' }}>{items.length}</span>
      </div>
      <p style={{ margin: '0 0 8px', fontSize: 10, color: theme.textFaint }}>{subtitle}</p>
      {items.length === 0 ? (
        <p style={{ margin: 0, fontSize: 11, color: theme.textFaint }}>—</p>
      ) : (
        <ul style={{ margin: 0, paddingLeft: 0, listStyle: 'none', display: 'flex', flexDirection: 'column', gap: 8 }}>
          {items.slice(0, 6).map((c, i) => (
            <li key={i} style={{ borderTop: i === 0 ? 'none' : `1px solid ${theme.borderAlt}`, paddingTop: i === 0 ? 0 : 6 }}>
              <div style={{ fontSize: 11, color: theme.text }}>{shortenPrompt(c.row.user_prompt, 100)}</div>
              <div style={{ fontSize: 10, color: theme.textFaint, marginTop: 2 }}>
                {c.row.action_taken} · {c.row.reason_code} · {formatRelative(c.row.created_at)}
                {c.follow_up_prompt && (
                  <> → <span style={{ color: accent }}>follow-up {c.follow_up_at_rel} later: "{shortenPrompt(c.follow_up_prompt, 60)}"</span></>
                )}
              </div>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

function RecentRow({ row }: Readonly<{ row: PromptRoutingRow }>) {
  const missing = (row.missing_signals ?? []).join(', ');
  return (
    <tr style={{ borderBottom: `1px solid ${theme.borderAlt}` }}>
      <td style={{ padding: '6px 8px', color: theme.textMuted, whiteSpace: 'nowrap' }}>{formatRelative(row.created_at)}</td>
      <td style={{ padding: '6px 8px' }}>
        <span style={{ display: 'inline-block', width: 6, height: 6, borderRadius: 99, background: routeColor(row.route), marginRight: 6, verticalAlign: 'middle' }} />
        <span style={{ color: theme.text }}>{row.route}</span>
      </td>
      <td style={{ padding: '6px 8px', color: theme.text }}>{row.reason_code}</td>
      <td style={{ padding: '6px 8px', color: theme.textMuted, fontFamily: 'var(--font-mono)' }}>{row.action_taken || '—'}</td>
      <td style={{ padding: '6px 8px', color: theme.text, maxWidth: 360 }}>{shortenPrompt(row.user_prompt)}</td>
      <td style={{ padding: '6px 8px', color: theme.yellow }}>{missing || '—'}</td>
    </tr>
  );
}
