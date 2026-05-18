'use client';
import { useState } from 'react';
import { alpha } from '../../autoflow/constants/styles';
import { useKnowledgeStrings } from '../i18n/useKnowledgeStrings';
import { ComplianceCenter } from './ComplianceCenter';
import { OperatorReplay } from './OperatorReplay';
import { ProductExplorer } from './ProductExplorer';
import { SourcesPanel } from './SourcesPanel';

type Tab = 'sources' | 'products' | 'compliance' | 'replay';

export function KnowledgeHub() {
  const { t } = useKnowledgeStrings();
  const [tab, setTab] = useState<Tab>('sources');

  const tabs: Array<{ id: Tab; label: string }> = [
    { id: 'sources', label: t.tabs.sources },
    { id: 'products', label: t.tabs.products },
    { id: 'compliance', label: t.tabs.compliance },
    { id: 'replay', label: t.tabs.replay },
  ];

  return (
    <div
      style={{
        minHeight: '100vh',
        background: 'var(--bg)',
        color: 'var(--text)',
        fontFamily: 'var(--font-sans)',
      }}
    >
      <style>{`
        @keyframes knowledgePulse {
          0%, 100% { box-shadow: 0 0 0 4px color-mix(in oklch, var(--info) 18%, transparent); }
          50%      { box-shadow: 0 0 0 8px color-mix(in oklch, var(--info) 8%, transparent); }
        }
      `}</style>
      <div style={{ maxWidth: 1280, margin: '0 auto', padding: '32px 28px 64px' }}>
        <header style={{ marginBottom: 28 }}>
          <div
            style={{
              fontSize: 11,
              fontFamily: 'var(--font-mono)',
              letterSpacing: '0.12em',
              textTransform: 'uppercase',
              color: 'var(--accent)',
              marginBottom: 8,
            }}
          >
            Workspace Knowledge OS
          </div>
          <h1
            style={{
              fontSize: 32,
              fontWeight: 800,
              margin: 0,
              color: 'var(--text)',
              letterSpacing: '-0.02em',
              lineHeight: 1.15,
            }}
          >
            {t.hub.title}
          </h1>
          <p
            style={{
              fontSize: 15,
              color: 'var(--text-mute)',
              margin: '10px 0 0',
              maxWidth: 760,
              lineHeight: 1.55,
            }}
          >
            {t.hub.subtitle}
          </p>
          <div
            style={{
              marginTop: 18,
              padding: '10px 14px',
              borderRadius: 'var(--radius-md)',
              border: `1px solid ${alpha('var(--warn)', 28)}`,
              background: alpha('var(--warn)', 6),
              color: 'var(--warn)',
              fontSize: 12,
              maxWidth: 760,
              lineHeight: 1.5,
            }}
          >
            <strong style={{ fontWeight: 700 }}>UI preview · </strong>
            {t.hub.backendNotice}
          </div>
        </header>

        <nav
          style={{
            display: 'flex',
            gap: 4,
            marginBottom: 28,
            borderBottom: '1px solid var(--line)',
            overflowX: 'auto',
          }}
          role="tablist"
        >
          {tabs.map((tabDef) => {
            const active = tab === tabDef.id;
            return (
              <button
                key={tabDef.id}
                role="tab"
                aria-selected={active}
                onClick={() => setTab(tabDef.id)}
                style={{
                  padding: '12px 18px',
                  border: 'none',
                  borderBottom: `2px solid ${active ? 'var(--accent)' : 'transparent'}`,
                  background: 'transparent',
                  color: active ? 'var(--text)' : 'var(--text-mute)',
                  fontSize: 14,
                  fontWeight: active ? 700 : 500,
                  cursor: 'pointer',
                  transition: 'color 0.15s ease, border-color 0.15s ease',
                  whiteSpace: 'nowrap',
                }}
              >
                {tabDef.label}
              </button>
            );
          })}
        </nav>

        <main role="tabpanel">
          {tab === 'sources' && <SourcesPanel />}
          {tab === 'products' && <ProductExplorer />}
          {tab === 'compliance' && <ComplianceCenter />}
          {tab === 'replay' && <OperatorReplay />}
        </main>
      </div>
    </div>
  );
}
