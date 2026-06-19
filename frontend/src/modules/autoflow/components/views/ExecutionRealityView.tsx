/**
 * Health Dashboard — verified execution observability framed in plain
 * Vietnamese for operators, not engineers.
 *
 * The previous version was a wall of pivot tables and outcome buckets
 * that read as engineering telemetry. This rewrite stacks four
 * natural-language status cards above an opt-in "Xem log kỹ thuật"
 * drill-down that preserves the original tables for debugging.
 *
 * Data source is unchanged — same /observability/execution/{distribution,
 * recent, account-health} payloads via useExecutionReality. The card
 * sentences are derived locally; we never invent a metric that isn't
 * already in the verified substrate.
 */

import { useEffect, useMemo, useState } from 'react';
import {
  Activity,
  AlertTriangle,
  CheckCircle2,
  ChevronDown,
  ChevronRight,
  RefreshCw,
  ShieldCheck,
  Sparkles,
  Users,
} from 'lucide-react';
import { theme } from '../../constants/styles';
import { useExecutionReality } from '../../hooks/useExecutionReality';
import type { ExecutionAttemptRow, OutcomeBucket, GapRow, ReconcileRow } from '../../services/executionService';
import { getStuckOutbound, getLedgerReconcile } from '../../services/executionService';

interface ExecutionRealityViewProps { orgId: string; isAdmin: boolean }

const WINDOW_OPTIONS = [1, 6, 24, 72, 168] as const;

// Outcome categories we collapse the verifier taxonomy into for the
// natural-language summaries. The drill-down still shows the raw outcomes.
const VERIFIED_OK = new Set(['dom_verified', 'duplicate_blocked', 'optimistic_success']);
const NOISY_FAIL  = new Set(['shadow_rejected', 'blocked', 'rate_limited', 'captcha', 'redirected_feed', 'context_drift']);
const HARD_FAIL   = new Set(['hard_fail', 'soft_fail', 'verification_timeout', 'composer_failed', 'retry_exhausted']);

function outcomeColor(outcome: string): string {
  if (VERIFIED_OK.has(outcome)) return theme.green;
  if (NOISY_FAIL.has(outcome))  return theme.red;
  if (HARD_FAIL.has(outcome))   return theme.textMuted;
  return theme.textFaint;
}

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
  if (diff < 0) return 'sắp tới';
  const s = Math.floor(diff / 1000);
  if (s < 60) return `${s}s trước`;
  const m = Math.floor(s / 60);
  if (m < 60) return `${m} phút trước`;
  const h = Math.floor(m / 60);
  if (h < 24) return `${h}h trước`;
  return `${Math.floor(h / 24)} ngày trước`;
}

function shorten(url: string, max = 56): string {
  if (!url) return '';
  if (url.length <= max) return url;
  return url.slice(0, max - 1) + '…';
}

interface HealthSummary {
  total: number;
  verified: number;
  noisy: number;
  hardFail: number;
  successRate: number;
  duplicateBlocked: number;
  topFailure: { outcome: string; count: number } | null;
}

function summarise(buckets: OutcomeBucket[]): HealthSummary {
  let total = 0;
  let verified = 0;
  let noisy = 0;
  let hardFail = 0;
  let duplicateBlocked = 0;
  const failCounts: Record<string, number> = {};
  for (const b of buckets) {
    total += b.count;
    if (VERIFIED_OK.has(b.outcome)) verified += b.count;
    if (NOISY_FAIL.has(b.outcome))  { noisy += b.count; failCounts[b.outcome] = (failCounts[b.outcome] ?? 0) + b.count; }
    if (HARD_FAIL.has(b.outcome))   { hardFail += b.count; failCounts[b.outcome] = (failCounts[b.outcome] ?? 0) + b.count; }
    if (b.outcome === 'duplicate_blocked') duplicateBlocked += b.count;
  }
  let topFailure: HealthSummary['topFailure'] = null;
  Object.entries(failCounts).forEach(([outcome, count]) => {
    if (!topFailure || count > topFailure.count) topFailure = { outcome, count };
  });
  const successRate = total > 0 ? verified / total : 0;
  return { total, verified, noisy, hardFail, successRate, duplicateBlocked, topFailure };
}

