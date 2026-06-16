// UX translation layer for the Account Health Board (PR-E). The backend emits
// machine-readable reason codes (connector_offline, actor_mismatch_blocked, …);
// a customer-facing SaaS must NEVER show those raw. This pure module maps each
// code to business-language title/description/action + a severity, and exposes the
// raw code only for an admin "technical details" view. Self-contained (no imports)
// so it is unit-testable in isolation.

export type Severity = 'ready' | 'warning' | 'blocked' | 'waiting';

export interface ReasonMessage {
  title: string;
  description: string;
  action: string;
  severity: Severity;
  technical_code: string; // shown only in admin/technical-details mode
}

type ReasonBody = Omit<ReasonMessage, 'technical_code'>;

const REASON_MESSAGES: Record<string, ReasonBody> = {
  connector_offline: {
    title: 'Chrome profile chưa kết nối',
    description: 'Tài khoản này chưa có Chrome sẵn sàng nên hệ thống chưa thể chạy tự động.',
    action: 'Mở Chrome profile đã gắn với tài khoản này, đăng nhập Facebook và đảm bảo THG Connector đang bật.',
    severity: 'blocked',
  },
  actor_identity_unknown: {
    title: 'Chưa xác minh được Facebook đang đăng nhập',
    description: 'Hệ thống chưa đọc được Facebook ID thật trong Chrome profile này.',
    action: 'Mở Facebook trong Chrome profile này rồi chờ vài giây để hệ thống nhận diện lại.',
    severity: 'warning',
  },
  actor_mismatch_blocked: {
    title: 'Đăng nhập nhầm Facebook',
    description: 'Chrome profile này đang đăng nhập một Facebook khác với tài khoản đã gắn trước đó. Để tránh chạy sai account, hệ thống đã chặn tự động.',
    action: 'Đăng nhập lại đúng Facebook hoặc nhờ admin gỡ chặn sau khi kiểm tra.',
    severity: 'blocked',
  },
  extension_version_outdated: {
    title: 'Extension cần cập nhật',
    description: 'Phiên bản THG Connector đang cũ nên không đủ điều kiện chạy tự động hoá.',
    action: 'Reload hoặc cập nhật THG Connector lên phiên bản mới nhất.',
    severity: 'warning',
  },
  extension_update_available: {
    title: 'Có bản cập nhật extension mới',
    description: 'Có bản cập nhật extension mới. Bạn vẫn có thể dùng, nhưng nên cập nhật để ổn định hơn.',
    action: 'Cập nhật THG Connector khi thuận tiện.',
    severity: 'warning',
  },
  extension_update_required: {
    title: 'Cần cập nhật extension để tiếp tục',
    description: 'Automation đang tạm dừng vì extension của bạn đã cũ. Cập nhật extension để tiếp tục nhận task.',
    action: 'Cập nhật THG Connector lên phiên bản mới rồi mở lại tab Facebook.',
    severity: 'blocked',
  },
  extension_unsupported: {
    title: 'Phiên bản extension không còn được hỗ trợ',
    description: 'Phiên bản extension này không còn được hỗ trợ. Vui lòng cài phiên bản mới.',
    action: 'Gỡ extension cũ và cài phiên bản mới nhất từ admin.',
    severity: 'blocked',
  },
  blocked_by_extension_version: {
    title: 'Task bị chặn do phiên bản extension',
    description: 'Hệ thống không giao task mới vì extension của tài khoản này chưa đạt phiên bản tối thiểu.',
    action: 'Cập nhật THG Connector để tiếp tục nhận task.',
    severity: 'blocked',
  },
  account_cooldown_active: {
    title: 'Tài khoản đang nghỉ an toàn',
    description: 'Hệ thống tạm nghỉ tài khoản này để tránh thao tác quá dày trên Facebook.',
    action: 'Chờ hết thời gian nghỉ để tránh thao tác quá dày trên Facebook.',
    severity: 'waiting',
  },
  risk_ceiling_exceeded: {
    title: 'Tài khoản đang ở chế độ bảo vệ',
    description: 'Tài khoản có nhiều lỗi gần đây nên hệ thống tạm dừng để bảo vệ uy tín tài khoản.',
    action: 'Kiểm tra lỗi gần đây hoặc chờ hệ thống hồi phục.',
    severity: 'blocked',
  },
  daily_limit_exceeded: {
    title: 'Đã đạt giới hạn hôm nay',
    description: 'Tài khoản đã dùng hết số lượt hành động cho hôm nay.',
    action: 'Tài khoản sẽ tiếp tục vào ngày mai hoặc admin điều chỉnh giới hạn.',
    severity: 'waiting',
  },
  assignment_paused_by_admin: {
    title: 'Admin đã tạm dừng automation',
    description: 'Admin đã tạm dừng giao task tự động cho tài khoản này để đảm bảo an toàn.',
    action: 'Liên hệ admin của workspace để mở lại khi sẵn sàng.',
    severity: 'blocked',
  },
};

// Priority: when several reasons apply at once, surface the most important one.
const REASON_PRIORITY: string[] = [
  'assignment_paused_by_admin',
  'actor_mismatch_blocked',
  'actor_identity_unknown',
  'connector_offline',
  'extension_unsupported',
  'extension_update_required',
  'blocked_by_extension_version',
  'extension_version_outdated',
  'extension_update_available',
  'risk_ceiling_exceeded',
  'account_cooldown_active',
  'daily_limit_exceeded',
];

