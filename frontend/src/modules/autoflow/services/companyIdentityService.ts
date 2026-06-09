import { get, put } from './api';
import type { CompanyIdentity } from '../components/companyIdentity/types';

// GET/PUT /api/org/company-identity — reads/writes the grounded brand identity the
// comment generator uses. Tenant-scoped + admin-only write (enforced server-side).
export async function getCompanyIdentity(): Promise<CompanyIdentity> {
  return get<CompanyIdentity>('/org/company-identity');
}

export async function saveCompanyIdentity(data: CompanyIdentity): Promise<CompanyIdentity> {
  return put<CompanyIdentity>('/org/company-identity', data);
}
