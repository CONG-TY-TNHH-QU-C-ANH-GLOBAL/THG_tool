import React from 'react';

interface AvatarProps {
  text: string;
  bg?: string;
  size?: number;
}

export const Avatar = React.memo(({ text, bg = '#4f46e5', size = 30 }: AvatarProps) => (
  <div style={{
    width: size, height: size, background: bg, borderRadius: '50%',
    display: 'flex', alignItems: 'center', justifyContent: 'center',
    color: '#fff', fontSize: size * 0.36, fontWeight: 700, flexShrink: 0,
  }}>
    {text}
  </div>
));
Avatar.displayName = 'Avatar';