const FALLBACK: ReasonBody = {
  title: 'Cần kiểm tra tài khoản',
  description: 'Hệ thống phát hiện một trạng thái cần kiểm tra ở tài khoản này.',
  action: 'Làm mới trạng thái hoặc liên hệ admin để kiểm tra.',
  severity: 'warning',
};

// mapReason turns one raw code into a customer-facing message. Unknown codes get a
// friendly fallback (never the raw code).
export function mapReason(code: string): ReasonMessage {
  const body = REASON_MESSAGES[code] || FALLBACK;
  return { ...body, technical_code: code };
}

// pickPrimaryReason chooses the highest-priority reason from a set; returns null
// when the set is empty (account is fine for that scope).
export function pickPrimaryReason(reasons: string[]): string | null {
  if (!reasons || reasons.length === 0) return null;
  for (const code of REASON_PRIORITY) {
    if (reasons.includes(code)) return code;
  }
  return reasons[0]; // unknown code(s) only → keep the first
}

const SEVERITY_LABEL: Record<Severity, string> = {
  ready: 'Sẵn sàng',
  warning: 'Cần kiểm tra',
  blocked: 'Đang bị chặn',
  waiting: 'Đang nghỉ an toàn',
};

// overallStatus reduces an account's per-capability reasons to one headline status.
export function overallStatus(allReasons: string[]): { severity: Severity; label: string; primary: ReasonMessage | null } {
  const primaryCode = pickPrimaryReason(allReasons);
  if (!primaryCode) {
    return { severity: 'ready', label: SEVERITY_LABEL.ready, primary: null };
  }
  const primary = mapReason(primaryCode);
  return { severity: primary.severity, label: SEVERITY_LABEL[primary.severity], primary };
}

export function severityLabel(s: Severity): string {
  return SEVERITY_LABEL[s];
}

// P1.3E account-level EXECUTABILITY states. The backend computes `executable` + a typed
// `exec_reason_code` from the REQUESTER's OWN live connector (not org-wide). These drive the
// green "Sẵn sàng" badge and the "Sẵn sàng" summary count strictly — green ONLY when executable.
export interface ExecStateMessage extends ReasonBody {
  label: string; // the headline badge label for this state
}

const EXEC_STATE: Record<string, ExecStateMessage> = {
  ready: { label: 'Sẵn sàng', severity: 'ready', title: 'Sẵn sàng', description: 'Chrome của bạn đang online đúng tài khoản Facebook này.', action: '' },
  no_connector: { label: 'Chưa kết nối Chrome', severity: 'blocked', title: 'Chưa kết nối Chrome', description: 'Tài khoản này chưa có Chrome (THG Connector) của bạn đang kết nối.', action: 'Mở Chrome profile của bạn, đăng nhập Facebook và ghép nối THG Connector.' },
  connector_stale: { label: 'Mất kết nối Chrome', severity: 'blocked', title: 'Mất kết nối Chrome', description: 'Chrome đã từng kết nối nhưng hiện không còn nhịp tín hiệu (offline).', action: 'Mở lại Chrome profile và đảm bảo THG Connector đang bật.' },
  pairing_pending: { label: 'Đang chờ pair extension', severity: 'warning', title: 'Đang chờ ghép nối extension', description: 'Chrome đang online nhưng chưa đăng nhập/ghép nối xong với tài khoản này.', action: 'Hoàn tất đăng nhập Facebook và nhập mã ghép nối trong extension.' },
  identity_mismatch: { label: 'Sai Facebook profile', severity: 'blocked', title: 'Sai Facebook profile', description: 'Chrome đang đăng nhập một Facebook khác với tài khoản này.', action: 'Đăng nhập đúng Facebook cho tài khoản này trong Chrome profile.' },
  session_blocked: { label: 'Session bị chặn/checkpoint', severity: 'blocked', title: 'Phiên Facebook bị chặn', description: 'Facebook đang ở checkpoint/login wall hoặc đã đăng xuất.', action: 'Mở Chrome, vượt checkpoint hoặc đăng nhập lại Facebook.' },
  account_blocked: { label: 'Đang bị chặn', severity: 'blocked', title: 'Tài khoản đang bị chặn', description: 'Tài khoản đang bị tạm ngưng hoặc bị hệ thống chặn để bảo vệ.', action: 'Liên hệ admin hoặc chờ hệ thống mở lại.' },
  not_controllable: { label: 'Bạn không có quyền dùng tài khoản này', severity: 'blocked', title: 'Không có quyền điều khiển', description: 'Bạn có thể xem nhưng không được điều khiển tài khoản này.', action: 'Dùng tài khoản Facebook do bạn kết nối, hoặc nhờ admin gán quyền.' },
};

// execState maps one exec_reason_code to its headline message. Unknown codes (e.g. version
// codes the backend passes through) fall back to the existing reason map, never the raw code.
export function execState(execReasonCode: string): ExecStateMessage {
  const e = EXEC_STATE[execReasonCode];
  if (e) return e;
  const m = mapReason(execReasonCode || '');
  return { label: SEVERITY_LABEL[m.severity], severity: m.severity, title: m.title, description: m.description, action: m.action };
}

// accountExecState is the account-level headline driven STRICTLY by `executable`. A green
// "Sẵn sàng" appears only when executable === true; otherwise the typed not-executable state.
export function accountExecState(account: { executable?: boolean; exec_reason_code?: string }): ExecStateMessage {
  if (account.executable) return EXEC_STATE.ready;
  return execState(account.exec_reason_code || 'no_connector');
}
