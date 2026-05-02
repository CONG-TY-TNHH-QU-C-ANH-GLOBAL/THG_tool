import React from 'react';
import { statusColor } from '../../constants/styles';

interface BadgeProps { label: string; }

export const Badge = React.memo(({ label }: BadgeProps) => {
  const c = statusColor(label);
  return (
    <span style={{
      background: c + '24', color: c, border: `1px solid ${c}55`,
      fontSize: 11, fontWeight: 750, padding: '2px 9px', borderRadius: 99,
      boxShadow: `0 0 18px ${c}18`,
    }}>
      {label}
    </span>
  );
});
Badge.displayName = 'Badge';