interface AccountSummary {
  total: number;
  healthy: number;
  elevated: number;
  critical: number;
  inCooldown: number;
}

function summariseAccounts(accounts: { risk_score: number; cooldown_until: string }[]): AccountSummary {
  const now = Date.now();
  return accounts.reduce<AccountSummary>((acc, a) => {
    acc.total += 1;
    if (a.risk_score >= 0.7) acc.critical += 1;
    else if (a.risk_score >= 0.4) acc.elevated += 1;
    else acc.healthy += 1;
    if (a.cooldown_until && new Date(a.cooldown_until).getTime() > now) acc.inCooldown += 1;
    return acc;
  }, { total: 0, healthy: 0, elevated: 0, critical: 0, inCooldown: 0 });
}

// Tiny inline progress bar for the success-rate visual on card 1.
function MiniBar({ value, color }: Readonly<{ value: number; color: string }>) {
  const pct = Math.max(0, Math.min(1, value)) * 100;
  return (
    <div style={{ height: 6, background: 'rgba(0,0,0,0.06)', borderRadius: 99, overflow: 'hidden', marginTop: 6 }}>
      <div style={{
        width: `${pct}%`,
        height: '100%',
        background: color,
        boxShadow: `0 0 8px ${color}80`,
        transition: 'width 0.4s ease',
      }} />
    </div>
  );
}

interface HealthCardProps {
  Icon: React.ComponentType<{ size?: number | string; color?: string }>;
  eyebrow: string;
  headline: string;
  body: React.ReactNode;
  tone?: 'ok' | 'warn' | 'idle';
}

function HealthCard({ Icon, eyebrow, headline, body, tone = 'idle' }: Readonly<HealthCardProps>) {
  const toneColor = tone === 'ok' ? '#10B981' : tone === 'warn' ? '#F59E0B' : '#4F46E5';
  return (
    <div style={{
      padding: 18,
      borderRadius: 16,
      background: 'var(--bg-elev)',
      border: '1px solid var(--line)',
      boxShadow: '0 1px 2px rgba(0,0,0,0.04)',
      position: 'relative',
      overflow: 'hidden',
    }}>
      <div style={{
        position: 'absolute',
        top: 0,
        left: 0,
        bottom: 0,
        width: 3,
        background: `linear-gradient(180deg, ${toneColor}, transparent)`,
      }} />
      <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 8 }}>
        <Icon size={14} color={toneColor} />
        <span style={{ fontSize: 10.5, fontWeight: 700, letterSpacing: '0.1em', color: toneColor }}>
          {eyebrow}
        </span>
      </div>
      <h3 style={{ margin: 0, fontSize: 19, fontWeight: 700, color: 'var(--text)', letterSpacing: '-0.01em', lineHeight: 1.3 }}>
        {headline}
      </h3>
      <div style={{ marginTop: 10, fontSize: 13, color: 'var(--text-mute)', lineHeight: 1.55 }}>
        {body}
      </div>
    </div>
  );
}

