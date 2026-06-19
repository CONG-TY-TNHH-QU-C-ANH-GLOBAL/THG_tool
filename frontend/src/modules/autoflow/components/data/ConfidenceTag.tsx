'use client';

interface ConfidenceTagProps {
  confidence: number | undefined;
}

/**
 * Small inline badge that visualises how confident the AI is in an
 * auto-inferred field value. Three semantic bands:
 *
 *   ≥ 0.7  → "AI" cyan (high confidence, lifted from the source)
 *   0.4-0.7 → "Suy luận" indigo (inferred from context)
 *   < 0.4  → "Đoán" amber (weak guess, user should verify)
 *
 * When confidence is undefined or zero we render nothing — that means
 * either the field is empty or the user has edited it and now owns the
 * value.
 */
export default function ConfidenceTag({ confidence }: Readonly<ConfidenceTagProps>) {
  if (confidence == null || confidence <= 0) return null;

  const palette = (() => {
    if (confidence >= 0.7) return { label: 'AI', fg: '#06B6D4', bg: 'rgba(6,182,212,0.12)' };
    if (confidence >= 0.4) return { label: 'Suy luận', fg: '#4F46E5', bg: 'rgba(79,70,229,0.12)' };
    return { label: 'Đoán', fg: '#F59E0B', bg: 'rgba(245,158,11,0.14)' };
  })();

  return (
    <span style={{
      display: 'inline-flex',
      alignItems: 'center',
      gap: 4,
      padding: '1px 7px',
      borderRadius: 999,
      background: palette.bg,
      color: palette.fg,
      fontSize: 9.5,
      fontWeight: 700,
      letterSpacing: '0.05em',
      textTransform: 'uppercase',
      verticalAlign: 'middle',
    }}>
      {palette.label}
      <span style={{ opacity: 0.7, fontWeight: 500 }}>{Math.round(confidence * 100)}%</span>
    </span>
  );
}
