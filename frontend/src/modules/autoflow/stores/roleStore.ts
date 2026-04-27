import { create } from 'zustand';
import type { Role } from '../services/authService';

interface RoleState {
  role: Role;
  isAdmin: boolean;
  isSuperAdmin: boolean;
  setRole(role: Role): void;
}

export const useRoleStore = create<RoleState>((set) => ({
  role: 'sales',
  isAdmin: false,
  isSuperAdmin: false,

  setRole(role) {
    set({
      role,
      isAdmin: role === 'admin' || role === 'superadmin',
      isSuperAdmin: role === 'superadmin',
    });
  },
}));
