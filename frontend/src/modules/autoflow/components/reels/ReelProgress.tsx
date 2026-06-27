'use client';

import type { ReelStrings } from '../../i18n/reelStrings';

interface ReelProgressProps {
  done: number;
  total: number;
  cost: number;
  tr: ReelStrings;
}

// ReelProgress renders the shot render progress bar + accrued cost. No cancel control —
// rendering is the spend-committed phase (money invariant).
export default function ReelProgress({ done, total, cost, tr }: Readonly<ReelProgressProps>) {
  const pct = total > 0 ? Math.round((done / total) * 100) : 0;
  return (
    <div style={{ marginBottom: 4 }}>
      <div className="mono" style={{ fontSize: 11, color: 'var(--text-mute)', marginBottom: 6 }}>
        {tr.renderingLabel(done, total, cost.toFixed(2))}
      </div>
      <div style={{ height: 6, borderRadius: 999, background: 'var(--bg-elev)', overflow: 'hidden' }}>
        <div style={{ width: `${pct}%`, height: '100%', background: 'var(--warm, #ffb454)', transition: 'width .4s' }} />
      </div>
    </div>
  );
}
