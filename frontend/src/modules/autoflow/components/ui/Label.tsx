import React from 'react';

interface LabelProps { text: string; }

export const Label = React.memo(({ text }: LabelProps) => (
  <p style={{ color: '#8190aa', fontSize: 12, fontWeight: 700, marginBottom: 5 }}>{text}</p>
));
Label.displayName = 'Label';
