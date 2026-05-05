import type { CSSProperties } from 'react';

/**
 * Compatibility shim. Historical components used inline styles via the
 * `theme` object below; the new design system (see /design-system/)
 * exposes the canonical palette via CSS variables on :root. This file
 * re-points every legacy key at a CSS var() so anything still referencing
 * `theme.bg` etc renders with the new tokens until that component is
 * rewritten to use semantic class names from components.css.
 *
 * Do NOT add new entries here. New code should reach for class names
 * (`.btn .btn-primary`, `.card`, `.tag-hot`, …) or `var(--…)` directly.
 */
export const theme = {
  bg:           'var(--bg)',
  bgSoft:       'var(--bg-elev)',
  surface:      'var(--bg-elev-2)',
  surfaceAlt:   'var(--bg-elev)',
  surfaceHot:   'rgba(255,255,255,0.06)',
  border:       'var(--line-strong)',
  borderAlt:    'var(--line)',
  text:         'var(--text)',
  textMuted:    'var(--text-mute)',
  textFaint:    'var(--text-faint)',
  textWhite:    'var(--text)',
  primary:      'var(--accent)',
  primaryDark:  'var(--accent)',
  primaryLight: 'var(--accent)',
  primaryPale:  'var(--accent-soft)',
  green:        'var(--ok)',
  greenDark:    'var(--ok)',
  red:          'var(--hot)',
  yellow:       'var(--warn)',
  blue:         'var(--info)',
  info:         'var(--info)',
  secondary:    'var(--text-faint)',
  facebook:     'var(--info)',
  focus:        'var(--accent-soft)',
  shadow:       '0 24px 80px rgba(0,0,0,0.34)',
  glow:         'none',
};

const STATUS_COLORS: Record<string, string> = {
  Hot:        'var(--hot)',
  Warm:       'var(--warn)',
  Cold:       'var(--info)',
  Active:     'var(--ok)',
  Converted:  'var(--accent)',
  Pending:    'var(--text-faint)',
  pending:    'var(--text-faint)',
  synced:     'var(--ok)',
  error:      'var(--hot)',
  needs_auth: 'var(--warn)',
  Live:       'var(--ok)',
  Ended:      'var(--text-faint)',
  Suspended:  'var(--hot)',
  Enterprise: 'var(--warn)',
  Pro:        'var(--accent)',
  Starter:    'var(--text-mute)',
};

export const statusColor = (s: string): string => STATUS_COLORS[s] ?? 'var(--text-faint)';

export const rootStyle: CSSProperties = {
  background: 'var(--bg)',
  color: 'var(--text)',
  fontFamily: 'var(--font-sans)',
};

export const cardStyle = (overrides: CSSProperties = {}): CSSProperties => ({
  background: 'var(--bg-elev)',
  border: '1px solid var(--line)',
  borderRadius: 'var(--radius-lg)',
  padding: 24,
  ...overrides,
});

export const inputStyle: CSSProperties = {
  background: 'var(--bg-elev-2)',
  border: '1px solid var(--line)',
  borderRadius: 'var(--radius-md)',
  padding: '10px 14px',
  color: 'var(--text)',
  fontSize: 14,
  outline: 'none',
  width: '100%',
  boxSizing: 'border-box',
};

export const primaryBtn = (overrides: CSSProperties = {}): CSSProperties => ({
  padding: '11px 18px',
  minHeight: 38,
  display: 'inline-flex',
  alignItems: 'center',
  justifyContent: 'center',
  gap: 8,
  borderRadius: 'var(--radius-pill)',
  border: '1px solid var(--accent)',
  cursor: 'pointer',
  fontSize: 14,
  fontWeight: 600,
  lineHeight: 1.1,
  textAlign: 'center',
  whiteSpace: 'nowrap',
  background: 'var(--accent)',
  color: 'var(--accent-ink)',
  transition: 'transform 0.15s ease, box-shadow 0.25s ease',
  ...overrides,
});

export const secondaryBtn = (overrides: CSSProperties = {}): CSSProperties => ({
  padding: '11px 18px',
  minHeight: 38,
  display: 'inline-flex',
  alignItems: 'center',
  justifyContent: 'center',
  gap: 8,
  borderRadius: 'var(--radius-pill)',
  border: '1px solid var(--line-strong)',
  cursor: 'pointer',
  fontSize: 14,
  fontWeight: 500,
  lineHeight: 1.1,
  textAlign: 'center',
  whiteSpace: 'nowrap',
  background: 'transparent',
  color: 'var(--text)',
  transition: 'transform 0.15s ease, border-color 0.25s ease, background 0.25s ease',
  ...overrides,
});

export const rowStyle: CSSProperties = {
  display: 'flex',
  alignItems: 'center',
};

export const tableHeaderCell: CSSProperties = {
  padding: '12px 14px',
  textAlign: 'left',
  color: 'var(--text-faint)',
  fontWeight: 400,
  fontSize: 11,
  fontFamily: 'var(--font-mono)',
  letterSpacing: '0.1em',
  textTransform: 'uppercase',
};

export const tableCell: CSSProperties = {
  padding: '12px 14px',
};
