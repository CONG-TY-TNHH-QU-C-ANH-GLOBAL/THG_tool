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

// effectiveOutcome resolves the outcome the UI should display: a succeeded ledger correction
// (human_verified / reverified) OVERRIDES a stale submitted_unverified — so a manually- or
// async-confirmed comment shows as posted, not "đang chờ xác minh".
export function effectiveOutcome(verificationOutcome: string | undefined, correctionReason?: string): string {
  if (correctionReason === 'human_verified' || correctionReason === 'reverified') return 'verified_success';
  return verificationOutcome ?? '';
}

export function commentStatus(state: string, outcome: string): CommentStatus {
  const s = (state || '').toLowerCase();
  const o = (outcome || '').toLowerCase();
  if (s === 'queued' || s === 'planned') return { label: 'Đang chờ', severity: 'waiting' };
  if (s === 'claimed' || s === 'executing' || s === 'in_progress') return { label: 'Đang chạy', severity: 'running' };
  if (s === 'expired') return { label: 'Hết hạn — chưa chạy', severity: 'failed' };
  if (s === 'finished') {
    if (o === 'verified_success' || o === 'dom_verified') return { label: 'Đã đăng thành công', severity: 'success' };
    if (o === 'submitted_unverified' || o === 'optimistic_success') return { label: 'Đã gửi, đang chờ xác minh', severity: 'unverified' };
    return { label: 'Thất bại', severity: 'failed' };
  }
  return { label: 'Đang chờ', severity: 'waiting' };
}

// CommentAction is a status-contextual action the UI can offer next to a comment row.
// Data-only (no JSX) so it stays unit-testable and the rendering view doesn't grow.
export interface CommentAction {
  key: 'open_post' | 'human_verify' | 'retry';
  label: string;
  href?: string;     // present for link actions (open_post)
  enabled: boolean;
  confirm?: string;  // confirmation-dialog text shown before a mutating action
}

// Retryable PRE-SUBMIT failure outcomes (the comment never landed → a fresh attempt is safe).
const RETRYABLE_OUTCOMES = new Set(['target_not_reached', 'execution_failed', 'comment_button_not_found']);

// commentActions returns the status-contextual actions for a comment row (data-only,
// unit-testable). "Xác nhận đã đăng" ONLY for submitted_unverified (operator saw it on FB);
// "Thử lại" ONLY for retryable pre-submit failures. Never offer manual confirm on a failure.
export function commentActions(severity: ExecSeverity, outcome: string, targetUrl?: string): CommentAction[] {
  const o = (outcome || '').toLowerCase();
  const actions: CommentAction[] = [];
  if (targetUrl) {
    actions.push({ key: 'open_post', label: 'Mở post', href: targetUrl, enabled: true });
  }
  if (severity === 'unverified') {
    actions.push({
      key: 'human_verify',
      label: 'Xác nhận đã đăng',
      enabled: true,
      confirm: 'Chỉ xác nhận nếu bạn đã mở Facebook và nhìn thấy comment này được đăng bởi đúng tài khoản. Hành động này sẽ ghi nhận comment là đã đăng thành công.',
    });
  } else if (severity === 'failed' && RETRYABLE_OUTCOMES.has(o)) {
    actions.push({ key: 'retry', label: 'Thử lại', enabled: true });
  }
  return actions;
}

// commentReason → plain Vietnamese for a failed/unverified outcome ('' for success).
export function commentReason(outcome: string): string {
  switch ((outcome || '').toLowerCase()) {
    case 'verified_success':
    case 'dom_verified':
      return '';
    case 'submitted_unverified':
    case 'optimistic_success':
      return 'Đã bấm gửi, đang chờ hệ thống xác minh comment xuất hiện trên Facebook.';
    case 'duplicate_execution_suppressed':
      return 'Lần gửi này đã được xử lý ở lần trước (chống gửi trùng).';
    case 'comment_quality_duplicate_text':
      return 'Comment bị lặp trước khi xếp hàng.';
    case 'comment_text_doubled':
      return 'Comment bị lặp trước khi gửi.';
    case 'comment_text_mismatch':
      return 'Nội dung trong ô comment không khớp nội dung agent đã soạn.';
    case 'composer_clear_failed':
      return 'Không xoá được bản nháp cũ trong ô comment.';
    case 'comment_submit_not_found':
    case 'submit_button_not_found':
      return 'Không tìm thấy nút gửi comment.';
    case 'submit_click_failed':
      return 'Không bấm được nút gửi comment.';
    case 'comment_submit_not_confirmed':
    case 'submit_not_accepted':
      return 'Facebook chưa nhận comment sau khi bấm gửi.';
    case 'comment_button_not_found':
      return 'Không tìm thấy nút bình luận/composer trên bài viết. Có thể bài bị giới hạn bình luận, DOM chưa render, hoặc cần thử lại.';
    case 'execution_failed':
      return 'Không hoàn tất được thao tác (có thể không thấy nút bình luận, bài bị giới hạn, hoặc DOM chưa render). Có thể thử lại.';
    case 'target_not_reached':
    case 'redirected_feed':
      return 'Không mở được đúng bài viết Facebook. Có thể thử lại.';
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
