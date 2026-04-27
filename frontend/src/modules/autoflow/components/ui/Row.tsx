import React from 'react';

interface RowProps extends React.HTMLAttributes<HTMLDivElement> {
  children: React.ReactNode;
}

export const Row = ({ children, style, ...rest }: RowProps) => (
  <div style={{ display: 'flex', alignItems: 'center', ...style }} {...rest}>
    {children}
  </div>
);