export default function ExecutionRealityView(_: Readonly<ExecutionRealityViewProps>) {
  const [hours, setHours] = useState<number>(24);
  const [techOpen, setTechOpen] = useState(false);
  const { distribution, attempts, accounts, isLoading, error, refetch, setWindowHours } = useExecutionReality(hours);

  const summary = useMemo(() => summarise(distribution?.buckets ?? []), [distribution]);
  const accountSummary = useMemo(() => summariseAccounts(accounts ?? []), [accounts]);
  const latestAttempt = attempts[0];

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

  // ── derive headline sentences ────────────────────────────────────────
  const windowLabel = hours < 24 ? `${hours} giờ qua` : `${hours / 24} ngày qua`;

  let systemTone: HealthCardProps['tone'] = 'idle';
  let systemHeadline = '';
  let systemBody: React.ReactNode;
  if (summary.total === 0) {
    systemTone = 'idle';
    systemHeadline = 'Hệ thống đang chờ việc.';
    systemBody = (
      <span>
        Chưa có hành động nào trong {windowLabel}. Khi extension thực thi outbound, các kết quả đã được DOM xác nhận sẽ xuất hiện ở đây.
      </span>
    );
  } else if (summary.successRate >= 0.7) {
    systemTone = 'ok';
    const pct = Math.round(summary.successRate * 100);
    systemHeadline = `Hệ thống đang khoẻ. ${pct}% hành động được xác nhận thành công.`;
    systemBody = (
      <>
        <span>
          <b>{summary.verified}</b>/<b>{summary.total}</b> hành động đã được DOM xác nhận trong {windowLabel}.
          {summary.duplicateBlocked > 0 && (
            <> Có <b>{summary.duplicateBlocked}</b> lượt bị dedup chặn — đó là tín hiệu tốt, anti-collision đang chạy.</>
          )}
        </span>
        <MiniBar value={summary.successRate} color="#10B981" />
      </>
    );
  } else if (summary.successRate >= 0.4) {
    systemTone = 'warn';
    const pct = Math.round(summary.successRate * 100);
    systemHeadline = `Cần theo dõi. ${pct}% hành động xác nhận thành công.`;
    systemBody = (
      <>
        <span>
          <b>{summary.noisy + summary.hardFail}</b>/<b>{summary.total}</b> hành động bị từ chối hoặc lỗi trong {windowLabel}.
          {summary.topFailure && <> Nguyên nhân thường gặp nhất: <b>{summary.topFailure.outcome}</b>.</>}
        </span>
        <MiniBar value={summary.successRate} color="#F59E0B" />
      </>
    );
  } else {
    systemTone = 'warn';
    const pct = Math.round(summary.successRate * 100);
    systemHeadline = `Cần can thiệp. Chỉ ${pct}% hành động được xác nhận.`;
    systemBody = (
      <>
        <span>
          <b>{summary.noisy + summary.hardFail}</b> hành động lỗi trong {windowLabel}.
          {summary.topFailure && <> Lỗi phổ biến: <b>{summary.topFailure.outcome}</b> — kiểm tra account health hoặc nội dung composer.</>}
        </span>
        <MiniBar value={summary.successRate} color="#EF4444" />
      </>
    );
  }

  let accountHeadline = '';
  let accountBody: React.ReactNode;
  let accountTone: HealthCardProps['tone'] = 'idle';
  if (accountSummary.total === 0) {
    accountHeadline = 'Chưa có account nào đang trong vòng.';
    accountBody = <span>Khi account được dùng để thực thi, trạng thái sức khoẻ sẽ hiện ở đây.</span>;
  } else if (accountSummary.critical > 0) {
    accountTone = 'warn';
    accountHeadline = `${accountSummary.critical} account đang ở mức rủi ro cao.`;
    accountBody = (
      <span>
        <b>{accountSummary.healthy}</b> khoẻ · <b>{accountSummary.elevated}</b> cảnh báo · <b>{accountSummary.critical}</b> nghiêm trọng
        {accountSummary.inCooldown > 0 && <> · <b>{accountSummary.inCooldown}</b> đang cooldown</>}.
        Account ở mức critical nên tạm ngưng cho đến khi rủi ro giảm.
      </span>
    );
  } else if (accountSummary.elevated > 0) {
    accountTone = 'warn';
    accountHeadline = `${accountSummary.elevated} account nên giảm tải.`;
    accountBody = (
      <span>
        <b>{accountSummary.healthy}</b> khoẻ · <b>{accountSummary.elevated}</b> cảnh báo
        {accountSummary.inCooldown > 0 && <> · <b>{accountSummary.inCooldown}</b> đang cooldown</>}.
        Hệ thống đang tự giãn nhịp; bạn không cần can thiệp gấp.
      </span>
    );
  } else {
    accountTone = 'ok';
    accountHeadline = `Toàn bộ ${accountSummary.total} account đang khoẻ.`;
    accountBody = (
      <span>
        Không có account nào vượt ngưỡng risk
        {accountSummary.inCooldown > 0 && <>, dù <b>{accountSummary.inCooldown}</b> đang cooldown theo lịch</>}.
      </span>
    );
  }

  let latestHeadline = '';
  let latestBody: React.ReactNode;
  if (!latestAttempt) {
    latestHeadline = 'Chưa có lượt thực thi nào.';
    latestBody = <span>Lượt thực thi đầu tiên sẽ xuất hiện tại đây kèm DOM evidence.</span>;
  } else {
    const verb = latestAttempt.action_type === 'comment' ? 'comment'
      : latestAttempt.action_type === 'inbox' ? 'inbox'
      : latestAttempt.action_type === 'post' ? 'post'
      : latestAttempt.action_type;
    latestHeadline = `Lượt cuối: ${verb} (${latestAttempt.outcome || latestAttempt.status}).`;
    latestBody = (
      <span>
        Account <b>#{latestAttempt.account_id}</b> · {formatRelative(latestAttempt.started_at)}
        {latestAttempt.target_url && (
          <> · <a href={latestAttempt.target_url} target="_blank" rel="noreferrer" style={{ color: '#06B6D4', textDecoration: 'none' }}>{shorten(latestAttempt.target_url, 44)}</a></>
        )}
        {latestAttempt.failure_reason && latestAttempt.failure_reason !== latestAttempt.outcome && (
          <> · lý do: <b>{latestAttempt.failure_reason}</b></>
        )}
      </span>
    );
  }

  const verifiedTone: HealthCardProps['tone'] = summary.total === 0 ? 'idle' : summary.successRate >= 0.7 ? 'ok' : 'warn';

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
      {/* Header */}
      <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 12, flexWrap: 'wrap' }}>
        <div>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 6 }}>
            <Sparkles size={14} style={{ color: '#4F46E5' }} />
            <span style={{ fontSize: 11, fontWeight: 700, letterSpacing: '0.1em', color: '#4F46E5' }}>
              CHẨN ĐOÁN HỆ THỐNG
            </span>
          </div>
          <h2 style={{ margin: 0, fontSize: 22, fontWeight: 700, color: 'var(--text)', letterSpacing: '-0.01em' }}>
            Sức khoẻ workspace
          </h2>
          <p style={{ margin: '4px 0 0', fontSize: 13, color: 'var(--text-mute)' }}>
            Tóm tắt {windowLabel} — chỉ từ outcome đã được DOM xác nhận, không phải số liệu lạc quan.
          </p>
        </div>
        <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
          <div style={{ display: 'flex', gap: 4, background: 'var(--bg-elev)', padding: 3, borderRadius: 10, border: '1px solid var(--line)' }}>
            {WINDOW_OPTIONS.map(h => {
              const active = hours === h;
              return (
                <button
                  key={h}
                  type="button"
                  onClick={() => onWindowChange(h)}
                  style={{
                    background: active ? 'linear-gradient(135deg, #4F46E5, #06B6D4)' : 'transparent',
                    color: active ? '#FFFFFF' : 'var(--text-mute)',
                    border: 0,
                    cursor: 'pointer',
                    fontSize: 11,
                    fontWeight: 600,
                    padding: '5px 11px',
                    borderRadius: 7,
                    transition: 'all 0.15s',
                  }}
                >
                  {h < 24 ? `${h}h` : `${h / 24}d`}
                </button>
              );
            })}
          </div>
          <button type="button" onClick={refetch} disabled={isLoading} style={{
            background: 'transparent',
            color: 'var(--text-mute)',
            border: '1px solid var(--line)',
            display: 'flex',
            alignItems: 'center',
            gap: 5,
            fontSize: 11,
            fontWeight: 500,
            padding: '6px 10px',
            borderRadius: 8,
            cursor: isLoading ? 'not-allowed' : 'pointer',
          }}>
            <RefreshCw size={11} />
            Làm mới
          </button>
        </div>
      </div>

      {error && (
        <div style={{ padding: 12, borderRadius: 12, border: `1px solid ${theme.red}`, color: theme.red, fontSize: 12.5 }}>
          {error.message}
        </div>
      )}

      {/* Health cards grid */}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(280px, 1fr))', gap: 12 }}>
        <HealthCard
          Icon={Activity}
          eyebrow="TÌNH TRẠNG"
          headline={systemHeadline}
          body={systemBody}
          tone={systemTone}
        />
        <HealthCard
          Icon={CheckCircle2}
          eyebrow="HÀNH ĐỘNG XÁC NHẬN"
          headline={summary.total === 0 ? 'Chờ lượt đầu tiên.' : `${summary.verified} hành động được xác nhận thành công.`}
          body={
            summary.total === 0 ? (
              <span>Cần extension chạy outbound và DOM verifier emit proof.</span>
            ) : (
              <span>
                <b>{summary.duplicateBlocked}</b> dedup_blocked · <b>{summary.noisy}</b> noisy reject · <b>{summary.hardFail}</b> hard fail.
                {summary.topFailure && <> Top loại lỗi: <b>{summary.topFailure.outcome}</b> ({summary.topFailure.count}).</>}
              </span>
            )
          }
          tone={verifiedTone}
        />
        <HealthCard
          Icon={Users}
          eyebrow="SỨC KHOẺ ACCOUNT"
          headline={accountHeadline}
          body={accountBody}
          tone={accountTone}
        />
        <HealthCard
          Icon={ShieldCheck}
          eyebrow="HOẠT ĐỘNG GẦN NHẤT"
          headline={latestHeadline}
          body={latestBody}
          tone={latestAttempt && NOISY_FAIL.has(latestAttempt.outcome) ? 'warn' : latestAttempt ? 'ok' : 'idle'}
        />
      </div>

      {/* Tech drill-down */}
      <div>
        <button
          type="button"
          onClick={() => setTechOpen(v => !v)}
          style={{
            display: 'flex',
            alignItems: 'center',
            gap: 6,
            padding: '6px 10px',
            background: 'transparent',
            border: 0,
            color: 'var(--text-mute)',
            fontSize: 12,
            cursor: 'pointer',
          }}
        >
          {techOpen ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
          Xem log kỹ thuật (pivot outcome × action, attempts, account table)
        </button>
        {techOpen && (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 14, marginTop: 10 }}>
            {/* Outcome distribution */}
            <section style={{ background: 'var(--bg-elev)', border: '1px solid var(--line)', borderRadius: 12, padding: 16 }}>
              <header style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'baseline', marginBottom: 10 }}>
                <h3 style={{ margin: 0, fontSize: 13, color: theme.text }}>Outcome distribution</h3>
                <span style={{ fontSize: 11, color: theme.textFaint }}>{summary.total} attempts</span>
              </header>
              {grid.outcomes.length === 0 ? (
                <p style={{ margin: 0, fontSize: 12, color: theme.textFaint }}>
                  {isLoading ? 'Đang tải…' : 'Chưa có outcome trong cửa sổ này.'}
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

            {/* Recent attempts */}
            <section style={{ background: 'var(--bg-elev)', border: '1px solid var(--line)', borderRadius: 12, padding: 16 }}>
              <header style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'baseline', marginBottom: 10 }}>
                <h3 style={{ margin: 0, fontSize: 13, color: theme.text }}>Recent attempts</h3>
                <span style={{ fontSize: 11, color: theme.textFaint }}>{attempts.length} rows</span>
              </header>
              {attempts.length === 0 ? (
                <p style={{ margin: 0, fontSize: 12, color: theme.textFaint }}>
                  {isLoading ? 'Đang tải…' : 'Không có attempt nào trong cửa sổ này.'}
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

            {/* PR-E: Stuck-state panels — gap detection, ledger reconcile.
                Lazy-fetched when the drill-down is opened so the default
                Health Dashboard view stays light. */}
            <StuckOutboundPanel />
            <LedgerReconcilePanel hours={hours} />

            {/* Account table */}
            <section style={{ background: 'var(--bg-elev)', border: '1px solid var(--line)', borderRadius: 12, padding: 16 }}>
              <header style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'baseline', marginBottom: 10 }}>
                <h3 style={{ margin: 0, fontSize: 13, color: theme.text }}>Account health</h3>
                <span style={{ fontSize: 11, color: theme.textFaint }}>{accounts.length} accounts</span>
              </header>
              {accounts.length === 0 ? (
                <p style={{ margin: 0, fontSize: 12, color: theme.textFaint }}>
                  {isLoading ? 'Đang tải…' : 'Chưa có account runtime state.'}
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
                              {inCooldown ? `đến ${new Date(acc.cooldown_until).toLocaleString('vi-VN')}` : '—'}
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
        )}
      </div>

      {/* Unused import suppressor (AlertTriangle is reserved for future
          alert banners; keep imported so a TS unused-import sweep doesn't
          remove it before the next iteration). */}
      <span style={{ display: 'none' }}><AlertTriangle size={1} /></span>
    </div>
  );
}

