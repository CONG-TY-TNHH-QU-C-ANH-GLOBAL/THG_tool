// Comment Execution Visibility (#3 lifecycle + #8 reason mapping). Pure +
// self-contained so it unit-tests in isolation. Maps the backend
// (execution_state, verification_outcome) pair to a business-friendly status and
// a plain-Vietnamese failure reason — the raw code never shows in the default UI.
//
// INVARIANT: success ONLY when verified. A submitted-but-not-verified attempt
// (optimistic_success) is 'unverified', NOT success.

export type ExecSeverity = 'waiting' | 'running' | 'success' | 'unverified' | 'failed';

export interface CommentStatus {
  label: string;
  severity: ExecSeverity;
}

export function commentStatus(state: string, outcome: string): CommentStatus {
  const s = (state || '').toLowerCase();
  const o = (outcome || '').toLowerCase();
  if (s === 'queued' || s === 'planned') return { label: 'Đang chờ', severity: 'waiting' };
  if (s === 'claimed' || s === 'executing' || s === 'in_progress') return { label: 'Đang chạy', severity: 'running' };
  if (s === 'expired') return { label: 'Hết hạn — chưa chạy', severity: 'failed' };
  if (s === 'finished') {
    if (o === 'verified_success' || o === 'dom_verified') return { label: 'Đã đăng thành công', severity: 'success' };
    if (o === 'submitted_unverified' || o === 'optimistic_success') return { label: 'Đã gửi nhưng chưa xác minh', severity: 'unverified' };
    return { label: 'Thất bại', severity: 'failed' };
  }
  return { label: 'Đang chờ', severity: 'waiting' };
}

// commentReason → plain Vietnamese for a failed/unverified outcome ('' for success).
export function commentReason(outcome: string): string {
  switch ((outcome || '').toLowerCase()) {
    case 'verified_success':
    case 'dom_verified':
      return '';
    case 'submitted_unverified':
    case 'optimistic_success':
      return 'Đã bấm gửi nhưng hệ thống chưa thấy comment xuất hiện để xác minh.';
    case 'target_not_reached':
    case 'redirected_feed':
      return 'Không mở được đúng bài viết Facebook.';
    case 'context_drift':
      return 'Facebook chuyển trang trước khi gửi comment.';
    case 'connector_offline':
      return 'Chrome profile chưa kết nối.';
    case 'actor_mismatch_blocked':
      return 'Đăng nhập nhầm Facebook.';
    case 'comment_quality_invalid':
      return 'Comment không đạt kiểm tra chất lượng.';
    case 'comment_required_website_missing':
    case 'comment_unsupported_contact':
      return 'Comment thiếu website/liên hệ bắt buộc theo chính sách.';
    case 'rate_limited':
      return 'Facebook tạm giới hạn — thử lại sau.';
    case 'blocked':
      return 'Facebook chặn hành động này.';
    case 'captcha':
      return 'Facebook yêu cầu xác minh thủ công.';
    case 'shadow_rejected':
      return 'Facebook đã ẩn comment.';
    case '':
      return '';
    default:
      return 'Lỗi chưa xác định, cần kiểm tra bằng chứng.';
  }
}
