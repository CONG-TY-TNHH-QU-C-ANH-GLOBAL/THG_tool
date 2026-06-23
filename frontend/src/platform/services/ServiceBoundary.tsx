'use client';
import { Suspense, type ReactNode } from 'react';
import { PlatformErrorBoundary } from '../components/PlatformErrorBoundary';

const DefaultFallback = () => (
  <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', height: '50vh' }}>
    <div className="skeleton" style={{ width: 240, height: 14 }} />
  </div>
);

export function ServiceBoundary({
  children,
  fallback,
}: Readonly<{
  children: ReactNode;
  fallback?: ReactNode;
}>) {
  return (
    <PlatformErrorBoundary>
      <Suspense fallback={fallback ?? <DefaultFallback />}>{children}</Suspense>
    </PlatformErrorBoundary>
  );
}
