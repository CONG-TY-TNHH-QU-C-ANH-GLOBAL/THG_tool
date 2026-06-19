'use client';
import { useState } from 'react';
import {
  alpha,
  cardStyle,
  primaryBtn,
  secondaryBtn,
  statusColor,
} from '../../autoflow/constants/styles';
import { useKnowledgeStrings } from '../i18n/useKnowledgeStrings';
import { MOCK_SOURCES } from '../mock/fixtures';
import type { HealthStatus, KnowledgeSource, SourceType } from '../types';
import { formatNumber, formatRelativeTime } from '../utils/format';

const SOURCE_ICONS: Record<SourceType, string> = {
  shopify: 'S',
  csv: 'C',
  google_sheets: 'G',
  notion: 'N',
  website: 'W',
  catalog: 'K',
};

const SOURCE_TINTS: Record<SourceType, string> = {
  shopify: 'oklch(72% 0.18 145)',
  csv: 'var(--info)',
  google_sheets: 'var(--ok)',
  notion: 'var(--text-faint)',
  website: 'var(--accent)',
  catalog: 'var(--warn)',
};

const HEALTH_COLOR: Record<HealthStatus, string> = {
  healthy: 'var(--ok)',
  syncing: 'var(--info)',
  stale: 'var(--warn)',
  error: 'var(--hot)',
  needs_auth: 'var(--warn)',
};

export function SourcesPanel() {
  const { t, lang } = useKnowledgeStrings();
  const [sources] = useState<KnowledgeSource[]>(MOCK_SOURCES);
  const [pickerOpen, setPickerOpen] = useState(false);

  return (
    <section style={{ display: 'flex', flexDirection: 'column', gap: 20 }}>
      <header style={panelHeaderStyle}>
        <div>
          <h2 style={panelTitleStyle}>{t.sources.title}</h2>
          <p style={panelSubtitleStyle}>{t.sources.subtitle}</p>
        </div>
        <button
          type="button"
          style={primaryBtn()}
          onClick={() => setPickerOpen((v) => !v)}
        >
          {t.sources.addCta}
        </button>
      </header>

      {pickerOpen && (
        <div
          style={{
            ...cardStyle({ padding: 18 }),
            display: 'grid',
            gridTemplateColumns: 'repeat(auto-fill, minmax(180px, 1fr))',
            gap: 12,
          }}
        >
          <div style={{ gridColumn: '1 / -1', fontSize: 13, color: 'var(--text-mute)' }}>
            {t.sources.pickType}
          </div>
          {(Object.keys(SOURCE_ICONS) as SourceType[]).map((type) => (
            <button
              key={type}
              type="button"
              style={typePickerStyle(type)}
              onClick={() => setPickerOpen(false)}
            >
              <span style={typeIconStyle(type)}>{SOURCE_ICONS[type]}</span>
              <span style={{ fontWeight: 600 }}>{t.sources.types[type]}</span>
            </button>
          ))}
        </div>
      )}

      {sources.length === 0 ? (
        <EmptyState title={t.sources.emptyTitle} desc={t.sources.emptyDesc} />
      ) : (
        <div
          style={{
            display: 'grid',
            gap: 16,
            gridTemplateColumns: 'repeat(auto-fill, minmax(360px, 1fr))',
          }}
        >
          {sources.map((s) => (
            <SourceCard key={s.id} source={s} lang={lang} />
          ))}
        </div>
      )}
    </section>
  );
}

