import React from 'react';
import { alpha, statusColor } from '../../constants/styles';

interface BadgeProps { label: string; }

export const Badge = React.memo(({ label }: BadgeProps) => {
  const c = statusColor(label);
  return (
    <span style={{
      background: alpha(c, 14), color: c, border: `1px solid ${alpha(c, 34)}`,
      fontSize: 11, fontWeight: 750, padding: '2px 9px', borderRadius: 99,
      boxShadow: `0 0 18px ${alpha(c, 10)}`,
    }}>
      {label}
    </span>
  );
});
Badge.displayName = 'Badge';
