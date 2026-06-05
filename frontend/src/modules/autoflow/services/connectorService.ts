/**
 * PR-M2 presence board — per-account connector + member + online status.
 * Thin wrapper over GET /api/connectors/status (internal/server/agent/
 * connector_status.go). Turns the opaque "N extensions online" into a per-
 * account "is this account reachable right now, and who owns it" view.
 */
import { get } from './api';

export type ConnectorAccountState =
  | 'online'
  | 'logged_out'
  | 'offline'
  | 'no_connector'
  | 'wrong_account'
  | 'unassigned';

export interface ConnectorAccountStatus {
  account_id: number;
  account_name: string;
  assigned_user_id: number;
  assigned_user_name: string;
  account_fb_user_id: string;
  account_fb_display_name: string;
  connector_id: number;
  connector_name: string;
  connector_online: boolean;
  stream_status: string;
  connector_fb_user_id: string;
  connector_fb_display_name: string;
  reachable: boolean;
  state: ConnectorAccountState;
}

export interface ConnectorStatusResponse {
  accounts: ConnectorAccountStatus[];
  unbound_online: ConnectorAccountStatus[];
  accounts_total: number;
  online_total: number;
  reachable_total: number;
}

export async function getConnectorStatus(): Promise<ConnectorStatusResponse> {
  return get<ConnectorStatusResponse>('/connectors/status');
}
