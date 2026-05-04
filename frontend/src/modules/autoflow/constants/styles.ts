import type { CSSProperties } from 'react';

export const theme = {
  bg:         '#07111f',
  bgSoft:     '#0b1730',
  surface:    'rgba(255, 255, 255, 0.078)',
  surfaceAlt: 'rgba(255, 255, 255, 0.052)',
  surfaceHot: 'rgba(255, 255, 255, 0.12)',
  border:     'rgba(255, 255, 255, 0.16)',
  borderAlt:  'rgba(255, 255, 255, 0.09)',
  text:       '#f7fbff',
  textMuted:  '#b8c6dc',
  textFaint:  '#8190aa',
  textWhite:  '#ffffff',
  primary:    '#1856FF',
  primaryDark:'#0f3bc4',
  primaryLight:'#7da4ff',
  primaryPale: '#d7e4ff',
  green:      '#07CA6B',
  greenDark:  '#079a55',
  red:        '#EA2143',
  yellow:     '#E89558',
  blue:       '#38bdf8',
  info:       '#38bdf8',
  secondary:  '#3A344E',
  facebook:   '#1877f2',
  focus:      'rgba(24, 86, 255, 0.34)',
  shadow:     '0 24px 80px rgba(0, 0, 0, 0.34)',
  glow:       '0 0 0 1px rgba(255,255,255,0.08) inset, 0 22px 70px rgba(24, 86, 255, 0.14)',
};

const STATUS_COLORS: Record<string, string> = {
  Hot:        theme.red,
  Warm:       theme.yellow,
  Cold:       theme.blue,
  Active:     theme.green,
  Converted:  theme.primaryLight,
  Pending:    theme.textFaint,
  pending:    theme.textFaint,
  synced:     theme.green,
  error:      theme.red,
  needs_auth: theme.yellow,
  Live:       theme.green,
  Ended:      theme.textFaint,
  Suspended:  theme.red,
  Enterprise: theme.yellow,
  Pro:        theme.primaryLight,
  Starter:    theme.textMuted,
};

export const statusColor = (s: string): string => STATUS_COLORS[s] ?? theme.textFaint;

export const rootStyle: CSSProperties = {
  background: `radial-gradient(circle at 12% 8%, rgba(24, 86, 255, 0.24), transparent 34%), radial-gradient(circle at 88% 18%, rgba(7, 202, 107, 0.16), transparent 30%), linear-gradient(135deg, ${theme.bg} 0%, ${theme.bgSoft} 52%, #050812 100%)`,
  color: theme.text,
  fontFamily: '"Plus Jakarta Sans", Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif',
};

export const cardStyle = (overrides: CSSProperties = {}): CSSProperties => ({
  background: theme.surface,
  border: `1px solid ${theme.border}`,
  borderRadius: 8,
  padding: 20,
  boxShadow: theme.glow,
  backdropFilter: 'blur(18px) saturate(142%)',
  WebkitBackdropFilter: 'blur(18px) saturate(142%)',
  ...overrides,
});

export const inputStyle: CSSProperties = {
  background: 'rgba(255, 255, 255, 0.075)',
  border: `1px solid ${theme.border}`,
  borderRadius: 8,
  padding: '10px 14px',
  color: theme.textWhite,
  fontSize: 13,
  outline: 'none',
  width: '100%',
  boxSizing: 'border-box',
  boxShadow: '0 1px 0 rgba(255,255,255,0.08) inset',
};

export const primaryBtn = (overrides: CSSProperties = {}): CSSProperties => ({
  padding: '10px 20px',
  minHeight: 40,
  display: 'inline-flex',
  alignItems: 'center',
  justifyContent: 'center',
  gap: 8,
  borderRadius: 8,
  border: `1px solid rgba(255,255,255,0.18)`,
  cursor: 'pointer',
  fontSize: 14,
  fontWeight: 750,
  lineHeight: 1.1,
  textAlign: 'center',
  whiteSpace: 'nowrap',
  background: `linear-gradient(135deg, ${theme.primary} 0%, #3b82ff 100%)`,
  color: theme.textWhite,
  boxShadow: '0 14px 34px rgba(24, 86, 255, 0.28)',
  transition: 'transform 160ms ease, box-shadow 160ms ease, border-color 160ms ease',
  ...overrides,
});

export const secondaryBtn = (overrides: CSSProperties = {}): CSSProperties => ({
  padding: '10px 20px',
  minHeight: 40,
  display: 'inline-flex',
  alignItems: 'center',
  justifyContent: 'center',
  gap: 8,
  borderRadius: 8,
  border: `1px solid ${theme.border}`,
  cursor: 'pointer',
  fontSize: 13,
  fontWeight: 750,
  lineHeight: 1.1,
  textAlign: 'center',
  whiteSpace: 'nowrap',
  background: 'rgba(255, 255, 255, 0.055)',
  color: theme.textMuted,
  backdropFilter: 'blur(12px) saturate(130%)',
  WebkitBackdropFilter: 'blur(12px) saturate(130%)',
  transition: 'transform 160ms ease, border-color 160ms ease, background 160ms ease',
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
