/**
 * Staff contact profile (PR-5): each salesperson's own contact line for
 * AI comments. Company identity (brand/services/website) is separate —
 * see CompanyIdentityForm.
 */
import { get, put } from './api';

export interface StaffContactProfile {
  user_id: number;
  org_id: number;
  display_name: string;
  role_title: string;
  telegram: string;
  zalo: string;
  phone: string;
  email: string;
  preferred_cta: string;
  signature_text: string;
  visibility: string;
  active: boolean;
}

export function emptyContactProfile(): StaffContactProfile {
  return {
    user_id: 0, org_id: 0, display_name: '', role_title: '', telegram: '', zalo: '',
    phone: '', email: '', preferred_cta: '', signature_text: '', visibility: 'team', active: true,
  };
}

export async function getMyContactProfile(): Promise<{ profile: StaffContactProfile; contact_line: string }> {
  return get('/me/contact-profile');
}

export async function saveMyContactProfile(p: StaffContactProfile): Promise<{ contact_line: string }> {
  return put('/me/contact-profile', p);
}

export async function listContactProfiles(): Promise<StaffContactProfile[]> {
  const r = await get<{ profiles?: StaffContactProfile[] }>('/org/contact-profiles');
  return r.profiles ?? [];
}

export async function saveContactProfileFor(userId: number, p: StaffContactProfile): Promise<void> {
  await put(`/org/contact-profiles/${userId}`, p);
}
