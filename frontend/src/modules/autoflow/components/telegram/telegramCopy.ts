// THE single feature-local source of Telegram UI copy + labels (vi primary, en fallback). Kept
// local so the central strings.ts is not grown. `strings(lang)` returns a translator `t`; the
// key->label helpers (eventLabel/groupLabel/destTypeLabel) translate enum-ish keys. Named
// `strings`, NOT `copy`, to avoid any confusion with copy-to-clipboard.
export type Lang = 'vi' | 'en';
type Dict = Record<string, string>;

const VI: Dict = {
  title: 'Kết nối Telegram',
  subtitle: 'Gửi lead mới, trạng thái agent và cảnh báo vận hành vào Telegram Channel của workspace.',
  state_not_connected: 'Chưa kết nối', state_connected: 'Đã kết nối', state_needs_attention: 'Cần xử lý', state_disabled: 'Đã tắt',
  bot_configured: 'Bot', yes: 'Có', no: 'Chưa', enabled_word: 'Bật', disabled_word: 'Tắt',
  notifications: 'Thông báo', actions_exec: 'Thực thi hành động', never: 'Chưa có',
  active_destinations: 'Channel đang nhận', last_delivery: 'Gửi gần nhất', last_error: 'Lỗi gần nhất',
  channels: 'Kênh', flags: 'Cấu hình',
  safety_notice: 'Telegram hiện dùng để nhận thông báo và theo dõi vận hành. Gửi comment/post/inbox trực tiếp từ Telegram đang bị TẮT.',
  channels_title: 'Telegram Channels', channels_empty: 'Chưa kết nối channel nào.',
  col_channel: 'Channel', col_type: 'Loại', col_filter: 'Lọc kênh', col_events: 'Sự kiện',
  col_last_delivery: 'Gửi gần nhất', col_last_error: 'Lỗi', col_status: 'Trạng thái', col_actions: '',
  connect_channel: 'Kết nối Telegram Channel', disconnect: 'Ngắt kết nối', test: 'Gửi thử', edit_prefs: 'Sửa tùy chọn',
  active: 'active', revoked: 'revoked', save: 'Lưu', saved: 'Đã lưu', cancel: 'Hủy',
  copy: 'Sao chép', copied: 'Đã sao chép', expires_in: 'Hết hạn sau', code_expired: 'Mã đã hết hạn — tạo mã mới',
  wiz_title: 'Kết nối Telegram Channel', wiz_choose: 'Chọn loại channel',
  type_public: 'Channel công khai (@username)', type_private: 'Channel riêng tư',
  pub_s1: 'Tạo hoặc mở Telegram Channel của bạn', pub_s2: 'Thêm bot THG làm admin (có quyền gửi)',
  pub_s3: 'Nhập @channel_username', pub_username_label: 'Tên channel (@username)', verify_connect: 'Xác minh & Kết nối',
  verifying: 'Đang xác minh…', connected_ok: 'Đã kết nối channel!',
  priv_s1: 'Tạo hoặc mở Channel riêng tư', priv_s2: 'Thêm bot THG làm admin', priv_s3: 'Tạo mã kết nối một lần',
  priv_post_hint: 'Đăng tin nhắn này trong channel:', priv_check_again: 'Tôi đã đăng — kiểm tra lại',
  prefs_title: 'Tùy chọn sự kiện', prefs_hint: 'Bạn có thể chọn loại sự kiện mà channel này sẽ nhận.',
  channel_filter: 'Lọc theo kênh', delivery_mode: 'Chế độ gửi', mode_immediate: 'Ngay lập tức', mode_digest: 'Tổng hợp (sắp có)',
  preview_title: 'Xem trước thông báo',
  test_delivered: 'Đã gửi thành công', test_failed: 'Gửi thất bại',
  needs_title: 'Cần xử lý',
  reason_token_missing: 'Chưa cấu hình bot token.', reason_notify_disabled: 'Thông báo đang TẮT (TELEGRAM_NOTIFY_ENABLED=false).',
  reason_delivery_failed: 'Lần gửi gần nhất thất bại — bot có thể đã bị xóa khỏi channel, không có quyền gửi, hoặc Telegram lỗi.',
  reason_resolve_failed: 'Không gửi được tới channel. Kiểm tra: bot đã là admin chưa, @username đúng chưa.',
  reason_username_required: 'Nhập @username của channel.', reason_not_found: 'Không tìm thấy channel.',
  personal_title: 'Người dùng liên kết cá nhân',
  personal_desc: 'Dành cho thông báo cá nhân hoặc lệnh /status. Không bắt buộc để nhận thông báo trong Telegram Channel.',
  personal_empty: 'Chưa có người dùng liên kết cá nhân. Đây là tùy chọn.',
  connect_dm: 'Liên kết DM cá nhân', bind_hint: 'Mở bot và gửi:', open_bot: 'Mở bot Telegram', generate_code: 'Tạo mã liên kết',
  col_user: 'Người dùng', col_role: 'Vai trò', col_bound: 'Liên kết lúc', col_last_command: 'Lệnh gần nhất', revoke: 'Thu hồi',
  audit_title: 'Nhật ký kiểm toán', audit_empty: 'Chưa có sự kiện.',
  col_time: 'Thời gian', col_actor: 'Người thực hiện', col_action: 'Hành động', col_result: 'Kết quả',
  empty_title: 'Chưa kết nối Telegram Channel',
  empty_b1: 'Nhận lead mới ngay khi xuất hiện', empty_b2: 'Nhận trạng thái agent comment / post / inbox',
  empty_b3: 'Nhận cảnh báo lỗi và sự cố vận hành', empty_cta: 'Kết nối Telegram Channel',
  admin_only: 'Chỉ admin mới quản lý được mục này.', err_generic: 'Có lỗi xảy ra, vui lòng thử lại.',
};

