'use client';
import { useState } from 'react';
import {
  alpha,
  cardStyle,
  secondaryBtn,
} from '../../autoflow/constants/styles';
import { useKnowledgeStrings } from '../i18n/useKnowledgeStrings';
import { MOCK_REPLAY_EVENTS } from '../mock/fixtures';
import type { ReplayEvent } from '../types';
import { formatRelativeTime } from '../utils/format';

const OUTCOME_COLOR: Record<ReplayEvent['outcome'], string> = {
  queued: 'var(--info)',
  approved: 'var(--accent)',
  rejected: 'var(--hot)',
  sent: 'var(--ok)',
  failed: 'var(--hot)',
};

export function OperatorReplay() {
  const { t, lang } = useKnowledgeStrings();
  const [events] = useState<ReplayEvent[]>(MOCK_REPLAY_EVENTS);
  const [filter, setFilter] = useState<ReplayEvent['outcome'] | 'all'>('all');
  const [expandedId, setExpandedId] = useState<string | null>(events[0]?.id ?? null);

  const visible = filter === 'all' ? events : events.filter((e) => e.outcome === filter);

  return (
    <section style={{ display: 'flex', flexDirection: 'column', gap: 20 }}>
      <header
        style={{
          display: 'flex',
          alignItems: 'flex-end',
          justifyContent: 'space-between',
          gap: 24,
          flexWrap: 'wrap',
        }}
      >
        <div>
          <h2 style={{ fontSize: 20, fontWeight: 700, margin: 0, color: 'var(--text)' }}>
            {t.replay.title}
          </h2>
          <p
            style={{
              fontSize: 14,
              color: 'var(--text-mute)',
              margin: '4px 0 0',
              maxWidth: 640,
              lineHeight: 1.5,
            }}
          >
            {t.replay.subtitle}
          </p>
        </div>
      </header>

      <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap' }}>
        {(['all', 'sent', 'approved', 'queued', 'rejected', 'failed'] as const).map((outcome) => {
          const active = filter === outcome;
          const label =
            outcome === 'all'
              ? t.products.filterAll
              : t.replay.outcomes[outcome];
          const count =
            outcome === 'all' ? events.length : events.filter((e) => e.outcome === outcome).length;
          return (
            <button
              key={outcome}
              type="button"
              onClick={() => setFilter(outcome)}
              style={{
                padding: '7px 14px',
                borderRadius: 99,
                border: `1px solid ${active ? 'var(--accent)' : 'var(--line)'}`,
                background: active ? 'var(--accent)' : 'var(--bg-elev-2)',
                color: active ? 'var(--accent-ink)' : 'var(--text-mute)',
                fontSize: 12,
                fontWeight: 600,
                cursor: 'pointer',
              }}
            >
              {label}
              <span
                style={{
                  marginLeft: 6,
                  fontFamily: 'var(--font-mono)',
                  fontSize: 11,
                  color: active ? 'var(--accent-ink)' : 'var(--text-faint)',
                }}
              >
                {count}
              </span>
            </button>
          );
        })}
      </div>

      {visible.length === 0 ? (
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
          <div style={{ fontSize: 16, fontWeight: 700, color: 'var(--text)' }}>
            {t.replay.emptyTitle}
          </div>
          <div style={{ fontSize: 13, color: 'var(--text-mute)', maxWidth: 420 }}>
            {t.replay.emptyDesc}
          </div>
        </div>
      ) : (
        <ol
          style={{
            listStyle: 'none',
            padding: 0,
            margin: 0,
            display: 'flex',
            flexDirection: 'column',
            gap: 12,
          }}
        >
          {visible.map((event) => (
            <ReplayRow
              key={event.id}
              event={event}
              lang={lang}
              expanded={expandedId === event.id}
              onToggle={() =>
                setExpandedId((cur) => (cur === event.id ? null : event.id))
              }
            />
          ))}
        </ol>
      )}
    </section>
  );
}

