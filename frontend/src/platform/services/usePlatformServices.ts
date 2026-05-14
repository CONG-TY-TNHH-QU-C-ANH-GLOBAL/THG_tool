'use client';
import { useEffect, useMemo, useState } from 'react';
import { useAuthStore } from '../../modules/autoflow/stores/authStore';
import { get } from '../../modules/autoflow/services/api';
import { bootstrapServices } from './bootstrap';
import { listServices } from './registry';
import type { PlatformService, ServiceModule, ResolutionTrace } from './types';
import type { AuthUser } from '../../modules/autoflow/services/authService';

bootstrapServices();

// projectLocal synthesises a PlatformService from a module's own resolvers.
// It is the OPTIMISTIC placeholder shown before the authoritative backend
// response arrives. Module resolvers tag their traces 'legacy'; the backend
// tags 'authoritative' — so resolutionTraces shows the cutover.
function projectLocal(mod: ServiceModule, user: AuthUser | null): PlatformService {
  const status = mod.resolveStatus?.(user) ?? 'available';
  const workspace = mod.resolveWorkspace(user);
  const capabilities = mod.resolveCapabilities(user);
  const access = mod.resolveAccess(user);
  const resolutionTraces = [workspace.trace, access.trace].filter(
    (t): t is ResolutionTrace => Boolean(t),
  );
  return {
    slug: mod.descriptor.slug,
    label: mod.descriptor.publicLabel,
    serviceVersion: mod.descriptor.version,
    descriptor: mod.descriptor,
    status,
    workspaceState: workspace.state,
    workspaceId: workspace.workspaceId,
    access: access.access,
    accessReason: access.reason,
    reason: workspace.reason,
    rbac: workspace.rbac,
    capabilities,
    resolutionTraces,
    icon: mod.icon,
  };
}

interface ServicesEnvelope {
  contractVersion: number;
  services: PlatformService[];
}

// The backend service registry is the source of truth (BOUNDARIES.md §
// Registry authority). The local projection is only an optimistic fallback.
// Cache is keyed by the whole user identity so a login swap or any change to
// the user (e.g. a freshly created workspace) busts it automatically. The key
// is opaque on purpose — the platform layer does not name storage fields.
let cache: { key: string; services: PlatformService[] } | null = null;
let inflight: { key: string; promise: Promise<PlatformService[]> } | null = null;

function userCacheKey(user: AuthUser | null): string {
  return user ? JSON.stringify(user) : '';
}

function fetchAuthoritative(key: string): Promise<PlatformService[]> {
  if (cache && cache.key === key) return Promise.resolve(cache.services);
  if (inflight && inflight.key === key) return inflight.promise;
  const promise = get<ServicesEnvelope>('/platform/services')
    .then(env => {
      const services = Array.isArray(env?.services) ? env.services : [];
      cache = { key, services };
      return services;
    })
    .catch(() => [] as PlatformService[])
    .finally(() => {
      if (inflight && inflight.key === key) inflight = null;
    });
  inflight = { key, promise };
  return promise;
}

export function usePlatformServices(): PlatformService[] {
  const user = useAuthStore(s => s.user);
  const key = userCacheKey(user);

  // Optimistic local projection — always available, never blocks render.
  const local = useMemo(
    () => listServices().map(mod => projectLocal(mod, user)),
    [user],
  );

  const [authoritative, setAuthoritative] = useState<PlatformService[] | null>(
    cache && cache.key === key ? cache.services : null,
  );

  useEffect(() => {
    if (!user) {
      setAuthoritative(null);
      return;
    }
    let cancelled = false;
    void fetchAuthoritative(key).then(services => {
      if (!cancelled) setAuthoritative(services.length > 0 ? services : null);
    });
    return () => {
      cancelled = true;
    };
  }, [user, key]);

  // Authoritative backend data wins; local projection is the fallback.
  return authoritative ?? local;
}

export function usePlatformService(slug: string): PlatformService | undefined {
  const services = usePlatformServices();
  return useMemo(() => services.find(s => s.slug === slug), [services, slug]);
}
