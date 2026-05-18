/**
 * Bilingual VI/EN strings for the Workspace Knowledge Hub.
 * Vietnamese-first, English fallback. Loaded via the same useLang()
 * hook as the autoflow module; keys are namespaced under `knowledge`.
 */

import type { Lang } from '../../autoflow/i18n/strings';

export interface KnowledgeStrings {
  hub: {
    title: string;
    subtitle: string;
    backendNotice: string;
  };
  tabs: {
    sources: string;
    products: string;
    compliance: string;
    replay: string;
  };
  sources: {
    title: string;
    subtitle: string;
    addCta: string;
    typeLabel: string;
    syncPolicy: string;
    health: string;
    lastSync: string;
    assets: string;
    syncNow: string;
    configure: string;
    disconnect: string;
    addModalTitle: string;
    pickType: string;
    emptyTitle: string;
    emptyDesc: string;
    health_healthy: string;
    health_syncing: string;
    health_stale: string;
    health_error: string;
    health_needs_auth: string;
    types: {
      shopify: string;
      csv: string;
      google_sheets: string;
      notion: string;
      website: string;
      catalog: string;
    };
    policies: {
      realtime: string;
      hourly: string;
      daily: string;
      manual: string;
    };
  };
  products: {
    title: string;
    subtitle: string;
    searchPlaceholder: string;
    filterAll: string;
    filterApproved: string;
    filterPending: string;
    filterHidden: string;
    approve: string;
    hide: string;
    unhide: string;
    pin: string;
    unpin: string;
    boost: string;
    retrievals: string;
    conversions: string;
    pinned: string;
    boostLabel: string;
    asset_POD_product: string;
    asset_faq: string;
    asset_shipping_policy: string;
    asset_sales_playbook: string;
    asset_pricing_rule: string;
    asset_banned_claim: string;
    asset_cta: string;
    emptyTitle: string;
    emptyDesc: string;
  };
  compliance: {
    title: string;
    subtitle: string;
    addCta: string;
    patternLabel: string;
    patternPlaceholder: string;
    reasonLabel: string;
    reasonPlaceholder: string;
    severityLabel: string;
    severityBlock: string;
    severityWarn: string;
    saveCta: string;
    addedBy: string;
    triggers30d: string;
    deleteCta: string;
    emptyTitle: string;
    emptyDesc: string;
  };
  replay: {
    title: string;
    subtitle: string;
    leadContext: string;
    action: string;
    outcome: string;
    retrievedAssets: string;
    score: string;
    generatedText: string;
    operator: string;
    expand: string;
    collapse: string;
    actions: {
      comment_drafted: string;
      inbox_drafted: string;
      comment_sent: string;
      inbox_sent: string;
    };
    outcomes: {
      queued: string;
      approved: string;
      rejected: string;
      sent: string;
      failed: string;
    };
    emptyTitle: string;
    emptyDesc: string;
  };
}

