import { create } from 'zustand';
import type { Organization } from '../types';

interface OrgState {
  orgs: Organization[];
  activeOrgId: string;
  activeOrg: Organization | null;
  setOrgs(orgs: Organization[]): void;
  setActiveOrg(id: string): void;
}

const DEFAULT_ORG_ID = '1';

export const useOrgStore = create<OrgState>((set, get) => ({
  orgs: [],
  activeOrgId: DEFAULT_ORG_ID,
  activeOrg: null,

  setOrgs(orgs) {
    const activeOrgId = get().activeOrgId;
    set({ orgs, activeOrg: orgs.find(o => String(o.id) === activeOrgId) ?? orgs[0] ?? null });
  },

  setActiveOrg(id) {
    const orgs = get().orgs;
    set({ activeOrgId: id, activeOrg: orgs.find(o => String(o.id) === id) ?? null });
  },
}));
