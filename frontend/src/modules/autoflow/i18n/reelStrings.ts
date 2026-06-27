// Reel-feature i18n, kept OUT of the large strings.ts god file (Engineering Guardrails:
// do not grow legacy large files). Same Lang union + same useLang().lang drives VI/EN.
import type { Lang } from './strings';

export interface ReelStrings {
  eyebrow: string; title: string; subtitle: string;
  refresh: string; createCta: string; createCardTitle: string;
  fAll: string; fDraft: string; fRendering: string; fReady: string; fPublished: string; fFailed: string;
  emptyTitle: string; emptyDesc: string;
  briefLabel: string; briefPlaceholder: string; kwLabel: string; kwPlaceholder: string;
  advanced: string; durationLabel: string; durationHint: (s: number) => string;
  cancel: string; submit: string; submitting: string; briefTooShort: string;
  scriptSummary: (v: number, n: number, sec: number, cost: string) => string;
  sDraft: string; sReady: string; sRendering: string; sStuck: string; sAssembled: string; sPosting: string; sPublished: string; sFailed: string;
  viewScript: string; approve: string; escalate: string; preview: string; publish: string; published: string; retry: string; download: string;
  watchVideo: string; hideVideo: string; videoLoading: string;
  confirmApprove: (id: number) => string;
  renderingLabel: (done: number, total: number, cost: string) => string;
  stuckBanner: string; assembledLabel: string;
  captionLabel: string; saveScript: string; verifyHeading: string;
  pubAccountLabel: string; pubAccountEmpty: string; pubTargetLabel: string; pubTargetPlaceholder: string; pubSubmit: string;
  pubQueued: string; pubDuplicate: string;
}

const vi: ReelStrings = {
  eyebrow: 'REEL STUDIO', title: 'Reel Studio',
  subtitle: 'AI viết kịch bản → render video ngắn → đăng lên trang qua kết nối Facebook có sẵn.',
  refresh: 'Làm mới', createCta: 'Tạo Reel', createCardTitle: 'Tạo reel mới',
  fAll: 'Tất cả', fDraft: 'Nháp', fRendering: 'Đang render', fReady: 'Sẵn sàng', fPublished: 'Đã đăng', fFailed: 'Lỗi',
  emptyTitle: 'Chưa có reel', emptyDesc: 'Tạo reel đầu tiên — AI sẽ tự sinh kịch bản để bạn duyệt.',
  briefLabel: 'Ý tưởng / phong cách', briefPlaceholder: 'vd: Kể chuyện seller ship chậm mất khách, kết bằng giải pháp fulfill US 3-5 ngày',
  kwLabel: 'Từ khoá (phẩy)', kwPlaceholder: 'vd: BUNG, fulfill US, 3-5 ngày',
  advanced: 'Nâng cao', durationLabel: 'Thời lượng', durationHint: (s) => `≈ ${s}s · 6–8 shot 4–5s`,
  cancel: 'Huỷ', submit: 'Tạo & sinh kịch bản', submitting: 'Đang tạo…', briefTooShort: 'Ý tưởng cần ít nhất 20 ký tự.',
  scriptSummary: (v, n, sec, cost) => `KỊCH BẢN v${v} · ${n} shot · ${sec}s · $${cost}`,
  sDraft: 'Nháp', sReady: 'Chờ duyệt', sRendering: 'Đang render', sStuck: 'Render kẹt', sAssembled: 'Sẵn sàng đăng', sPosting: 'Đang đăng', sPublished: 'Đã đăng', sFailed: 'Lỗi',
  viewScript: 'Xem/Sửa kịch bản', approve: 'Duyệt → Render', escalate: 'Báo xử lý', preview: 'Xem trước', publish: 'Đăng', published: 'Đã đăng ✓', retry: 'Thử lại shot lỗi', download: 'Tải kết quả',
  watchVideo: 'Xem video', hideVideo: 'Ẩn video', videoLoading: 'Đang tải video…',
  confirmApprove: (id) => `Render reel #${id} sẽ TIÊU CHI PHÍ và KHÔNG THỂ HUỶ khi đã bắt đầu. Tiếp tục?`,
  renderingLabel: (done, total, cost) => `Đang render ${done}/${total} shot · $${cost}`,
  stuckBanner: 'Render kẹt (lease hết hạn). Cần người xử lý — hệ thống KHÔNG tự render lại để tránh tiêu tiền lần hai.',
  assembledLabel: 'Video đã ghép, sẵn sàng đăng.',
  captionLabel: 'Caption', saveScript: 'Lưu (version +1)', verifyHeading: 'Cần xác minh',
  pubAccountLabel: 'Account đăng', pubAccountEmpty: 'Chưa có account sẵn sàng. Mở Kết nối Facebook, pair extension và đăng nhập, rồi quay lại.',
  pubTargetLabel: 'Target URL', pubTargetPlaceholder: 'https://facebook.com/me (mỗi reel một đích để tránh trùng)', pubSubmit: 'Đăng reel',
  pubQueued: 'Đã đưa vào hàng đợi đăng.', pubDuplicate: 'Đích này vừa được đăng trong 24h (dedup của outbound). Đổi target/account rồi thử lại.',
};

