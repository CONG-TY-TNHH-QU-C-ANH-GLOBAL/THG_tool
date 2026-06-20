'use client';

import { Clock, Pause, Play, Archive, Pencil, Trash2 } from 'lucide-react';
import type { CrawlIntent } from '../../services/crawlIntentService';
import { useLang } from '../../i18n/useLang';
import type { DashboardStrings } from '../../i18n/strings';

interface MissionCardProps {
  intent: CrawlIntent;
  variant?: 'compact' | 'full';
  busy?: boolean;
  onPause?: (intent: CrawlIntent) => void;
  onResume?: (intent: CrawlIntent) => void;
  onArchive?: (intent: CrawlIntent) => void;
  onEditInterval?: (intent: CrawlIntent) => void;
  onDelete?: (intent: CrawlIntent) => void;
}

function scheduleLabel(value: string | undefined, tm: DashboardStrings['missionsView'], tc: DashboardStrings['chatView'], locale: string) {
  if (!value) return '—';
  const timestamp = new Date(value).getTime();
  if (!Number.isFinite(timestamp)) return '—';
  const diff = timestamp - Date.now();
  if (diff <= 0) return tc.schedulePending;
  const minutes = Math.ceil(diff / 60000);
  if (minutes < 60) return tc.scheduleInMinutes(minutes);
  return new Date(value).toLocaleString(locale, {
    hour: '2-digit',
    minute: '2-digit',
    day: '2-digit',
    month: '2-digit',
  });
  void tm; // kept for future per-mission overrides
}

function statusPill(status: CrawlIntent['status'], tm: DashboardStrings['missionsView']) {
  switch (status) {
    case 'paused':   return { label: tm.statusPaused,   color: 'var(--text-mute)' };
    case 'archived': return { label: tm.statusArchived, color: 'var(--text-faint)' };
    case 'failed':   return { label: tm.statusFailed,   color: 'var(--hot)' };
    case 'cooldown': return { label: tm.statusCooldown, color: 'var(--info)' };
    default:         return { label: tm.statusActive,   color: 'var(--ok)' };
  }
}

export default function MissionCard({ intent, variant = 'full', busy, onPause, onResume, onArchive, onEditInterval, onDelete }: Readonly<MissionCardProps>) {
  const { lang, t } = useLang();
  const locale = lang === 'vi' ? 'vi-VN' : 'en-US';
  const tm = t.missionsView;
  const pill = statusPill(intent.status, tm);
  const isActive = intent.status === 'active';
  const isPaused = intent.status === 'paused';
  const isArchived = intent.status === 'archived';
  const showActions = variant === 'full' && (onPause || onResume || onArchive || onEditInterval || onDelete);

  if (variant === 'compact') {
    return (
      <div style={{ borderTop: '1px solid var(--line)', paddingTop: 10, marginTop: 10 }}>
        <div style={{ fontSize: 12, fontWeight: 500, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
          {intent.name || intent.source_type}
        </div>
        <div className="mono" style={{ fontSize: 11, color: 'var(--text-faint)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', marginTop: 3 }}>
          {intent.source_url}
        </div>
        <div style={{ marginTop: 6, display: 'flex', alignItems: 'center', justifyContent: 'space-between', fontSize: 11, gap: 8 }}>
          <span style={{ display: 'flex', alignItems: 'center', gap: 5, color: 'var(--text-mute)' }}>
            <Clock size={11} />
            {t.chatView.automationEvery(intent.interval_minutes)}
          </span>
          <span className="mono" style={{ color: intent.last_error ? 'var(--hot)' : 'var(--ok)' }}>
            {intent.last_error ? t.chatView.automationError : scheduleLabel(intent.next_run_at, tm, t.chatView, locale)}
          </span>
        </div>
      </div>
    );
  }

  return (
    <div className="card" style={{ padding: 16, display: 'flex', flexDirection: 'column', gap: 10 }}>
      <header style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 12 }}>
        <div style={{ minWidth: 0, flex: 1 }}>
          <div style={{ fontSize: 14, fontWeight: 600, color: 'var(--text)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
            {intent.name || intent.source_label || intent.source_type}
          </div>
          <a
            href={intent.source_url}
            target="_blank"
            rel="noreferrer noopener"
            className="mono"
            style={{ fontSize: 11, color: 'var(--text-faint)', textDecoration: 'none', display: 'block', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', marginTop: 2 }}
          >
            {intent.source_url}
          </a>
        </div>
        <span
          className="mono"
          style={{
            fontSize: 10,
            letterSpacing: '0.08em',
            padding: '3px 8px',
            borderRadius: 999,
            background: 'var(--surface-alt)',
            color: pill.color,
            whiteSpace: 'nowrap',
          }}
        >
          {pill.label.toUpperCase()}
        </span>
      </header>

      {intent.prompt && (
        <p style={{ fontSize: 12.5, color: 'var(--text-mute)', lineHeight: 1.5, margin: 0, display: '-webkit-box', WebkitLineClamp: 2, WebkitBoxOrient: 'vertical', overflow: 'hidden' }}>
          {intent.prompt}
        </p>
      )}

      <dl style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 6, margin: 0, fontSize: 11 }}>
        <div>
          <dt style={{ color: 'var(--text-faint)' }}>{tm.fieldInterval}</dt>
          <dd className="mono" style={{ margin: 0, color: 'var(--text)' }}>
            <Clock size={10} style={{ verticalAlign: 'middle', marginRight: 4 }} />
            {t.chatView.automationEvery(intent.interval_minutes)}
          </dd>
        </div>
        <div>
          <dt style={{ color: 'var(--text-faint)' }}>{tm.fieldNextRun}</dt>
          <dd className="mono" style={{ margin: 0, color: intent.last_error ? 'var(--hot)' : 'var(--text)' }}>
            {intent.last_error ? t.chatView.automationError : scheduleLabel(intent.next_run_at, tm, t.chatView, locale)}
          </dd>
        </div>
      </dl>

      {intent.last_error && (
        <div style={{ fontSize: 11, color: 'var(--hot)', background: 'rgba(220,40,40,0.08)', padding: '6px 8px', borderRadius: 6 }}>
          <span style={{ color: 'var(--text-faint)', marginRight: 6 }}>{tm.fieldLastError}:</span>
          {intent.last_error}
        </div>
      )}

      {showActions && (
        <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap', marginTop: 2 }}>
          {isActive && onPause && (
            <button type="button" className="btn btn-ghost btn-sm" onClick={() => onPause(intent)} disabled={busy}>
              <Pause size={12} /> {tm.pauseCta}
            </button>
          )}
          {(isPaused || isArchived) && onResume && (
            <button type="button" className="btn btn-ghost btn-sm" onClick={() => onResume(intent)} disabled={busy}>
              <Play size={12} /> {tm.resumeCta}
            </button>
          )}
          {onEditInterval && (
            <button type="button" className="btn btn-ghost btn-sm" onClick={() => onEditInterval(intent)} disabled={busy}>
              <Pencil size={12} /> {tm.editIntervalCta}
            </button>
          )}
          {!isArchived && onArchive && (
            <button type="button" className="btn btn-ghost btn-sm" onClick={() => onArchive(intent)} disabled={busy} style={{ color: 'var(--hot)' }}>
              <Archive size={12} /> {tm.archiveCta}
            </button>
          )}
          {onDelete && (
            <button type="button" className="btn btn-ghost btn-sm" onClick={() => onDelete(intent)} disabled={busy} style={{ color: 'var(--hot)' }}>
              <Trash2 size={12} /> {tm.deleteCta}
            </button>
          )}
        </div>
      )}
    </div>
  );
}
