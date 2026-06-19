'use client';
import { useMemo, useState } from 'react';
import {
  alpha,
  cardStyle,
  inputStyle,
  secondaryBtn,
} from '../../autoflow/constants/styles';
import { useKnowledgeStrings } from '../i18n/useKnowledgeStrings';
import { MOCK_ASSETS } from '../mock/fixtures';
import type { AssetState, AssetType, KnowledgeAsset } from '../types';
import { formatNumber, formatPercent, formatRelativeTime } from '../utils/format';

const STATE_FILTERS: Array<AssetState | 'all'> = ['all', 'approved', 'pending', 'hidden'];

const TYPE_TINT: Record<AssetType, string> = {
  POD_product: 'var(--accent)',
  faq: 'var(--info)',
  shipping_policy: 'var(--ok)',
  sales_playbook: 'var(--warn)',
  pricing_rule: 'oklch(72% 0.18 145)',
  banned_claim: 'var(--hot)',
  cta: 'oklch(74% 0.16 320)',
};

export function ProductExplorer() {
  const { t, lang } = useKnowledgeStrings();
  const [assets, setAssets] = useState<KnowledgeAsset[]>(MOCK_ASSETS);
  const [filter, setFilter] = useState<AssetState | 'all'>('all');
  const [search, setSearch] = useState('');

  const visible = useMemo(() => {
    const q = search.trim().toLowerCase();
    return assets
      .filter((a) => (filter === 'all' ? true : a.state === filter))
      .filter((a) => {
        if (!q) return true;
        return (
          a.title.toLowerCase().includes(q) ||
          a.source_label.toLowerCase().includes(q) ||
          a.tags.some((tag) => tag.toLowerCase().includes(q))
        );
      })
      .sort((a, b) => {
        if (a.pinned !== b.pinned) return a.pinned ? -1 : 1;
        if (a.boost !== b.boost) return b.boost - a.boost;
        return b.retrieval_count_30d - a.retrieval_count_30d;
      });
  }, [assets, filter, search]);

  function patch(id: string, updater: (a: KnowledgeAsset) => KnowledgeAsset) {
    setAssets((prev) => prev.map((a) => (a.id === id ? updater(a) : a)));
  }

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
            {t.products.title}
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
            {t.products.subtitle}
          </p>
        </div>
      </header>

      <div
        style={{
          display: 'flex',
          gap: 12,
          flexWrap: 'wrap',
          alignItems: 'center',
        }}
      >
        <input
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder={t.products.searchPlaceholder}
          style={{ ...inputStyle, maxWidth: 360 }}
        />
        <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap' }}>
          {STATE_FILTERS.map((s) => {
            const active = filter === s;
            const label =
              s === 'all'
                ? t.products.filterAll
                : s === 'approved'
                  ? t.products.filterApproved
                  : s === 'pending'
                    ? t.products.filterPending
                    : t.products.filterHidden;
            return (
              <button
                key={s}
                type="button"
                onClick={() => setFilter(s)}
                style={chipStyle(active)}
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
                  {s === 'all'
                    ? assets.length
                    : assets.filter((a) => a.state === s).length}
                </span>
              </button>
            );
          })}
        </div>
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
            {t.products.emptyTitle}
          </div>
          <div style={{ fontSize: 13, color: 'var(--text-mute)', maxWidth: 420 }}>
            {t.products.emptyDesc}
          </div>
        </div>
      ) : (
        <div
          style={{
            display: 'grid',
            gap: 14,
            gridTemplateColumns: 'repeat(auto-fill, minmax(320px, 1fr))',
          }}
        >
          {visible.map((asset) => (
            <AssetCard
              key={asset.id}
              asset={asset}
              lang={lang}
              onPatch={(updater) => patch(asset.id, updater)}
            />
          ))}
        </div>
      )}
    </section>
  );
}

