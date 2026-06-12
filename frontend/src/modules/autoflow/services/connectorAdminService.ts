/**
 * Connector visibility (SaaS UX Hardening PR-3).
 *
 * Admin: workspace-level OPERATIONAL status only — the backend endpoint
 * serializes a dedicated projection (no cookies/tokens/session data)
 * and grants no device control. Staff devices stay pair/unpair-able by
 * their owner only.
 */
import { get, put } from './api';

export interface ConnectorOverviewRow {
  account_id: number;
  account_name: string;
  fb_display_name: string;
  staff_user_id: number;
  staff_name: string;
  staff_email: string;
  staff_role: string;
  connector_online: boolean;
  last_seen: string;
  extension_version: string;
  extension_version_state: 'latest' | 'update_available' | 'update_required' | 'unsupported' | string;
  readiness: string;
  automation_eligible: boolean;
  assignment_paused: boolean;
  block_reasons: string[];
}

export async function getConnectorOverview(): Promise<ConnectorOverviewRow[]> {
  const r = await get<{ accounts?: ConnectorOverviewRow[] }>('/admin/connectors/overview');
  return r.accounts ?? [];
}

export async function pauseAccountAssignment(accountId: number): Promise<void> {
  await put(`/accounts/${accountId}/pause`, {});
}

export async function resumeAccountAssignment(accountId: number): Promise<void> {
  await put(`/accounts/${accountId}/resume`, {});
}

export const UPDATE_INSTRUCTIONS_VI =
  'Cập nhật THG Connector: mở chrome://extensions, bật Developer mode, bấm "Update" (hoặc tải bản mới từ admin), rồi mở lại tab Facebook đã đăng nhập.';

export function versionStateLabel(state: string): { label: string; tone: 'ok' | 'warn' | 'blocked' } {
  switch (state) {
    case 'latest':
      return { label: 'Mới nhất', tone: 'ok' };
    case 'update_available':
      return { label: 'Nên cập nhật', tone: 'warn' };
    case 'update_required':
      return { label: 'Bắt buộc cập nhật', tone: 'blocked' };
    case 'unsupported':
      return { label: 'Không còn hỗ trợ', tone: 'blocked' };
    default:
      return { label: state || '—', tone: 'warn' };
  }
}