const en: ReelStrings = {
  eyebrow: 'REEL STUDIO', title: 'Reel Studio',
  subtitle: 'AI writes the script → renders a short video → posts to the page via the existing Facebook connector.',
  refresh: 'Refresh', createCta: 'New Reel', createCardTitle: 'Create a reel',
  fAll: 'All', fDraft: 'Draft', fRendering: 'Rendering', fReady: 'Ready', fPublished: 'Posted', fFailed: 'Failed',
  emptyTitle: 'No reels yet', emptyDesc: 'Create your first reel — AI drafts the script for you to approve.',
  briefLabel: 'Idea / style', briefPlaceholder: 'e.g. A seller loses customers to slow shipping, ending with a 3-5 day US fulfill solution',
  kwLabel: 'Keywords (comma)', kwPlaceholder: 'e.g. BUNG, fulfill US, 3-5 days',
  advanced: 'Advanced', durationLabel: 'Duration', durationHint: (s) => `≈ ${s}s · 6–8 shots of 4–5s`,
  cancel: 'Cancel', submit: 'Create & draft script', submitting: 'Creating…', briefTooShort: 'Idea needs at least 20 characters.',
  scriptSummary: (v, n, sec, cost) => `SCRIPT v${v} · ${n} shots · ${sec}s · $${cost}`,
  sDraft: 'Draft', sReady: 'Awaiting approval', sRendering: 'Rendering', sStuck: 'Render stuck', sAssembled: 'Ready to post', sPosting: 'Posting', sPublished: 'Posted', sFailed: 'Failed',
  viewScript: 'View/Edit script', approve: 'Approve → Render', escalate: 'Escalate', preview: 'Preview', publish: 'Post', published: 'Posted ✓', retry: 'Retry failed shot', download: 'Download result',
  watchVideo: 'Watch video', hideVideo: 'Hide video', videoLoading: 'Loading video…',
  confirmApprove: (id) => `Rendering reel #${id} WILL SPEND money and CANNOT be cancelled once started. Continue?`,
  renderingLabel: (done, total, cost) => `Rendering ${done}/${total} shots · $${cost}`,
  stuckBanner: 'Render stuck (lease expired). Needs a human — the system will NOT auto re-render to avoid double charging.',
  assembledLabel: 'Video assembled, ready to post.',
  captionLabel: 'Caption', saveScript: 'Save (version +1)', verifyHeading: 'Needs verification',
  pubAccountLabel: 'Posting account', pubAccountEmpty: 'No ready account. Open Facebook Connect, pair the extension and log in, then come back.',
  pubTargetLabel: 'Target URL', pubTargetPlaceholder: 'https://facebook.com/me (one target per reel to avoid dedup)', pubSubmit: 'Post reel',
  pubQueued: 'Queued for posting.', pubDuplicate: 'This target was just posted within 24h (outbound dedup). Change target/account and retry.',
};

export const REEL_STRINGS: Record<Lang, ReelStrings> = { vi, en };