const VI: KnowledgeStrings = {
  hub: {
    title: 'Workspace Knowledge Hub',
    subtitle:
      'Nguồn dữ liệu, catalog, chính sách và CTA mà AI sẽ truy xuất khi viết comment, inbox và bài đăng cho workspace này.',
    backendNotice:
      'Hệ thống Knowledge OS đang được triển khai. Dữ liệu hiện tại là mẫu để kiểm thử UI — sẽ thay bằng dữ liệu thực khi backend hoàn tất.',
  },
  tabs: {
    sources: 'Nguồn dữ liệu',
    products: 'Catalog & Assets',
    compliance: 'Compliance',
    replay: 'Operator Replay',
  },
  sources: {
    title: 'Nguồn dữ liệu',
    subtitle: 'Kết nối Shopify, CSV, Google Sheets, Notion hoặc website để AI luôn có catalog & chính sách mới nhất.',
    addCta: '+ Thêm nguồn',
    typeLabel: 'Loại',
    syncPolicy: 'Tần suất sync',
    health: 'Trạng thái',
    lastSync: 'Sync gần nhất',
    assets: 'Asset',
    syncNow: 'Sync ngay',
    configure: 'Cấu hình',
    disconnect: 'Ngắt kết nối',
    addModalTitle: 'Thêm nguồn dữ liệu',
    pickType: 'Chọn loại nguồn',
    emptyTitle: 'Chưa có nguồn nào',
    emptyDesc: 'Thêm Shopify, CSV hoặc website để hệ thống bắt đầu xây catalog.',
    health_healthy: 'Đang chạy ổn',
    health_syncing: 'Đang sync',
    health_stale: 'Dữ liệu cũ',
    health_error: 'Lỗi',
    health_needs_auth: 'Cần xác thực lại',
    types: {
      shopify: 'Shopify',
      csv: 'CSV / Bảng tính',
      google_sheets: 'Google Sheets',
      notion: 'Notion',
      website: 'Website crawler',
      catalog: 'Catalog',
    },
    policies: {
      realtime: 'Realtime',
      hourly: 'Mỗi giờ',
      daily: 'Mỗi ngày',
      manual: 'Thủ công',
    },
  },
  products: {
    title: 'Catalog & Assets',
    subtitle: 'Duyệt, ghim, hoặc tăng độ ưu tiên cho từng SKU / FAQ / CTA mà AI sẽ truy xuất.',
    searchPlaceholder: 'Tìm theo tên, tag, hoặc nguồn…',
    filterAll: 'Tất cả',
    filterApproved: 'Đã duyệt',
    filterPending: 'Chờ duyệt',
    filterHidden: 'Đã ẩn',
    approve: 'Duyệt',
    hide: 'Ẩn',
    unhide: 'Bỏ ẩn',
    pin: 'Ghim',
    unpin: 'Bỏ ghim',
    boost: 'Boost',
    retrievals: 'Lượt truy xuất 30 ngày',
    conversions: 'Conversion 30 ngày',
    pinned: 'Đã ghim',
    boostLabel: 'Ưu tiên',
    asset_POD_product: 'Sản phẩm POD',
    asset_faq: 'FAQ',
    asset_shipping_policy: 'Chính sách ship',
    asset_sales_playbook: 'Playbook',
    asset_pricing_rule: 'Giá',
    asset_banned_claim: 'Banned claim',
    asset_cta: 'CTA',
    emptyTitle: 'Chưa có asset nào',
    emptyDesc: 'Khi nguồn dữ liệu sync xong, các SKU và policy sẽ xuất hiện ở đây.',
  },
  compliance: {
    title: 'Compliance Center',
    subtitle: 'Các tuyên bố mà AI tuyệt đối không được dùng khi viết comment / inbox. Mọi vi phạm bị chặn ở runtime.',
    addCta: '+ Thêm banned claim',
    patternLabel: 'Cụm từ cấm',
    patternPlaceholder: 've.d "best price guaranteed"',
    reasonLabel: 'Lý do',
    reasonPlaceholder: 'Vì sao claim này không được dùng (legal, brand voice, etc.)',
    severityLabel: 'Mức độ',
    severityBlock: 'Chặn',
    severityWarn: 'Cảnh báo',
    saveCta: 'Lưu',
    addedBy: 'Người thêm',
    triggers30d: 'Số lần kích hoạt 30 ngày',
    deleteCta: 'Xoá',
    emptyTitle: 'Chưa có claim nào bị cấm',
    emptyDesc: 'Thêm cụm từ mà bạn không muốn AI sử dụng (legal, brand voice, etc.).',
  },
  replay: {
    title: 'Operator Replay',
    subtitle: 'Mỗi hành động AI thực hiện: ngữ cảnh lead, asset được truy xuất, text sinh ra, và outcome.',
    leadContext: 'Ngữ cảnh lead',
    action: 'Hành động',
    outcome: 'Kết quả',
    retrievedAssets: 'Asset được truy xuất',
    score: 'Score',
    generatedText: 'Văn bản AI sinh ra',
    operator: 'Người / Agent thực hiện',
    expand: 'Xem chi tiết',
    collapse: 'Ẩn chi tiết',
    actions: {
      comment_drafted: 'Comment draft',
      inbox_drafted: 'Inbox draft',
      comment_sent: 'Comment đã gửi',
      inbox_sent: 'Inbox đã gửi',
    },
    outcomes: {
      queued: 'Đang chờ',
      approved: 'Đã duyệt',
      rejected: 'Bị từ chối',
      sent: 'Đã gửi',
      failed: 'Lỗi',
    },
    emptyTitle: 'Chưa có hoạt động',
    emptyDesc: 'Khi AI bắt đầu viết comment / inbox, mỗi hành động sẽ được ghi lại ở đây.',
  },
};

