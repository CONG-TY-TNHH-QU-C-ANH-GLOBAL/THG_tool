import type { ServiceModule } from './types';
import { assertContractInvariants } from './contractInvariants';

const registry = new Map<string, ServiceModule>();

export function registerService(mod: ServiceModule): void {
  // Contract invariants are enforced at registration — fail loud at boot,
  // never silently at runtime. See contractInvariants.ts.
  assertContractInvariants(mod);
  const slug = mod.descriptor.slug;
  if (registry.has(slug)) {
    // Slugs are immutable. Re-registration is a bug — fail loud.
    throw new Error(`[platform/services] service "${slug}" already registered. Slugs are immutable.`);
  }
  registry.set(slug, mod);
}

export function getService(slug: string): ServiceModule | undefined {
  return registry.get(slug);
}

// Listing is deterministic: by descriptor.displayOrder, then slug.
// No reliance on Map insertion order across rebuilds or bootstrap orderings.
export function listServices(): ServiceModule[] {
  return Array.from(registry.values()).sort((a, b) => {
    const da = a.descriptor.displayOrder;
    const db = b.descriptor.displayOrder;
    if (da !== db) return da - db;
    return a.descriptor.slug.localeCompare(b.descriptor.slug);
  });
}
