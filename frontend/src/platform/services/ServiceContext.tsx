'use client';
import { createContext, useContext, type ReactNode } from 'react';
import type { PlatformService, ServiceModule } from './types';

export interface ServiceContextValue {
  service: PlatformService;
  module: ServiceModule;
  workspaceId?: string;
}

const ServiceContext = createContext<ServiceContextValue | null>(null);

export function ServiceProvider({
  service,
  module,
  workspaceId,
  children,
}: ServiceContextValue & { children: ReactNode }) {
  return (
    <ServiceContext.Provider value={{ service, module, workspaceId }}>
      {children}
    </ServiceContext.Provider>
  );
}

export function useServiceContext(): ServiceContextValue {
  const ctx = useContext(ServiceContext);
  if (!ctx) {
    throw new Error('useServiceContext must be used inside <ServiceProvider>');
  }
  return ctx;
}

export function useOptionalServiceContext(): ServiceContextValue | null {
  return useContext(ServiceContext);
}
