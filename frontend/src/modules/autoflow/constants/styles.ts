import type { CSSProperties } from 'react';

export const theme = {
  bg:         '#0d101a',
  surface:    '#1e2130',
  surfaceAlt: '#111520',
  border:     '#2a2f45',
  borderAlt:  '#1a1f35',
  text:       '#e5e7eb',
  textMuted:  '#9ca3af',
  textFaint:  '#6b7280',
  textWhite:  '#f9fafb',
  primary:    '#4f46e5',
  primaryLight:'#818cf8',
  primaryPale: '#a5b4fc',
  green:      '#22c55e',
  greenDark:  '#16a34a',
  red:        '#ef4444',
  yellow:     '#f59e0b',
  blue:       '#3b82f6',
  facebook:   '#1877f2',
};

const STATUS_COLORS: Record<string, string> = {
  Hot:        '#ef4444',
  Warm:       '#f59e0b',
  Cold:       '#3b82f6',
  Active:     '#22c55e',
  Converted:  '#6366f1',
  Pending:    '#6b7280',
  Live:       '#22c55e',
  Ended:      '#6b7280',
  Suspended:  '#ef4444',
  Enterprise: '#d97706',
  Pro:        '#6366f1',
  Starter:    '#9ca3af',
};

export const statusColor = (s: string): string => STATUS_COLORS[s] ?? '#6b7280';

export const rootStyle: CSSProperties = {
  background: theme.bg,
  color: theme.text,
  fontFamily: 'system-ui, sans-serif',
};

export const cardStyle = (overrides: CSSProperties = {}): CSSProperties => ({
  background: theme.surface,
  border: `1px solid ${theme.border}`,
  borderRadius: 12,
  padding: 20,
  ...overrides,
});

export const inputStyle: CSSProperties = {
  background: theme.border,
  border: `1px solid #374151`,
  borderRadius: 9,
  padding: '10px 14px',
  color: '#fff',
  fontSize: 13,
  outline: 'none',
  width: '100%',
  boxSizing: 'border-box',
};

export const primaryBtn = (overrides: CSSProperties = {}): CSSProperties => ({
  padding: '10px 20px',
  borderRadius: 9,
  border: 'none',
  cursor: 'pointer',
  fontSize: 14,
  fontWeight: 500,
  background: theme.primary,
  color: '#fff',
  ...overrides,
});

export const secondaryBtn = (overrides: CSSProperties = {}): CSSProperties => ({
  padding: '10px 20px',
  borderRadius: 9,
  border: '1px solid #374151',
  cursor: 'pointer',
  fontSize: 13,
  background: 'transparent',
  color: '#d1d5db',
  ...overrides,
});

export const rowStyle: CSSProperties = {
  display: 'flex',
  alignItems: 'center',
};

export const tableHeaderCell: CSSProperties = {
  padding: '9px 14px',
  textAlign: 'left',
  color: theme.textFaint,
  fontWeight: 500,
  fontSize: 11,
};

export const tableCell: CSSProperties = {
  padding: '9px 14px',
};