const EN: Dict = {
  title: 'Connect Telegram',
  subtitle: 'Send new leads, agent status and operational alerts into your workspace Telegram channel.',
  state_not_connected: 'Not connected', state_connected: 'Connected', state_needs_attention: 'Needs attention', state_disabled: 'Disabled',
  bot_configured: 'Bot', yes: 'Yes', no: 'No', enabled_word: 'On', disabled_word: 'Off',
  notifications: 'Notifications', actions_exec: 'Action execution', never: 'None',
  active_destinations: 'Active channels', last_delivery: 'Last delivery', last_error: 'Last error',
  channels: 'Channels', flags: 'Config',
  safety_notice: 'Telegram is for receiving notifications and monitoring operations. Sending comments/posts/inbox directly from Telegram is DISABLED.',
  channels_title: 'Telegram Channels', channels_empty: 'No channels connected yet.',
  col_channel: 'Channel', col_type: 'Type', col_filter: 'Filter', col_events: 'Events',
  col_last_delivery: 'Last delivery', col_last_error: 'Error', col_status: 'Status', col_actions: '',
  connect_channel: 'Connect Telegram channel', disconnect: 'Disconnect', test: 'Test', edit_prefs: 'Edit preferences',
  active: 'active', revoked: 'revoked', save: 'Save', saved: 'Saved', cancel: 'Cancel',
  copy: 'Copy', copied: 'Copied', expires_in: 'Expires in', code_expired: 'Code expired — generate a new one',
  wiz_title: 'Connect a Telegram channel', wiz_choose: 'Choose channel type',
  type_public: 'Public channel (@username)', type_private: 'Private channel',
  pub_s1: 'Create or open your Telegram channel', pub_s2: 'Add the THG bot as admin (with send rights)',
  pub_s3: 'Enter @channel_username', pub_username_label: 'Channel (@username)', verify_connect: 'Verify & connect',
  verifying: 'Verifying…', connected_ok: 'Channel connected!',
  priv_s1: 'Create or open your private channel', priv_s2: 'Add the THG bot as admin', priv_s3: 'Generate a one-time connect code',
  priv_post_hint: 'Post this message in the channel:', priv_check_again: "I posted it — check again",
  prefs_title: 'Event preferences', prefs_hint: 'Choose which event types this channel receives.',
  channel_filter: 'Channel filter', delivery_mode: 'Delivery mode', mode_immediate: 'Immediate', mode_digest: 'Digest (soon)',
  preview_title: 'Notification preview',
  test_delivered: 'Delivered', test_failed: 'Delivery failed',
  needs_title: 'Needs attention',
  reason_token_missing: 'Bot token is not configured.', reason_notify_disabled: 'Notifications are OFF (TELEGRAM_NOTIFY_ENABLED=false).',
  reason_delivery_failed: 'The last delivery failed — the bot may have been removed, lacks send rights, or Telegram errored.',
  reason_resolve_failed: 'Could not send to the channel. Check the bot is an admin and the @username is correct.',
  reason_username_required: 'Enter the channel @username.', reason_not_found: 'Channel not found.',
  personal_title: 'Personal DM bindings',
  personal_desc: 'For personal notifications or /status commands. Not required to receive Telegram channel notifications.',
  personal_empty: 'No personal bindings yet. This is optional.',
  connect_dm: 'Link personal DM', bind_hint: 'Open the bot and send:', open_bot: 'Open Telegram bot', generate_code: 'Generate code',
  col_user: 'User', col_role: 'Role', col_bound: 'Bound at', col_last_command: 'Last command', revoke: 'Revoke',
  audit_title: 'Audit log', audit_empty: 'No events yet.',
  col_time: 'Time', col_actor: 'Actor', col_action: 'Action', col_result: 'Result',
  empty_title: 'No Telegram channel connected',
  empty_b1: 'Get new leads the moment they appear', empty_b2: 'Get agent comment / post / inbox status',
  empty_b3: 'Get error and operational incident alerts', empty_cta: 'Connect Telegram channel',
  admin_only: 'Only admins can manage this section.', err_generic: 'Something went wrong, please try again.',
};

