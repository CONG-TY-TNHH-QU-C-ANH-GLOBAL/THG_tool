'use client';
import { useState } from 'react';
import {
  alpha,
  cardStyle,
  inputStyle,
  primaryBtn,
  secondaryBtn,
} from '../../autoflow/constants/styles';
import { useKnowledgeStrings } from '../i18n/useKnowledgeStrings';
import { MOCK_BANNED_CLAIMS } from '../mock/fixtures';
import type { BannedClaim, ClaimSeverity } from '../types';
import { formatNumber, formatRelativeTime } from '../utils/format';

export function ComplianceCenter() {
  const { t, lang } = useKnowledgeStrings();
  const [claims, setClaims] = useState<BannedClaim[]>(MOCK_BANNED_CLAIMS);
  const [formOpen, setFormOpen] = useState(false);
  const [draft, setDraft] = useState<{ pattern: string; reason: string; severity: ClaimSeverity }>({
    pattern: '',
    reason: '',
    severity: 'block',
  });

  function submit() {
    const pattern = draft.pattern.trim();
    const reason = draft.reason.trim();
    if (!pattern || !reason) return;
    setClaims((prev) => [
      {
        id: `ban_${Date.now()}`,
        pattern,
        reason,
        severity: draft.severity,
        added_by: 'you@workspace',
        added_at: new Date().toISOString(),
        trigger_count_30d: 0,
      },
      ...prev,
    ]);
    setDraft({ pattern: '', reason: '', severity: 'block' });
    setFormOpen(false);
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
            {t.compliance.title}
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
            {t.compliance.subtitle}
          </p>
        </div>
        <button
          type="button"
          style={primaryBtn()}
          onClick={() => setFormOpen((v) => !v)}
        >
          {t.compliance.addCta}
        </button>
      </header>

      {formOpen && (
        <div style={cardStyle({ padding: 20 })}>
          <div style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
            <Field
              label={t.compliance.patternLabel}
              input={
                <input
                  value={draft.pattern}
                  onChange={(e) => setDraft({ ...draft, pattern: e.target.value })}
                  placeholder={t.compliance.patternPlaceholder}
                  style={inputStyle}
                  autoFocus
                />
              }
            />
            <Field
              label={t.compliance.reasonLabel}
              input={
                <textarea
                  value={draft.reason}
                  onChange={(e) => setDraft({ ...draft, reason: e.target.value })}
                  placeholder={t.compliance.reasonPlaceholder}
                  rows={3}
                  style={{ ...inputStyle, resize: 'vertical', fontFamily: 'var(--font-sans)' }}
                />
              }
            />
            <Field
              label={t.compliance.severityLabel}
              input={
                <div style={{ display: 'flex', gap: 8 }}>
                  {(['block', 'warn'] as ClaimSeverity[]).map((sev) => {
                    const active = draft.severity === sev;
                    const c = sev === 'block' ? 'var(--hot)' : 'var(--warn)';
                    return (
                      <button
                        key={sev}
                        type="button"
                        onClick={() => setDraft({ ...draft, severity: sev })}
                        style={{
                          padding: '8px 16px',
                          borderRadius: 99,
                          border: `1px solid ${active ? c : 'var(--line)'}`,
                          background: active ? alpha(c, 14) : 'var(--bg-elev-2)',
                          color: active ? c : 'var(--text-mute)',
                          fontSize: 12,
                          fontWeight: 700,
                          cursor: 'pointer',
                        }}
                      >
                        {sev === 'block' ? t.compliance.severityBlock : t.compliance.severityWarn}
                      </button>
                    );
                  })}
                </div>
              }
            />
            <div style={{ display: 'flex', gap: 10, marginTop: 4 }}>
              <button type="button" style={primaryBtn()} onClick={submit}>
                {t.compliance.saveCta}
              </button>
              <button
                type="button"
                style={secondaryBtn()}
                onClick={() => {
                  setFormOpen(false);
                  setDraft({ pattern: '', reason: '', severity: 'block' });
                }}
              >
                {lang === 'vi' ? 'Huỷ' : 'Cancel'}
              </button>
            </div>
          </div>
        </div>
      )}

      {claims.length === 0 ? (
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
            {t.compliance.emptyTitle}
          </div>
          <div style={{ fontSize: 13, color: 'var(--text-mute)', maxWidth: 420 }}>
            {t.compliance.emptyDesc}
          </div>
        </div>
      ) : (
        <div style={cardStyle({ padding: 0, overflow: 'hidden' })}>
          <table style={{ width: '100%', borderCollapse: 'collapse' }}>
            <thead>
              <tr style={{ background: 'var(--bg-elev-2)' }}>
                <Th>{t.compliance.patternLabel}</Th>
                <Th>{t.compliance.reasonLabel}</Th>
                <Th>{t.compliance.severityLabel}</Th>
                <Th align="right">{t.compliance.triggers30d}</Th>
                <Th>{t.compliance.addedBy}</Th>
                <Th />
              </tr>
            </thead>
            <tbody>
              {claims.map((claim) => (
                <ClaimRow
                  key={claim.id}
                  claim={claim}
                  lang={lang}
                  onDelete={() =>
                    setClaims((prev) => prev.filter((c) => c.id !== claim.id))
                  }
                />
              ))}
            </tbody>
          </table>
        </div>
      )}
    </section>
  );
}

