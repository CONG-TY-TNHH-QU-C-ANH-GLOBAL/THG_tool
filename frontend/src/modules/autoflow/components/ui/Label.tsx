import React from 'react';

interface LabelProps { text: string; }

export const Label = React.memo(({ text }: LabelProps) => (
  <p style={{ color: '#9ca3af', fontSize: 12, marginBottom: 5 }}>{text}</p>
));
Label.displayName = 'Label';