function SourceCard({ source, lang }: Readonly<{ source: KnowledgeSource; lang: 'vi' | 'en' }>) {
  const { t } = useKnowledgeStrings();
  const healthC = HEALTH_COLOR[source.health_status];
  const tint = SOURCE_TINTS[source.type];

  return (
    <article
      style={{
        ...cardStyle({ padding: 0, overflow: 'hidden' }),
        display: 'flex',
        flexDirection: 'column',
        borderColor: source.health_status === 'error' ? alpha('var(--hot)', 40) : 'var(--line)',
      }}
    >
      <div
        style={{
          padding: '18px 20px',
          display: 'flex',
          alignItems: 'flex-start',
          gap: 14,
          borderBottom: '1px solid var(--line)',
          background: alpha(tint, 4),
        }}
      >
        <div style={typeIconStyle(source.type)}>{SOURCE_ICONS[source.type]}</div>
        <div style={{ flex: 1, minWidth: 0 }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap' }}>
            <div style={{ fontSize: 15, fontWeight: 700, color: 'var(--text)' }}>
              {source.label}
            </div>
            <HealthChip status={source.health_status} label={t.sources[`health_${source.health_status}`]} />
          </div>
          <div
            style={{
              fontSize: 12,
              color: 'var(--text-faint)',
              fontFamily: 'var(--font-mono)',
              marginTop: 4,
              overflow: 'hidden',
              textOverflow: 'ellipsis',
              whiteSpace: 'nowrap',
            }}
            title={source.connection_hint}
          >
            {source.connection_hint}
          </div>
        </div>
      </div>

      {source.error_message && (
        <div
          style={{
            padding: '10px 20px',
            fontSize: 12,
            color: 'var(--hot)',
            background: alpha('var(--hot)', 6),
            borderBottom: '1px solid var(--line)',
          }}
        >
          {source.error_message}
        </div>
      )}

      <dl
        style={{
          padding: '14px 20px',
          display: 'grid',
          gridTemplateColumns: '1fr 1fr 1fr',
          gap: 12,
          margin: 0,
        }}
      >
        <MetaCell label={t.sources.assets} value={formatNumber(source.asset_count)} />
        <MetaCell
          label={t.sources.syncPolicy}
          value={t.sources.policies[source.sync_policy]}
        />
        <MetaCell label={t.sources.lastSync} value={formatRelativeTime(source.last_sync_at, lang)} />
      </dl>

      <footer
        style={{
          padding: '14px 20px',
          borderTop: '1px solid var(--line)',
          display: 'flex',
          gap: 8,
          flexWrap: 'wrap',
        }}
      >
        <button
          type="button"
          style={{ ...secondaryBtn({ padding: '7px 14px', minHeight: 32, fontSize: 12 }) }}
        >
          {t.sources.syncNow}
        </button>
        <button
          type="button"
          style={{ ...secondaryBtn({ padding: '7px 14px', minHeight: 32, fontSize: 12 }) }}
        >
          {t.sources.configure}
        </button>
        <button
          type="button"
          style={{
            ...secondaryBtn({
              padding: '7px 14px',
              minHeight: 32,
              fontSize: 12,
              color: 'var(--hot)',
              borderColor: alpha('var(--hot)', 30),
            }),
            marginLeft: 'auto',
          }}
        >
          {t.sources.disconnect}
        </button>
      </footer>
      <span style={{ display: 'none' }}>{healthC}</span>
    </article>
  );
}

function HealthChip({ status, label }: Readonly<{ status: HealthStatus; label: string }>) {
  const c = HEALTH_COLOR[status];
  return (
    <span
      style={{
        display: 'inline-flex',
        alignItems: 'center',
        gap: 6,
        background: alpha(c, 12),
        color: c,
        border: `1px solid ${alpha(c, 34)}`,
        padding: '3px 9px',
        borderRadius: 99,
        fontSize: 11,
        fontWeight: 700,
        letterSpacing: '0.04em',
      }}
    >
      <span
        style={{
          width: 6,
          height: 6,
          borderRadius: 99,
          background: c,
          boxShadow: status === 'syncing' ? `0 0 0 4px ${alpha(c, 18)}` : 'none',
          animation: status === 'syncing' ? 'knowledgePulse 1.5s ease-in-out infinite' : undefined,
        }}
      />
      {label}
    </span>
  );
}

function MetaCell({ label, value }: Readonly<{ label: string; value: string }>) {
  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
      <span
        style={{
          fontSize: 10,
          fontFamily: 'var(--font-mono)',
          letterSpacing: '0.08em',
          textTransform: 'uppercase',
          color: 'var(--text-faint)',
        }}
      >
        {label}
      </span>
      <span style={{ fontSize: 13, fontWeight: 600, color: 'var(--text)' }}>{value}</span>
    </div>
  );
}

function EmptyState({ title, desc }: Readonly<{ title: string; desc: string }>) {
  return (
    <div
      style={{
        ...cardStyle({ padding: 48 }),
        display: 'flex',
        flexDirection: 'column',
        alignItems: 'center',
        textAlign: 'center',
        gap: 8,
      }}
    >
      <div style={{ fontSize: 16, fontWeight: 700, color: 'var(--text)' }}>{title}</div>
      <div style={{ fontSize: 13, color: 'var(--text-mute)', maxWidth: 420 }}>{desc}</div>
    </div>
  );
}

const panelHeaderStyle = {
  display: 'flex',
  alignItems: 'flex-end',
  justifyContent: 'space-between',
  gap: 24,
  flexWrap: 'wrap' as const,
};
const panelTitleStyle = {
  fontSize: 20,
  fontWeight: 700,
  color: 'var(--text)',
  margin: 0,
  letterSpacing: '-0.01em',
};
const panelSubtitleStyle = {
  fontSize: 14,
  color: 'var(--text-mute)',
  margin: '4px 0 0',
  maxWidth: 640,
  lineHeight: 1.5,
};

function typeIconStyle(type: SourceType) {
  const tint = SOURCE_TINTS[type];
  return {
    width: 36,
    height: 36,
    borderRadius: 10,
    background: alpha(tint, 14),
    color: tint,
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    fontWeight: 800,
    fontSize: 16,
    fontFamily: 'var(--font-mono)',
    border: `1px solid ${alpha(tint, 24)}`,
    flexShrink: 0,
  } as const;
}

function typePickerStyle(type: SourceType) {
  const tint = SOURCE_TINTS[type];
  return {
    padding: 12,
    display: 'flex',
    alignItems: 'center',
    gap: 10,
    border: `1px solid ${alpha(tint, 18)}`,
    borderRadius: 'var(--radius-md)',
    background: alpha(tint, 4),
    color: 'var(--text)',
    cursor: 'pointer',
    transition: 'background 0.15s ease, border-color 0.15s ease',
    fontSize: 13,
    textAlign: 'left' as const,
  };
}
