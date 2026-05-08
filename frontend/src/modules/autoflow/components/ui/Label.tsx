import React from 'react';
import { theme } from '../../constants/styles';

interface LabelProps { text: string; }

export const Label = React.memo(({ text }: LabelProps) => (
  <p style={{ color: theme.textFaint, fontSize: 12, fontWeight: 700, marginBottom: 5 }}>{text}</p>
));
Label.displayName = 'Label';
