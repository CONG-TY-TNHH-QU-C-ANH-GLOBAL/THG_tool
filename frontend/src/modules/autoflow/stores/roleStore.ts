import { create } from 'zustand';
import { isPlatformRole, type Role } from '../services/authService';

interface RoleState {
  role: Role;
  isAdmin: boolean;
  isFounder: boolean;
  isSuperAdmin: boolean;
  setRole(role: Role): void;
}

export const useRoleStore = create<RoleState>((set) => ({
  role: 'sales',
  isAdmin: false,
  isFounder: false,
  isSuperAdmin: false,

  setRole(role) {
    const platformRole = isPlatformRole(role);
    set({
      role,
      isAdmin: role === 'admin' || platformRole,
      isFounder: platformRole,
      isSuperAdmin: platformRole,
    });
  },
}));