function AssetCard({
  asset,
  lang,
  onPatch,
}: Readonly<{
  asset: KnowledgeAsset;
  lang: 'vi' | 'en';
  onPatch: (updater: (a: KnowledgeAsset) => KnowledgeAsset) => void;
}>) {
  const { t } = useKnowledgeStrings();
  const tint = TYPE_TINT[asset.type];
  const isHidden = asset.state === 'hidden';

  return (
    <article
      style={{
        ...cardStyle({ padding: 0 }),
        display: 'flex',
        flexDirection: 'column',
        opacity: isHidden ? 0.62 : 1,
        borderColor: asset.pinned ? alpha('var(--accent)', 50) : 'var(--line)',
        boxShadow: asset.pinned ? `0 0 0 1px ${alpha('var(--accent)', 18)}` : undefined,
      }}
    >
      <div
        style={{
          padding: '14px 16px',
          display: 'flex',
          alignItems: 'flex-start',
          gap: 12,
          borderBottom: '1px solid var(--line)',
        }}
      >
        <div
          style={{
            width: 8,
            alignSelf: 'stretch',
            background: tint,
            borderRadius: 99,
            flexShrink: 0,
          }}
        />
        <div style={{ flex: 1, minWidth: 0 }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 6, flexWrap: 'wrap' }}>
            <span
              style={{
                fontSize: 10,
                fontFamily: 'var(--font-mono)',
                letterSpacing: '0.08em',
                textTransform: 'uppercase',
                background: alpha(tint, 14),
                color: tint,
                padding: '2px 7px',
                borderRadius: 99,
                border: `1px solid ${alpha(tint, 24)}`,
              }}
            >
              {t.products[`asset_${asset.type}`]}
            </span>
            {asset.pinned && (
              <span
                style={{
                  fontSize: 10,
                  fontFamily: 'var(--font-mono)',
                  letterSpacing: '0.08em',
                  textTransform: 'uppercase',
                  color: 'var(--accent)',
                  background: alpha('var(--accent)', 12),
                  padding: '2px 7px',
                  borderRadius: 99,
                  border: `1px solid ${alpha('var(--accent)', 30)}`,
                }}
              >
                ★ {t.products.pinned}
              </span>
            )}
            <span style={{ marginLeft: 'auto', fontSize: 11, color: 'var(--text-faint)' }}>
              {asset.source_label}
            </span>
          </div>
          <div
            style={{
              fontSize: 14,
              fontWeight: 700,
              color: 'var(--text)',
              marginTop: 8,
              lineHeight: 1.35,
            }}
          >
            {asset.title}
          </div>
          <div
            style={{
              fontSize: 12,
              color: 'var(--text-mute)',
              marginTop: 6,
              lineHeight: 1.45,
            }}
          >
            {asset.description}
          </div>
          {asset.price && (
            <div
              style={{
                marginTop: 8,
                fontSize: 12,
                color: 'var(--ok)',
                fontFamily: 'var(--font-mono)',
              }}
            >
              {asset.price}
            </div>
          )}
        </div>
      </div>

      <div
        style={{
          padding: '10px 16px',
          display: 'flex',
          flexWrap: 'wrap',
          gap: 6,
          borderBottom: '1px solid var(--line)',
        }}
      >
        {asset.tags.map((tag) => (
          <span
            key={tag}
            style={{
              fontSize: 11,
              color: 'var(--text-mute)',
              background: 'var(--bg-elev-2)',
              padding: '2px 8px',
              borderRadius: 99,
              border: '1px solid var(--line)',
            }}
          >
            #{tag}
          </span>
        ))}
      </div>

      <div
        style={{
          padding: '12px 16px',
          display: 'grid',
          gridTemplateColumns: '1fr 1fr 1fr',
          gap: 10,
          borderBottom: '1px solid var(--line)',
        }}
      >
        <Metric
          label={t.products.retrievals}
          value={formatNumber(asset.retrieval_count_30d)}
        />
        <Metric
          label={t.products.conversions}
          value={formatNumber(asset.conversion_count_30d)}
          accent={asset.conversion_count_30d > 0 ? 'var(--accent)' : undefined}
        />
        <Metric
          label="CVR"
          value={formatPercent(asset.conversion_count_30d, asset.retrieval_count_30d)}
        />
      </div>

      <div style={{ padding: '10px 16px', display: 'flex', alignItems: 'center', gap: 10 }}>
        <BoostSlider
          value={asset.boost}
          onChange={(v) => onPatch((a) => ({ ...a, boost: v }))}
          label={t.products.boostLabel}
        />
      </div>

      <footer
        style={{
          padding: '12px 16px',
          borderTop: '1px solid var(--line)',
          display: 'flex',
          gap: 6,
          flexWrap: 'wrap',
          alignItems: 'center',
        }}
      >
        {asset.state === 'pending' && (
          <button
            type="button"
            style={miniBtn('var(--accent)')}
            onClick={() => onPatch((a) => ({ ...a, state: 'approved' }))}
          >
            ✓ {t.products.approve}
          </button>
        )}
        <button
          type="button"
          style={miniBtn(asset.pinned ? 'var(--text-mute)' : 'var(--accent)')}
          onClick={() => onPatch((a) => ({ ...a, pinned: !a.pinned }))}
        >
          {asset.pinned ? `☆ ${t.products.unpin}` : `★ ${t.products.pin}`}
        </button>
        <button
          type="button"
          style={miniBtn(isHidden ? 'var(--text-mute)' : 'var(--hot)')}
          onClick={() =>
            onPatch((a) => ({ ...a, state: isHidden ? 'approved' : 'hidden' }))
          }
        >
          {isHidden ? t.products.unhide : t.products.hide}
        </button>
        <span
          style={{
            marginLeft: 'auto',
            fontSize: 11,
            color: 'var(--text-faint)',
          }}
        >
          {formatRelativeTime(asset.updated_at, lang)}
        </span>
      </footer>
    </article>
  );
}

