import React from 'react';

interface AvatarProps {
  text: string;
  bg?: string;
  size?: number;
}

export const Avatar = React.memo(({ text, bg = '#1856FF', size = 30 }: AvatarProps) => (
  <div style={{
    width: size, height: size, background: `linear-gradient(135deg, ${bg}, #7da4ff)`, borderRadius: 8,
    display: 'flex', alignItems: 'center', justifyContent: 'center',
    color: '#fff', fontSize: size * 0.36, fontWeight: 700, flexShrink: 0,
    border: '1px solid rgba(255,255,255,0.22)',
    boxShadow: '0 12px 28px rgba(24,86,255,0.2)',
  }}>
    {text}
  </div>
));
Avatar.displayName = 'Avatar';