function ReplayRow({
  event,
  lang,
  expanded,
  onToggle,
}: Readonly<{
  event: ReplayEvent;
  lang: 'vi' | 'en';
  expanded: boolean;
  onToggle: () => void;
}>) {
  const { t } = useKnowledgeStrings();
  const oc = OUTCOME_COLOR[event.outcome];

  return (
    <li
      style={{
        ...cardStyle({ padding: 0, overflow: 'hidden' }),
        borderColor:
          event.outcome === 'rejected' || event.outcome === 'failed'
            ? alpha('var(--hot)', 32)
            : 'var(--line)',
      }}
    >
      <button
        type="button"
        onClick={onToggle}
        style={{
          width: '100%',
          padding: '14px 18px',
          background: 'transparent',
          border: 'none',
          color: 'inherit',
          textAlign: 'left',
          cursor: 'pointer',
          display: 'flex',
          alignItems: 'center',
          gap: 14,
        }}
      >
        <span
          style={{
            width: 4,
            alignSelf: 'stretch',
            background: oc,
            borderRadius: 99,
            flexShrink: 0,
          }}
        />
        <div style={{ flex: 1, minWidth: 0 }}>
          <div
            style={{
              display: 'flex',
              alignItems: 'center',
              gap: 8,
              flexWrap: 'wrap',
            }}
          >
            <span
              style={{
                fontSize: 10,
                fontFamily: 'var(--font-mono)',
                letterSpacing: '0.08em',
                textTransform: 'uppercase',
                color: 'var(--text-faint)',
              }}
            >
              {t.replay.actions[event.action]}
            </span>
            <span
              style={{
                fontSize: 10,
                fontFamily: 'var(--font-mono)',
                letterSpacing: '0.08em',
                textTransform: 'uppercase',
                color: oc,
                background: alpha(oc, 12),
                border: `1px solid ${alpha(oc, 28)}`,
                padding: '2px 8px',
                borderRadius: 99,
                fontWeight: 700,
              }}
            >
              {t.replay.outcomes[event.outcome]}
            </span>
            <span style={{ fontSize: 11, color: 'var(--text-faint)', marginLeft: 'auto' }}>
              {formatRelativeTime(event.occurred_at, lang)}
            </span>
          </div>
          <div
            style={{
              fontSize: 13,
              color: 'var(--text)',
              marginTop: 6,
              fontWeight: 600,
              lineHeight: 1.4,
              overflow: 'hidden',
              display: '-webkit-box',
              WebkitLineClamp: expanded ? 'unset' : 1,
              WebkitBoxOrient: 'vertical',
            }}
          >
            {event.lead_context}
          </div>
          {!expanded && (
            <div
              style={{
                fontSize: 12,
                color: 'var(--text-mute)',
                marginTop: 6,
                display: 'flex',
                gap: 6,
                alignItems: 'center',
                flexWrap: 'wrap',
              }}
            >
              <span style={{ color: 'var(--text-faint)' }}>{t.replay.retrievedAssets}:</span>
              {event.retrieved_assets.slice(0, 2).map((a) => (
                <span
                  key={a.asset_id}
                  style={{
                    fontSize: 11,
                    color: 'var(--text-mute)',
                    background: 'var(--bg-elev-2)',
                    padding: '2px 8px',
                    borderRadius: 99,
                    border: '1px solid var(--line)',
                  }}
                >
                  {a.asset_title}
                </span>
              ))}
              {event.retrieved_assets.length > 2 && (
                <span style={{ fontSize: 11, color: 'var(--text-faint)' }}>
                  +{event.retrieved_assets.length - 2}
                </span>
              )}
            </div>
          )}
        </div>
        <span
          style={{
            ...secondaryBtn({
              padding: '5px 11px',
              minHeight: 26,
              fontSize: 11,
            }),
          }}
        >
          {expanded ? t.replay.collapse : t.replay.expand}
        </span>
      </button>

      {expanded && (
        <div
          style={{
            padding: '0 18px 18px 36px',
            display: 'flex',
            flexDirection: 'column',
            gap: 14,
          }}
        >
          <DetailBlock
            label={t.replay.retrievedAssets}
            content={
              <ol
                style={{
                  listStyle: 'none',
                  padding: 0,
                  margin: 0,
                  display: 'flex',
                  flexDirection: 'column',
                  gap: 6,
                }}
              >
                {event.retrieved_assets.map((a, idx) => (
                  <li
                    key={a.asset_id}
                    style={{
                      display: 'flex',
                      alignItems: 'center',
                      gap: 10,
                      padding: '8px 10px',
                      borderRadius: 8,
                      background: 'var(--bg-elev-2)',
                      border: '1px solid var(--line)',
                    }}
                  >
                    <span
                      style={{
                        fontSize: 11,
                        fontFamily: 'var(--font-mono)',
                        color: 'var(--text-faint)',
                        minWidth: 24,
                      }}
                    >
                      #{idx + 1}
                    </span>
                    <span style={{ fontSize: 13, color: 'var(--text)', flex: 1, minWidth: 0 }}>
                      {a.asset_title}
                    </span>
                    <span
                      style={{
                        fontSize: 11,
                        fontFamily: 'var(--font-mono)',
                        color:
                          a.score >= 0.85 ? 'var(--accent)' : 'var(--text-mute)',
                        fontWeight: 700,
                      }}
                    >
                      {t.replay.score} {a.score.toFixed(2)}
                    </span>
                  </li>
                ))}
              </ol>
            }
          />
          <DetailBlock
            label={t.replay.generatedText}
            content={
              <blockquote
                style={{
                  margin: 0,
                  padding: '10px 14px',
                  borderLeft: `3px solid ${oc}`,
                  background: alpha(oc, 4),
                  fontSize: 13,
                  color: 'var(--text)',
                  fontStyle: 'italic',
                  lineHeight: 1.6,
                }}
              >
                {event.generated_text}
              </blockquote>
            }
          />
          {event.operator && (
            <DetailBlock
              label={t.replay.operator}
              content={
                <span
                  style={{
                    fontSize: 12,
                    fontFamily: 'var(--font-mono)',
                    color: 'var(--text-mute)',
                  }}
                >
                  {event.operator}
                </span>
              }
            />
          )}
        </div>
      )}
    </li>
  );
}

function DetailBlock({ label, content }: Readonly<{ label: string; content: React.ReactNode }>) {
  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
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
      {content}
    </div>
  );
}
