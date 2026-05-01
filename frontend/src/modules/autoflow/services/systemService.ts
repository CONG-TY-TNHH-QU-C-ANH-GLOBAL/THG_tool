import { get } from './api';

export interface SystemInfo {
  headless: boolean;
  agent_builds: {
    windows?: boolean;
    mac_intel?: boolean;
    mac_m1?: boolean;
    linux?: boolean;
    chrome_extension?: boolean;
    local_kit_windows?: boolean;
    local_kit_mac_m1?: boolean;
    local_kit_mac_intel?: boolean;
    local_kit_linux?: boolean;
  };
}

export async function getSystemInfo(): Promise<SystemInfo> {
  return get('/system/info');
}