function ClaimRow({
  claim,
  lang,
  onDelete,
}: Readonly<{
  claim: BannedClaim;
  lang: 'vi' | 'en';
  onDelete: () => void;
}>) {
  const { t } = useKnowledgeStrings();
  const c = claim.severity === 'block' ? 'var(--hot)' : 'var(--warn)';
  return (
    <tr style={{ borderTop: '1px solid var(--line)' }}>
      <Td>
        <code
          style={{
            background: alpha(c, 10),
            border: `1px solid ${alpha(c, 24)}`,
            color: c,
            padding: '4px 10px',
            borderRadius: 6,
            fontSize: 12,
            fontFamily: 'var(--font-mono)',
            fontWeight: 700,
          }}
        >
          {claim.pattern}
        </code>
      </Td>
      <Td>
        <span style={{ fontSize: 13, color: 'var(--text-mute)', lineHeight: 1.5 }}>
          {claim.reason}
        </span>
      </Td>
      <Td>
        <span
          style={{
            display: 'inline-flex',
            alignItems: 'center',
            gap: 6,
            background: alpha(c, 12),
            color: c,
            border: `1px solid ${alpha(c, 30)}`,
            padding: '3px 9px',
            borderRadius: 99,
            fontSize: 11,
            fontWeight: 700,
            letterSpacing: '0.04em',
            textTransform: 'uppercase',
          }}
        >
          {claim.severity === 'block' ? t.compliance.severityBlock : t.compliance.severityWarn}
        </span>
      </Td>
      <Td align="right">
        <span
          style={{
            fontFamily: 'var(--font-mono)',
            fontSize: 13,
            fontWeight: 700,
            color: claim.trigger_count_30d > 0 ? 'var(--text)' : 'var(--text-faint)',
          }}
        >
          {formatNumber(claim.trigger_count_30d)}
        </span>
      </Td>
      <Td>
        <div style={{ display: 'flex', flexDirection: 'column' }}>
          <span style={{ fontSize: 12, color: 'var(--text-mute)' }}>{claim.added_by}</span>
          <span style={{ fontSize: 11, color: 'var(--text-faint)' }}>
            {formatRelativeTime(claim.added_at, lang)}
          </span>
        </div>
      </Td>
      <Td align="right">
        <button
          type="button"
          onClick={onDelete}
          style={{
            ...secondaryBtn({
              padding: '5px 11px',
              minHeight: 26,
              fontSize: 11,
              color: 'var(--hot)',
              borderColor: alpha('var(--hot)', 24),
            }),
          }}
        >
          {t.compliance.deleteCta}
        </button>
      </Td>
    </tr>
  );
}

function Field({ label, input }: Readonly<{ label: string; input: React.ReactNode }>) {
  return (
    <label style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
      <span
        style={{
          fontSize: 11,
          fontFamily: 'var(--font-mono)',
          letterSpacing: '0.08em',
          textTransform: 'uppercase',
          color: 'var(--text-faint)',
        }}
      >
        {label}
      </span>
      {input}
    </label>
  );
}

function Th({ children, align }: Readonly<{ children?: React.ReactNode; align?: 'left' | 'right' }>) {
  return (
    <th
      style={{
        padding: '12px 16px',
        textAlign: align ?? 'left',
        color: 'var(--text-faint)',
        fontWeight: 400,
        fontSize: 11,
        fontFamily: 'var(--font-mono)',
        letterSpacing: '0.1em',
        textTransform: 'uppercase',
        borderBottom: '1px solid var(--line)',
      }}
    >
      {children}
    </th>
  );
}

function Td({
  children,
  align,
}: Readonly<{
  children: React.ReactNode;
  align?: 'left' | 'right';
}>) {
  return (
    <td
      style={{
        padding: '14px 16px',
        textAlign: align ?? 'left',
        verticalAlign: 'top',
        color: 'var(--text)',
        fontSize: 13,
      }}
    >
      {children}
    </td>
  );
}