const EN: KnowledgeStrings = {
  hub: {
    title: 'Workspace Knowledge Hub',
    subtitle:
      'Data sources, catalog, policies and CTAs that the AI retrieves when writing comments, inbox and posts for this workspace.',
    backendNotice:
      'Knowledge OS is being built. Current data is sample fixtures for UI review — will be swapped for live data once the backend is ready.',
  },
  tabs: {
    sources: 'Sources',
    products: 'Catalog & Assets',
    compliance: 'Compliance',
    replay: 'Operator Replay',
  },
  sources: {
    title: 'Data sources',
    subtitle: 'Connect Shopify, CSV, Google Sheets, Notion or a website so the AI always has up-to-date catalog & policy.',
    addCta: '+ Add source',
    typeLabel: 'Type',
    syncPolicy: 'Sync frequency',
    health: 'Health',
    lastSync: 'Last sync',
    assets: 'Assets',
    syncNow: 'Sync now',
    configure: 'Configure',
    disconnect: 'Disconnect',
    addModalTitle: 'Add a data source',
    pickType: 'Pick a source type',
    emptyTitle: 'No sources yet',
    emptyDesc: 'Connect Shopify, a CSV, or a website to start building the catalog.',
    health_healthy: 'Healthy',
    health_syncing: 'Syncing',
    health_stale: 'Stale',
    health_error: 'Error',
    health_needs_auth: 'Re-auth needed',
    types: {
      shopify: 'Shopify',
      csv: 'CSV / Spreadsheet',
      google_sheets: 'Google Sheets',
      notion: 'Notion',
      website: 'Website crawler',
      catalog: 'Catalog',
    },
    policies: {
      realtime: 'Realtime',
      hourly: 'Hourly',
      daily: 'Daily',
      manual: 'Manual',
    },
  },
  products: {
    title: 'Catalog & assets',
    subtitle: 'Approve, pin, or boost the retrieval priority of every SKU / FAQ / CTA the AI can use.',
    searchPlaceholder: 'Search by title, tag, or source…',
    filterAll: 'All',
    filterApproved: 'Approved',
    filterPending: 'Pending',
    filterHidden: 'Hidden',
    approve: 'Approve',
    hide: 'Hide',
    unhide: 'Unhide',
    pin: 'Pin',
    unpin: 'Unpin',
    boost: 'Boost',
    retrievals: 'Retrievals (30d)',
    conversions: 'Conversions (30d)',
    pinned: 'Pinned',
    boostLabel: 'Priority',
    asset_POD_product: 'POD product',
    asset_faq: 'FAQ',
    asset_shipping_policy: 'Shipping policy',
    asset_sales_playbook: 'Playbook',
    asset_pricing_rule: 'Pricing',
    asset_banned_claim: 'Banned claim',
    asset_cta: 'CTA',
    emptyTitle: 'No assets yet',
    emptyDesc: 'Once a source finishes syncing, every SKU / policy will show up here.',
  },
  compliance: {
    title: 'Compliance Center',
    subtitle: 'Claims the AI must NEVER use when writing comments / inbox. Violations are blocked at runtime.',
    addCta: '+ Add banned claim',
    patternLabel: 'Banned phrase',
    patternPlaceholder: 'e.g. "best price guaranteed"',
    reasonLabel: 'Reason',
    reasonPlaceholder: 'Why this claim is banned (legal, brand voice, etc.)',
    severityLabel: 'Severity',
    severityBlock: 'Block',
    severityWarn: 'Warn',
    saveCta: 'Save',
    addedBy: 'Added by',
    triggers30d: 'Triggers (30d)',
    deleteCta: 'Delete',
    emptyTitle: 'No banned claims yet',
    emptyDesc: 'Add phrases you do not want the AI to use (legal, brand voice, etc.).',
  },
  replay: {
    title: 'Operator Replay',
    subtitle: 'Every AI action with the lead context, retrieved assets, generated text, and final outcome.',
    leadContext: 'Lead context',
    action: 'Action',
    outcome: 'Outcome',
    retrievedAssets: 'Retrieved assets',
    score: 'Score',
    generatedText: 'Generated text',
    operator: 'Operator / Agent',
    expand: 'Show details',
    collapse: 'Hide details',
    actions: {
      comment_drafted: 'Comment drafted',
      inbox_drafted: 'Inbox drafted',
      comment_sent: 'Comment sent',
      inbox_sent: 'Inbox sent',
    },
    outcomes: {
      queued: 'Queued',
      approved: 'Approved',
      rejected: 'Rejected',
      sent: 'Sent',
      failed: 'Failed',
    },
    emptyTitle: 'No activity yet',
    emptyDesc: 'Once the AI starts writing comments / inbox, every action is logged here.',
  },
};

export const KNOWLEDGE_STRINGS: Record<Lang, KnowledgeStrings> = { vi: VI, en: EN };