function Metric({
  label,
  value,
  accent,
}: Readonly<{
  label: string;
  value: string;
  accent?: string;
}>) {
  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
      <span
        style={{
          fontSize: 9,
          fontFamily: 'var(--font-mono)',
          letterSpacing: '0.08em',
          textTransform: 'uppercase',
          color: 'var(--text-faint)',
        }}
      >
        {label}
      </span>
      <span
        style={{
          fontSize: 14,
          fontWeight: 700,
          color: accent ?? 'var(--text)',
          fontFamily: 'var(--font-mono)',
        }}
      >
        {value}
      </span>
    </div>
  );
}

function BoostSlider({
  value,
  onChange,
  label,
}: Readonly<{
  value: number;
  onChange: (v: number) => void;
  label: string;
}>) {
  return (
    <label style={{ display: 'flex', alignItems: 'center', gap: 10, flex: 1 }}>
      <span
        style={{
          fontSize: 10,
          fontFamily: 'var(--font-mono)',
          letterSpacing: '0.08em',
          textTransform: 'uppercase',
          color: 'var(--text-faint)',
          minWidth: 64,
        }}
      >
        {label}
      </span>
      <input
        type="range"
        min={0}
        max={100}
        value={value}
        onChange={(e) => onChange(Number(e.target.value))}
        style={{
          flex: 1,
          accentColor: 'var(--accent)',
        }}
      />
      <span
        style={{
          fontSize: 12,
          fontFamily: 'var(--font-mono)',
          fontWeight: 700,
          color: value > 0 ? 'var(--accent)' : 'var(--text-faint)',
          minWidth: 32,
          textAlign: 'right',
        }}
      >
        {value}
      </span>
    </label>
  );
}

function chipStyle(active: boolean) {
  return {
    padding: '7px 14px',
    borderRadius: 99,
    border: `1px solid ${active ? 'var(--accent)' : 'var(--line)'}`,
    background: active ? 'var(--accent)' : 'var(--bg-elev-2)',
    color: active ? 'var(--accent-ink)' : 'var(--text-mute)',
    fontSize: 12,
    fontWeight: 600,
    cursor: 'pointer',
    transition: 'background 0.15s ease, border-color 0.15s ease, color 0.15s ease',
  } as const;
}

function miniBtn(c: string) {
  return {
    ...secondaryBtn({
      padding: '6px 12px',
      minHeight: 28,
      fontSize: 11,
      color: c,
      borderColor: alpha(c, 30),
    }),
  };
}