// strings(lang).t(key) -> localized prose. Unknown keys return the key (safe).
export function strings(lang: Lang) {
  const d = lang === 'en' ? EN : VI;
  return { t: (k: string) => d[k] ?? k };
}

// ── enum-ish key -> label helpers (vi explicit; en humanizes the key) ──
const EVENT_VI: Dict = {
  lead_created: 'Lead mới', lead_assigned: 'Lead được giao', lead_ready_for_review: 'Lead sẵn sàng xử lý',
  comment_submitted: 'Comment đã gửi', comment_verified: 'Comment đã xác minh',
  comment_unverified: 'Comment chưa xác minh', comment_failed: 'Comment lỗi',
  post_submitted: 'Post đã gửi', post_failed: 'Post lỗi', inbox_sent: 'Inbox đã gửi', inbox_failed: 'Inbox lỗi',
  connector_offline: 'Connector offline', account_attention: 'Tài khoản cần chú ý',
  automation_paused: 'Tự động hoá tạm dừng', gate1_failure_spike: 'Tăng đột biến lỗi gate1',
  submitted_unverified_spike: 'Tăng đột biến chưa xác minh', circuit_breaker_triggered: 'Ngắt mạch kích hoạt',
};
const GROUP_VI: Dict = { lead: 'Sự kiện Lead', agent: 'Hành động Agent', system: 'Hệ thống / Cảnh báo' };
const GROUP_EN: Dict = { lead: 'Lead events', agent: 'Agent actions', system: 'System / Health' };
const TYPE_VI: Dict = { channel: 'Channel', group: 'Group', personal_dm: 'DM cá nhân' };

function humanize(key: string): string {
  return key.replace(/_/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase());
}
export function eventLabel(lang: Lang, key: string): string {
  return lang === 'vi' ? EVENT_VI[key] ?? humanize(key) : humanize(key);
}
export function groupLabel(lang: Lang, key: string): string {
  return (lang === 'vi' ? GROUP_VI : GROUP_EN)[key] ?? key;
}
export function destTypeLabel(lang: Lang, type: string): string {
  return lang === 'en' ? humanize(type) : TYPE_VI[type] ?? type;
}