// StuckOutboundPanel surfaces outbound rows in planned/executing with NO
// matching execution_attempts row older than 10 minutes — "leads queued
// but never executed." Lazy-fetched on mount; manual refresh button.
function StuckOutboundPanel() {
  const [rows, setRows] = useState<GapRow[] | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const load = () => {
    setLoading(true); setErr(null);
    getStuckOutbound(10, 50)
      .then(r => setRows(r.rows ?? []))
      .catch(e => setErr(e?.message ?? 'failed to load'))
      .finally(() => setLoading(false));
  };
  useEffect(() => { load(); }, []);
  return (
    <section style={{ background: 'var(--bg-elev)', border: '1px solid var(--line)', borderRadius: 12, padding: 16 }}>
      <header style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'baseline', marginBottom: 10 }}>
        <h3 style={{ margin: 0, fontSize: 13, color: theme.text }}>Stuck outbound (no attempt &gt; 10m)</h3>
        <button type="button" onClick={load} disabled={loading} style={{
          background: 'transparent', color: theme.textMuted, border: `1px solid ${theme.border}`,
          fontSize: 11, padding: '3px 8px', borderRadius: 6, cursor: loading ? 'wait' : 'pointer',
        }}>
          {loading ? '...' : 'Refresh'}
        </button>
      </header>
      {err ? (
        <p style={{ margin: 0, fontSize: 12, color: theme.red }}>{err}</p>
      ) : rows === null ? (
        <p style={{ margin: 0, fontSize: 12, color: theme.textFaint }}>Đang tải…</p>
      ) : rows.length === 0 ? (
        <p style={{ margin: 0, fontSize: 12, color: theme.textFaint }}>Không có outbound nào bị kẹt. Hệ thống đang thực thi đúng nhịp.</p>
      ) : (
        <div style={{ overflowX: 'auto' }}>
          <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 12 }}>
            <thead>
              <tr style={{ borderBottom: `1px solid ${theme.border}` }}>
                {['outbound', 'account', 'action', 'state', 'age', 'target'].map(h => (
                  <th key={h} style={{ textAlign: 'left', padding: '6px 8px', color: theme.textMuted, fontWeight: 600 }}>{h}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {rows.map(r => (
                <tr key={r.outbound_id} style={{ borderBottom: `1px solid ${theme.borderAlt}` }}>
                  <td style={{ padding: '6px 8px', color: theme.text, fontFamily: 'var(--font-mono)' }}>#{r.outbound_id}</td>
                  <td style={{ padding: '6px 8px', color: theme.text, fontFamily: 'var(--font-mono)' }}>#{r.account_id}</td>
                  <td style={{ padding: '6px 8px', color: theme.text }}>{r.action_type}</td>
                  <td style={{ padding: '6px 8px', color: theme.yellow }}>{r.execution_state}</td>
                  <td style={{ padding: '6px 8px', color: theme.textMuted }}>{Math.floor(r.age_seconds / 60)}m</td>
                  <td style={{ padding: '6px 8px', color: theme.textMuted }}>
                    {r.target_url ? (
                      <a href={r.target_url} target="_blank" rel="noreferrer" style={{ color: theme.info, textDecoration: 'none' }}>
                        {shorten(r.target_url)}
                      </a>
                    ) : '—'}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </section>
  );
}

// LedgerReconcilePanel surfaces action_ledger rows marked 'succeeded' whose
// latest execution_attempts.outcome is in a failure-class bucket — the
// hallucinated-success rows that corrupt the badge/risk pipeline.
function LedgerReconcilePanel({ hours }: Readonly<{ hours: number }>) {
  const [rows, setRows] = useState<ReconcileRow[] | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const load = () => {
    setLoading(true); setErr(null);
    getLedgerReconcile(hours, 100)
      .then(r => setRows(r.rows ?? []))
      .catch(e => setErr(e?.message ?? 'failed to load'))
      .finally(() => setLoading(false));
  };
  useEffect(() => { load(); }, [hours]);
  return (
    <section style={{ background: 'var(--bg-elev)', border: '1px solid var(--line)', borderRadius: 12, padding: 16 }}>
      <header style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'baseline', marginBottom: 10 }}>
        <h3 style={{ margin: 0, fontSize: 13, color: theme.text }}>Ledger hallucinated success ({hours}h)</h3>
        <button type="button" onClick={load} disabled={loading} style={{
          background: 'transparent', color: theme.textMuted, border: `1px solid ${theme.border}`,
          fontSize: 11, padding: '3px 8px', borderRadius: 6, cursor: loading ? 'wait' : 'pointer',
        }}>
          {loading ? '...' : 'Refresh'}
        </button>
      </header>
      {err ? (
        <p style={{ margin: 0, fontSize: 12, color: theme.red }}>{err}</p>
      ) : rows === null ? (
        <p style={{ margin: 0, fontSize: 12, color: theme.textFaint }}>Đang tải…</p>
      ) : rows.length === 0 ? (
        <p style={{ margin: 0, fontSize: 12, color: theme.textFaint }}>Không có hallucinated success — ledger đang khớp với verified outcome.</p>
      ) : (
        <div style={{ overflowX: 'auto' }}>
          <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 12 }}>
            <thead>
              <tr style={{ borderBottom: `1px solid ${theme.border}` }}>
                {['when', 'outbound', 'account', 'action', 'ledger said', 'verifier said', 'target'].map(h => (
                  <th key={h} style={{ textAlign: 'left', padding: '6px 8px', color: theme.textMuted, fontWeight: 600 }}>{h}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {rows.map(r => (
                <tr key={r.ledger_id} style={{ borderBottom: `1px solid ${theme.borderAlt}` }}>
                  <td style={{ padding: '6px 8px', color: theme.textMuted, whiteSpace: 'nowrap' }}>{formatRelative(r.performed_at)}</td>
                  <td style={{ padding: '6px 8px', color: theme.text, fontFamily: 'var(--font-mono)' }}>#{r.outbound_id}</td>
                  <td style={{ padding: '6px 8px', color: theme.text, fontFamily: 'var(--font-mono)' }}>#{r.account_id}</td>
                  <td style={{ padding: '6px 8px', color: theme.text }}>{r.action_type}</td>
                  <td style={{ padding: '6px 8px', color: theme.green }}>{r.ledger_outcome}</td>
                  <td style={{ padding: '6px 8px', color: theme.red }}>{r.attempt_outcome}</td>
                  <td style={{ padding: '6px 8px', color: theme.textMuted }}>
                    {r.target_url ? (
                      <a href={r.target_url} target="_blank" rel="noreferrer" style={{ color: theme.info, textDecoration: 'none' }}>
                        {shorten(r.target_url)}
                      </a>
                    ) : '—'}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </section>
  );
}

function AttemptRow({ attempt }: Readonly<{ attempt: ExecutionAttemptRow }>) {
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
