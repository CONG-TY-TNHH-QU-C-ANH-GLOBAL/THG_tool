// Telegram UI copy (vi primary, en fallback). Kept local to the feature so the 900-line central
// strings.ts is not grown. Self-contained map keyed by the logic.ts keys; pick by lang from
// useLang().
export type Lang = 'vi' | 'en';

type Dict = Record<string, string>;

const VI: Dict = {
  title: 'Tích hợp Telegram',
  subtitle: 'Thiết lập, theo dõi và thu hồi kết nối Telegram cho workspace.',
  state_not_connected: 'Chưa kết nối',
  state_connected: 'Đã kết nối',
  state_needs_attention: 'Cần xử lý',
  bot_configured: 'Bot token',
  yes: 'Có', no: 'Chưa',
  webhook: 'Webhook', last_webhook: 'Webhook gần nhất', never: 'Chưa có',
  bound_users: 'Người dùng đã liên kết', alert_recipients: 'Người nhận cảnh báo',
  channels: 'Kênh', flags: 'Cấu hình',
  safety_notice: 'Telegram chỉ hỗ trợ thiết lập, trạng thái, chỉ số, cảnh báo và thao tác control-plane. Thực thi hành động (gửi bình luận) qua Telegram đang TẮT.',
  setup_title: 'Hướng dẫn thiết lập',
  step_enable: 'Bật tích hợp', step_code: 'Tạo mã liên kết một lần',
  step_open_bot: 'Mở bot Telegram', step_bind: 'Gửi /bind <mã> cho bot',
  step_test: 'Gửi thông báo thử',
  enable: 'Bật tích hợp', disable: 'Tắt', generate_code: 'Tạo mã liên kết',
  copy: 'Sao chép', copied: 'Đã sao chép', open_bot: 'Mở bot Telegram',
  expires_in: 'Hết hạn sau', code_expired: 'Mã đã hết hạn — tạo mã mới',
  bind_hint: 'Mở bot và gửi:', test_notification: 'Gửi thông báo thử',
  bindings_title: 'Người dùng đã liên kết', bindings_empty: 'Chưa có liên kết nào.',
  col_user: 'Người dùng', col_role: 'Vai trò', col_bound: 'Liên kết lúc',
  col_recipient: 'Nhận cảnh báo', col_status: 'Trạng thái', col_actions: '',
  revoke: 'Thu hồi', active: 'active', revoked: 'revoked',
  alerts_title: 'Tùy chọn cảnh báo', alerts_enabled: 'Bật cảnh báo Telegram',
  channel_filter: 'Lọc theo kênh', alert_types: 'Loại cảnh báo', save: 'Lưu', saved: 'Đã lưu',
  audit_title: 'Nhật ký kiểm toán', audit_empty: 'Chưa có sự kiện.',
  col_time: 'Thời gian', col_actor: 'Người thực hiện', col_action: 'Hành động', col_result: 'Kết quả',
  empty_title: 'Chưa kết nối Telegram', empty_body: 'Bật tích hợp và liên kết tài khoản Telegram để nhận cảnh báo và điều khiển từ xa.',
  needs_title: 'Cần xử lý',
  reason_bot_disabled: 'Bot Telegram đang tắt (TELEGRAM_BOT_ENABLED=false).',
  reason_token_missing: 'Chưa cấu hình bot token.',
  reason_webhook_error: 'Webhook gặp lỗi gần đây.',
  reason_no_bound_users: 'Chưa có người dùng nào liên kết.',
  reason_no_alert_recipients: 'Chưa có người nhận cảnh báo nào.',
  reason_notify_disabled: 'Thông báo đang tắt (TELEGRAM_NOTIFY_ENABLED=false).',
  admin_only: 'Chỉ admin mới xem được mục này.',
  err_generic: 'Có lỗi xảy ra, vui lòng thử lại.',
};

const EN: Dict = {
  title: 'Telegram Integration', subtitle: 'Set up, monitor and revoke the Telegram connection for this workspace.',
  state_not_connected: 'Not connected', state_connected: 'Connected', state_needs_attention: 'Needs attention',
  bot_configured: 'Bot token', yes: 'Yes', no: 'No',
  webhook: 'Webhook', last_webhook: 'Last webhook', never: 'None',
  bound_users: 'Bound users', alert_recipients: 'Alert recipients', channels: 'Channels', flags: 'Config',
  safety_notice: 'Telegram supports setup, status, metrics, alerts and control-plane actions only. Action execution (sending comments) via Telegram is DISABLED.',
  setup_title: 'Setup guide', step_enable: 'Enable integration', step_code: 'Generate a one-time bind code',
  step_open_bot: 'Open the Telegram bot', step_bind: 'Send /bind <code> to the bot', step_test: 'Send a test notification',
  enable: 'Enable integration', disable: 'Disable', generate_code: 'Generate bind code',
  copy: 'Copy', copied: 'Copied', open_bot: 'Open Telegram bot',
  expires_in: 'Expires in', code_expired: 'Code expired — generate a new one',
  bind_hint: 'Open the bot and send:', test_notification: 'Send test notification',
  bindings_title: 'Bound users', bindings_empty: 'No bindings yet.',
  col_user: 'User', col_role: 'Role', col_bound: 'Bound at', col_recipient: 'Alerts', col_status: 'Status', col_actions: '',
  revoke: 'Revoke', active: 'active', revoked: 'revoked',
  alerts_title: 'Alert preferences', alerts_enabled: 'Enable Telegram alerts',
  channel_filter: 'Channel filter', alert_types: 'Alert types', save: 'Save', saved: 'Saved',
  audit_title: 'Audit log', audit_empty: 'No events yet.',
  col_time: 'Time', col_actor: 'Actor', col_action: 'Action', col_result: 'Result',
  empty_title: 'Telegram not connected', empty_body: 'Enable the integration and bind a Telegram account to receive alerts and control automation remotely.',
  needs_title: 'Needs attention',
  reason_bot_disabled: 'Telegram bot is disabled (TELEGRAM_BOT_ENABLED=false).',
  reason_token_missing: 'Bot token is not configured.',
  reason_webhook_error: 'The webhook reported a recent error.',
  reason_no_bound_users: 'No users are bound yet.',
  reason_no_alert_recipients: 'No alert recipients yet.',
  reason_notify_disabled: 'Notifications are disabled (TELEGRAM_NOTIFY_ENABLED=false).',
  admin_only: 'Only admins can view this section.',
  err_generic: 'Something went wrong, please try again.',
};

const ALERT_LABELS: Record<Lang, Dict> = {
  vi: {
    connector_offline: 'Connector offline', gate1_failure_spike: 'Tăng đột biến lỗi gate1',
    submitted_unverified_spike: 'Tăng đột biến chưa xác minh', automation_paused: 'Tự động hoá bị tạm dừng',
    account_needs_attention: 'Tài khoản cần chú ý', circuit_breaker_triggered: 'Ngắt mạch kích hoạt',
  },
  en: {
    connector_offline: 'Connector offline', gate1_failure_spike: 'Gate1 failure spike',
    submitted_unverified_spike: 'Submitted-unverified spike', automation_paused: 'Automation paused',
    account_needs_attention: 'Account needs attention', circuit_breaker_triggered: 'Circuit breaker triggered',
  },
};

export function copy(lang: Lang) {
  const d = lang === 'en' ? EN : VI;
  return {
    t: (k: string) => d[k] ?? k,
    alertLabel: (k: string) => ALERT_LABELS[lang][k] ?? k,
  };
}
