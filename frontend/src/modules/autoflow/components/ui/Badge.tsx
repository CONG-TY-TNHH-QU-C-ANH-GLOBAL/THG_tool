import React from 'react';
import { statusColor } from '../../constants/styles';

interface BadgeProps { label: string; }

export const Badge = React.memo(({ label }: BadgeProps) => {
  const c = statusColor(label);
  return (
    <span style={{
      background: c + '22', color: c, border: `1px solid ${c}44`,
      fontSize: 11, fontWeight: 500, padding: '2px 9px', borderRadius: 99,
    }}>
      {label}
    </span>
  );
});
Badge.displayName = 'Badge';
