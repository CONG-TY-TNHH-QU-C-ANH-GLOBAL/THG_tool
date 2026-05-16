/**
 * Bilingual VI/EN strings for the AutoFlow dashboard.
 *
 * The marketing landing page also has its own copy in
 * design-system/i18n.js; keep these dashboard strings synchronized
 * with that file when adding new keys there. Anything that appears
 * inside the authenticated app (sidebar, page headers, empty states,
 * common buttons) lives here.
 */

export type Lang = 'vi' | 'en';

export interface DashboardStrings {
  nav: {
    main: string;
    leads: string;
    chat: string;
    browser: string;
    inbox: string;
    posting: string;
    commenting: string;
    analytics: string;
    leaderboard: string;
    dataPrivate: string;
    execution: string;
    routing: string;
    system: string;
    settings: string;
  };
  topbar: {
    workspace: string;
    search: string;
    profile: string;
    logout: string;
    superadmin: string;
  };
  common: {
    save: string;
    cancel: string;
    create: string;
    delete: string;
    edit: string;
    confirm: string;
    back: string;
    next: string;
    loading: string;
    refresh: string;
    export: string;
    empty: string;
    emptyDesc: string;
    error: string;
    retry: string;
    close: string;
  };
  density: {
    label: string;
    compact: string;
    balanced: string;
    airy: string;
  };
  auth: {
    loginTitle: string;
    loginSubtitle: string;
    registerTitle: string;
    registerSubtitle: string;
    email: string;
    password: string;
    confirmPassword: string;
    name: string;
    loginCta: string;
    registerCta: string;
    forgot: string;
    noAccount: string;
    hasAccount: string;
    googleCta: string;
  };
  views: {
    leadsTitle: string;
    leadsSub: string;
    inboxTitle: string;
    inboxSub: string;
    chatTitle: string;
    chatSub: string;
    browserTitle: string;
    browserSub: string;
    postingTitle: string;
    postingSub: string;
    commentingTitle: string;
    commentingSub: string;
    leaderboardTitle: string;
    leaderboardSub: string;
    dataPrivateTitle: string;
    dataPrivateSub: string;
    settingsTitle: string;
    settingsSub: string;
  };
  inboxView: {
    eyebrow: string;
    statTotal: string;
    statActive: string;
    statPending: string;
    statConverted: string;
    filtersLabel: string;
    filterAll: string;
    listTitle: string;
    listCount: (count: number) => string;
    emptyEyebrow: string;
    emptyTitle: string;
    emptyDesc: string;
    noRecentMessage: string;
    threadKind: string;
    conversationEyebrow: string;
    conversationEmptyTitle: string;
    conversationEmptyDesc: string;
    placeholderInput: string;
    selectEyebrow: string;
    selectTitle: string;
    selectDesc: string;
  };
  commentingView: {
    eyebrow: string;
    filterAll: string;
    filterDraft: string;
    filterApproved: string;
    filterSent: string;
    filterFailed: string;
    filterRejected: string;
    statSent: string;
    statToday: string;
    statPending: string;
    statTotal: string;
    filtersLabel: string;
    listTitle: string;
    listCount: (count: number) => string;
    loadError: string;
    updateError: string;
    emptyTitle: string;
    emptyDesc: string;
    noTarget: string;
    targetFallback: string;
    contentTitle: string;
    contextTitle: string;
    fieldTarget: string;
    fieldContext: string;
    fieldMedia: string;
    contextEmpty: string;
    actionApprove: string;
    actionReject: string;
    actionOpenTarget: string;
    actionDelete: string;
    selectTitle: string;
    selectDesc: string;
    emptyValue: string;
  };
  chatView: {
    eyebrow: string;
    titlePrefix: string;
    titleHighlight: string;
    copilotName: string;
    copilotSubtitleNoAccount: string;
    copilotSubtitleWith: (account: string) => string;
    clearHistoryLabel: string;
    clearingHistoryLabel: string;
    clearError: string;
    deleteError: string;
    confirmDeleteTurn: string;
    confirmClearAll: string;
    emptyEyebrow: string;
    emptyTitle: string;
    emptyDesc: string;
    senderYou: string;
    senderSystem: string;
    senderCopilot: string;
    deleteAria: string;
    thinking: string;
    placeholderInput: string;
    sendAria: string;
    accountLabel: string;
    accountAuto: string;
    noteEyebrow: string;
    noAccountWarning: string;
    fieldRunning: string;
    fieldSession: string;
    fieldIdentity: string;
    fieldFbId: string;
    valYes: string;
    valNo: string;
    valSaved: string;
    valPending: string;
    connectorLabel: string;
    connectorSuffix: string;
    automationLabel: string;
    automationEmpty: string;
    automationEvery: (minutes: number) => string;
    automationError: string;
    schedulePending: string;
    scheduleInMinutes: (minutes: number) => string;
    copilotErrorFallback: string;
  };
  connector: {
    panelTitle: string;
    panelSub: string;
    statusReady: (online: number, total: number) => string;
    setupToggle: string;
    stepInstallTitle: string;
    stepInstallBody: string;
    stepInstallStore: string;
    stepInstallBeta: string;
    stepInstallNoConfig: string;
    stepInstallStoreHint: string;
    stepInstallNoConfigHint: string;
    stepInstallBetaHint: string;
    stepPairTitle: string;
    stepPairBody: string;
    stepPairServerHint: string;
    stepPairCopyServer: string;
    stepPairShow: string;
    stepPairHide: string;
    stepPairCopy: string;
    stepPairRefresh: string;
    stepPairCreate: string;
    stepPairExpired: string;
    stepPairRemaining: (label: string) => string;
    stepPairExpiresAt: (when: string) => string;
    stepPairHidden: string;
    stepFacebookTitle: string;
    stepFacebookBody: string;
    stepFacebookSecurity: string;
    statusEmpty: string;
    statusOnline: string;
    statusOffline: string;
    statusWaitingChrome: string;
    statusWaitingFacebook: string;
    statusStreamReady: string;
    deviceMine: string;
    deviceOther: (userId: number) => string;
    deviceAccountBound: (accountId: number) => string;
    deviceLastSeen: (when: string) => string;
    deviceDisconnect: string;
    deviceDisconnecting: string;
  };
  leadsView: {
    eyebrowSales: string;
    filterAll: string;
    filtersLabel: string;
    searchLabel: string;
    searchPlaceholder: string;
    listTitle: string;
    listCount: (count: number) => string;
    statTotal: string;
    statHot: string;
    statWarm: string;
    statAvgScore: string;
    statScore: string;
    statLastSeen: string;
    contextTitle: string;
    fieldSource: string;
    fieldClassifier: string;
    fieldNote: string;
    noteEmpty: string;
    unknownGroup: string;
    unknownSource: string;
    defaultClassifier: string;
    openFacebook: string;
    syncAgain: string;
    emptyTitle: string;
    emptyDesc: string;
    errorTitle: string;
    selectTitle: string;
    selectDesc: string;
  };
}

export const STRINGS: Record<Lang, DashboardStrings> = {
  vi: {
    nav: {
      main: 'CHÍNH',
      leads: 'Leads',
      chat: 'Chat AI',
      browser: 'Browser',
      inbox: 'Inbox',
      posting: 'Posting',
      commenting: 'Commenting',
      analytics: 'PHÂN TÍCH',
      leaderboard: 'Leaderboard',
      dataPrivate: 'Data Private',
      execution: 'Thực thi',
      routing: 'Định tuyến',
      system: 'HỆ THỐNG',
      settings: 'Settings',
    },
    topbar: {
      workspace: 'Workspace',
      search: 'Tìm lead, thread, tài khoản...',
      profile: 'Hồ sơ',
      logout: 'Đăng xuất',
      superadmin: 'SuperAdmin',
    },
    common: {
      save: 'Lưu',
      cancel: 'Hủy',
      create: 'Tạo mới',
      delete: 'Xóa',
      edit: 'Sửa',
      confirm: 'Xác nhận',
      back: 'Quay lại',
      next: 'Tiếp tục',
      loading: 'Đang tải...',
      refresh: 'Làm mới',
      export: 'Export',
      empty: 'Chưa có dữ liệu',
      emptyDesc: 'Khi có dữ liệu mới, danh sách sẽ tự cập nhật.',
      error: 'Có lỗi xảy ra',
      retry: 'Thử lại',
      close: 'Đóng',
    },
    density: {
      label: 'Mật độ',
      compact: 'Gọn',
      balanced: 'Cân đối',
      airy: 'Thoáng',
    },
    auth: {
      loginTitle: 'Đăng nhập',
      loginSubtitle: 'Tiếp tục vào workspace của bạn.',
      registerTitle: 'Tạo tài khoản',
      registerSubtitle: 'Bắt đầu workspace AutoFlow trong 2 phút.',
      email: 'Email',
      password: 'Mật khẩu',
      confirmPassword: 'Xác nhận mật khẩu',
      name: 'Họ tên',
      loginCta: 'Đăng nhập',
      registerCta: 'Tạo tài khoản',
      forgot: 'Quên mật khẩu?',
      noAccount: 'Chưa có tài khoản?',
      hasAccount: 'Đã có tài khoản?',
      googleCta: 'Tiếp tục với Google',
    },
    views: {
      leadsTitle: 'Leads',
      leadsSub: 'Tín hiệu thị trường được lọc theo hồ sơ doanh nghiệp.',
      inboxTitle: 'Inbox',
      inboxSub: 'Messenger threads được agent chăm theo conversation state.',
      chatTitle: 'Chat AI',
      chatSub: 'Một prompt - agent điều phối toàn bộ Facebook automation.',
      browserTitle: 'Browser',
      browserSub: 'Phiên Chrome thật của organization, quan sát trực tiếp từ dashboard.',
      postingTitle: 'Posting',
      postingSub: 'Hàng chờ đăng group hoặc profile với guardrail rõ ràng.',
      commentingTitle: 'Commenting',
      commentingSub: 'Watch list và composer bám đúng sales voice.',
      leaderboardTitle: 'Leaderboard',
      leaderboardSub: 'KPI sales từ dữ liệu thật, không phải mock UI.',
      dataPrivateTitle: 'Data Private',
      dataPrivateSub: 'Knowledge hub cho file, nguồn dữ liệu và business memory.',
      settingsTitle: 'Settings',
      settingsSub: 'Workspace, thành viên, phân quyền và automation policy.',
    },
    inboxView: {
      eyebrow: 'MESSENGER',
      statTotal: 'TỔNG THREAD',
      statActive: 'ACTIVE',
      statPending: 'PENDING',
      statConverted: 'CONVERTED',
      filtersLabel: 'BỘ LỌC',
      filterAll: 'Tất cả',
      listTitle: 'DANH SÁCH THREAD',
      listCount: (count) => `${count.toLocaleString('vi-VN')} hội thoại đang được quan sát`,
      emptyEyebrow: 'TRỐNG',
      emptyTitle: 'Chưa có thread nào',
      emptyDesc: 'Khi agent inbox lead, hội thoại sẽ tự động xuất hiện ở đây.',
      noRecentMessage: 'Chưa có tin nhắn gần đây',
      threadKind: 'Facebook thread',
      conversationEyebrow: 'HỘI THOẠI',
      conversationEmptyTitle: 'Chưa có tin nhắn',
      conversationEmptyDesc: 'Khi lead phản hồi hoặc agent bắt đầu hội thoại, transcript sẽ xuất hiện tại đây.',
      placeholderInput: 'Nhập tin nhắn...',
      selectEyebrow: 'GỢI Ý',
      selectTitle: 'Chọn một hội thoại',
      selectDesc: 'Chọn một thread từ danh sách bên trái để bắt đầu.',
    },
    commentingView: {
      eyebrow: 'OUTBOX',
      filterAll: 'Tất cả',
      filterDraft: 'Bản nháp',
      filterApproved: 'Đã duyệt',
      filterSent: 'Đã gửi',
      filterFailed: 'Lỗi',
      filterRejected: 'Từ chối',
      statSent: 'ĐÃ GỬI',
      statToday: 'HÔM NAY',
      statPending: 'CHỜ DUYỆT',
      statTotal: 'TỔNG',
      filtersLabel: 'BỘ LỌC',
      listTitle: 'HÀNG ĐỢI COMMENT',
      listCount: (count) => `${count.toLocaleString('vi-VN')} comment đang được theo dõi`,
      loadError: 'Không tải được outbox comment.',
      updateError: 'Không cập nhật được comment.',
      emptyTitle: 'Chưa có comment nào',
      emptyDesc: 'Khi agent draft comment cho lead, item sẽ vào hàng đợi tại đây.',
      noTarget: 'Chưa có target',
      targetFallback: 'Comment target',
      contentTitle: 'NỘI DUNG COMMENT',
      contextTitle: 'NGỮ CẢNH GỬI',
      fieldTarget: 'Target',
      fieldContext: 'Context',
      fieldMedia: 'Ảnh / media',
      contextEmpty: 'Chưa có context mở rộng cho comment này.',
      actionApprove: 'Duyệt gửi',
      actionReject: 'Từ chối',
      actionOpenTarget: 'Mở target',
      actionDelete: 'Xóa item',
      selectTitle: 'Chọn một comment',
      selectDesc: 'Chọn item từ danh sách để duyệt, từ chối hoặc mở target thật.',
      emptyValue: '(trống)',
    },
    chatView: {
      eyebrow: 'COPILOT',
      titlePrefix: 'Một prompt — ',
      titleHighlight: 'agent điều phối tất cả.',
      copilotName: 'Facebook Copilot',
      copilotSubtitleNoAccount: 'Facebook-only AI chat',
      copilotSubtitleWith: (account) => `${account} · Facebook-only`,
      clearHistoryLabel: 'Xóa lịch sử',
      clearingHistoryLabel: 'Đang xóa...',
      clearError: 'Không xóa được.',
      deleteError: 'Không thể xóa.',
      confirmDeleteTurn: 'Xóa lượt chat này?',
      confirmClearAll: 'Xóa toàn bộ lịch sử Copilot?',
      emptyEyebrow: 'PROMPT',
      emptyTitle: 'Bắt đầu bằng một nhu cầu Facebook',
      emptyDesc: 'Tìm tệp khách, phân tích group hoặc fanpage, soạn comment, inbox, posting — Copilot điều phối tất cả.',
      senderYou: 'Bạn',
      senderSystem: 'System',
      senderCopilot: 'Copilot',
      deleteAria: 'Xóa lượt chat',
      thinking: 'Copilot đang xử lý...',
      placeholderInput: 'Hỏi hoặc ra lệnh: tìm leads, phân tích group, soạn comment / inbox / post... (Ctrl+Enter)',
      sendAria: 'Gửi',
      accountLabel: 'TÀI KHOẢN',
      accountAuto: 'Tự chọn',
      noteEyebrow: 'GHI CHÚ',
      noAccountWarning: 'Chưa có Facebook workspace nào sẵn sàng. Tạo phiên Browser trước để Copilot có session thật.',
      fieldRunning: 'Đang chạy',
      fieldSession: 'Session',
      fieldIdentity: 'Identity',
      fieldFbId: 'FB ID',
      valYes: 'có',
      valNo: 'không',
      valSaved: 'đã lưu',
      valPending: 'chờ',
      connectorLabel: 'CONNECTOR',
      connectorSuffix: 'Chrome workspace',
      automationLabel: 'AUTOMATION 24/7',
      automationEmpty: 'Chưa có lịch tự động. Prompt crawl đầu tiên sẽ dạy hệ thống nguồn cần theo dõi.',
      automationEvery: (minutes) => `mỗi ${minutes} phút`,
      automationError: 'có lỗi',
      schedulePending: 'đang chờ',
      scheduleInMinutes: (minutes) => `còn ${minutes} phút`,
      copilotErrorFallback: 'Copilot chưa phản hồi.',
    },
    connector: {
      panelTitle: 'Chrome Extension',
      panelSub: 'Mỗi nhân viên tự kết nối Chrome cá nhân vào workspace để phiên Facebook chạy thật, dashboard ghi nhận stream và action log tập trung.',
      statusReady: (online, total) => `${online}/${total} extension sẵn sàng`,
      setupToggle: 'Hướng dẫn kết nối',
      stepInstallTitle: 'Bước 1 — Cài Chrome Extension',
      stepInstallBody: 'Cài extension vào Chrome cá nhân đang đăng nhập Facebook. THG không lưu mật khẩu Facebook.',
      stepInstallStore: 'Cài từ Chrome Web Store',
      stepInstallBeta: 'Cài bản beta nội bộ',
      stepInstallNoConfig: 'Chưa cấu hình Web Store',
      stepInstallStoreHint: 'Chrome Web Store sẽ cài và tự động cập nhật extension.',
      stepInstallNoConfigHint: 'Cấu hình CHROME_EXTENSION_ID hoặc CHROME_EXTENSION_STORE_URL để mở nút cài đặt.',
      stepInstallBetaHint: 'Bản beta đang bật trong giai đoạn Google xét duyệt phiên bản mới.',
      stepPairTitle: 'Bước 2 — Mã ghép nối',
      stepPairBody: 'Mỗi nhân viên tạo mã riêng. Mã chỉ dùng một lần và hết hạn sau 10 phút.',
      stepPairServerHint: 'Server workspace',
      stepPairCopyServer: 'Sao chép',
      stepPairShow: 'Hiện mã',
      stepPairHide: 'Ẩn mã',
      stepPairCopy: 'Sao chép mã',
      stepPairRefresh: 'Tạo mã mới',
      stepPairCreate: 'Tạo mã ghép nối',
      stepPairExpired: 'Mã đã hết hạn',
      stepPairRemaining: (label) => `Còn hiệu lực ${label}`,
      stepPairExpiresAt: (when) => `Hết hạn vào ${when}`,
      stepPairHidden: '••••-••••',
      stepFacebookTitle: 'Bước 3 — Mở tab Facebook đã đăng nhập',
      stepFacebookBody: 'Mở facebook.com trong Chrome đã cài extension. Khi bạn nhấn "Bắt đầu" trên một account, extension sẽ stream tab Facebook về dashboard.',
      stepFacebookSecurity: 'Chrome vẫn dùng thiết bị và IP thật của nhân viên.',
      statusEmpty: 'Chưa có Chrome nào kết nối. Cài extension, tạo mã ghép nối, dán vào popup extension rồi mở tab Facebook đã đăng nhập.',
      statusOnline: 'Online',
      statusOffline: 'Offline',
      statusWaitingChrome: 'Đang chờ Chrome kết nối',
      statusWaitingFacebook: 'Đã có thiết bị, đang chờ tab Facebook',
      statusStreamReady: 'Extension sẵn sàng stream',
      deviceMine: 'Chrome của bạn',
      deviceOther: (userId) => `Chrome của thành viên #${userId}`,
      deviceAccountBound: (accountId) => `Gắn account #${accountId}`,
      deviceLastSeen: (when) => `Lần cuối ${when}`,
      deviceDisconnect: 'Ngắt kết nối',
      deviceDisconnecting: 'Đang ngắt...',
    },
    leadsView: {
      eyebrowSales: 'SALES',
      filterAll: 'Tất cả',
      filtersLabel: 'BỘ LỌC',
      searchLabel: 'TÌM NHANH',
      searchPlaceholder: 'Tìm theo tên, nhóm hoặc role...',
      listTitle: 'DANH SÁCH LEAD',
      listCount: (count) => `${count.toLocaleString('vi-VN')} lead khớp với bộ lọc hiện tại`,
      statTotal: 'TỔNG',
      statHot: 'HOT',
      statWarm: 'WARM',
      statAvgScore: 'ĐIỂM TB',
      statScore: 'ĐIỂM',
      statLastSeen: 'CẬP NHẬT',
      contextTitle: 'NGỮ CẢNH LEAD',
      fieldSource: 'Nguồn / nhóm',
      fieldClassifier: 'Bộ phân loại',
      fieldNote: 'Tín hiệu / ghi chú',
      noteEmpty: 'Chưa có ghi chú bổ sung.',
      unknownGroup: 'Chưa xác định nhóm',
      unknownSource: 'Chưa xác định nguồn',
      defaultClassifier: 'Bộ phân loại AI',
      openFacebook: 'Mở trên Facebook',
      syncAgain: 'Đồng bộ lại',
      emptyTitle: 'Chưa có lead nào',
      emptyDesc: 'Lead sẽ tự động hiện ở đây sau khi hoàn tất hồ sơ doanh nghiệp.',
      errorTitle: 'Không tải được danh sách lead',
      selectTitle: 'Chọn một lead',
      selectDesc: 'Chọn một lead từ danh sách để xem tín hiệu thị trường và bước hành động kế tiếp.',
    },
  },
  en: {
    nav: {
      main: 'MAIN',
      leads: 'Leads',
      chat: 'Chat AI',
      browser: 'Browser',
      inbox: 'Inbox',
      posting: 'Posting',
      commenting: 'Commenting',
      analytics: 'ANALYTICS',
      leaderboard: 'Leaderboard',
      dataPrivate: 'Data Private',
      execution: 'Execution',
      routing: 'Routing',
      system: 'SYSTEM',
      settings: 'Settings',
    },
    topbar: {
      workspace: 'Workspace',
      search: 'Search leads, threads, accounts...',
      profile: 'Profile',
      logout: 'Sign out',
      superadmin: 'SuperAdmin',
    },
    common: {
      save: 'Save',
      cancel: 'Cancel',
      create: 'Create',
      delete: 'Delete',
      edit: 'Edit',
      confirm: 'Confirm',
      back: 'Back',
      next: 'Next',
      loading: 'Loading...',
      refresh: 'Refresh',
      export: 'Export',
      empty: 'No data yet',
      emptyDesc: 'New items will appear here automatically.',
      error: 'Something went wrong',
      retry: 'Retry',
      close: 'Close',
    },
    density: {
      label: 'Density',
      compact: 'Compact',
      balanced: 'Balanced',
      airy: 'Airy',
    },
    auth: {
      loginTitle: 'Sign in',
      loginSubtitle: 'Continue to your workspace.',
      registerTitle: 'Create account',
      registerSubtitle: 'Spin up an AutoFlow workspace in 2 minutes.',
      email: 'Email',
      password: 'Password',
      confirmPassword: 'Confirm password',
      name: 'Full name',
      loginCta: 'Sign in',
      registerCta: 'Create account',
      forgot: 'Forgot password?',
      noAccount: 'No account yet?',
      hasAccount: 'Already have an account?',
      googleCta: 'Continue with Google',
    },
    views: {
      leadsTitle: 'Leads',
      leadsSub: 'Market signals filtered by your business profile.',
      inboxTitle: 'Inbox',
      inboxSub: 'Messenger threads tracked with conversation state.',
      chatTitle: 'Chat AI',
      chatSub: 'One prompt - the agent orchestrates Facebook automation.',
      browserTitle: 'Browser',
      browserSub: 'Real Chrome session per org, observable from the dashboard.',
      postingTitle: 'Posting',
      postingSub: 'Group/profile post queue with guardrails.',
      commentingTitle: 'Commenting',
      commentingSub: 'Watch list + composer in your sales tone.',
      leaderboardTitle: 'Leaderboard',
      leaderboardSub: 'Real-data sales KPIs - no mocks.',
      dataPrivateTitle: 'Data Private',
      dataPrivateSub: 'Knowledge hub: files, sources, business memory.',
      settingsTitle: 'Settings',
      settingsSub: 'Workspace, members, roles, automation policy.',
    },
    inboxView: {
      eyebrow: 'MESSENGER',
      statTotal: 'TOTAL THREADS',
      statActive: 'ACTIVE',
      statPending: 'PENDING',
      statConverted: 'CONVERTED',
      filtersLabel: 'FILTERS',
      filterAll: 'All',
      listTitle: 'THREAD LIST',
      listCount: (count) => `${count.toLocaleString('en-US')} conversations in view`,
      emptyEyebrow: 'EMPTY',
      emptyTitle: 'No threads yet',
      emptyDesc: 'Threads will appear automatically once the agent starts inboxing leads.',
      noRecentMessage: 'No recent message',
      threadKind: 'Facebook thread',
      conversationEyebrow: 'CONVERSATION',
      conversationEmptyTitle: 'No messages yet',
      conversationEmptyDesc: 'The transcript will appear here once the lead replies or the agent starts the conversation.',
      placeholderInput: 'Type a message...',
      selectEyebrow: 'SELECT',
      selectTitle: 'Select a thread',
      selectDesc: 'Pick a thread from the list on the left to start.',
    },
    commentingView: {
      eyebrow: 'OUTBOX',
      filterAll: 'All',
      filterDraft: 'Draft',
      filterApproved: 'Approved',
      filterSent: 'Sent',
      filterFailed: 'Failed',
      filterRejected: 'Rejected',
      statSent: 'SENT',
      statToday: 'TODAY',
      statPending: 'PENDING',
      statTotal: 'TOTAL',
      filtersLabel: 'FILTERS',
      listTitle: 'COMMENT QUEUE',
      listCount: (count) => `${count.toLocaleString('en-US')} comments in the current queue`,
      loadError: 'Failed to load comment outbox.',
      updateError: 'Could not update the comment.',
      emptyTitle: 'No comments yet',
      emptyDesc: 'Drafted comments from the agent will appear here.',
      noTarget: 'No target',
      targetFallback: 'Comment target',
      contentTitle: 'COMMENT CONTENT',
      contextTitle: 'DELIVERY CONTEXT',
      fieldTarget: 'Target',
      fieldContext: 'Context',
      fieldMedia: 'Media',
      contextEmpty: 'No additional context stored for this comment yet.',
      actionApprove: 'Approve',
      actionReject: 'Reject',
      actionOpenTarget: 'Open target',
      actionDelete: 'Delete item',
      selectTitle: 'Select a comment',
      selectDesc: 'Pick a queue item to approve, reject, or open the real target.',
      emptyValue: '(empty)',
    },
    chatView: {
      eyebrow: 'COPILOT',
      titlePrefix: 'One prompt — ',
      titleHighlight: 'the agent runs everything.',
      copilotName: 'Facebook Copilot',
      copilotSubtitleNoAccount: 'Facebook-only AI chat',
      copilotSubtitleWith: (account) => `${account} · Facebook-only`,
      clearHistoryLabel: 'Clear history',
      clearingHistoryLabel: 'Clearing...',
      clearError: 'Could not clear.',
      deleteError: 'Could not delete.',
      confirmDeleteTurn: 'Delete this turn?',
      confirmClearAll: 'Clear entire Copilot history?',
      emptyEyebrow: 'PROMPT',
      emptyTitle: 'Start with a Facebook intent',
      emptyDesc: 'Find prospects, analyse groups or pages, draft replies, and publish posts — Copilot orchestrates everything.',
      senderYou: 'You',
      senderSystem: 'System',
      senderCopilot: 'Copilot',
      deleteAria: 'Delete turn',
      thinking: 'Copilot is thinking...',
      placeholderInput: 'Ask or command: find leads, analyse groups, draft comment / inbox / post... (Ctrl+Enter)',
      sendAria: 'Send',
      accountLabel: 'ACCOUNT',
      accountAuto: 'Auto',
      noteEyebrow: 'NOTE',
      noAccountWarning: 'No ready Facebook workspace yet. Open a Browser session first.',
      fieldRunning: 'Running',
      fieldSession: 'Session',
      fieldIdentity: 'Identity',
      fieldFbId: 'FB ID',
      valYes: 'yes',
      valNo: 'no',
      valSaved: 'saved',
      valPending: 'pending',
      connectorLabel: 'CONNECTOR',
      connectorSuffix: 'Chrome workspace',
      automationLabel: 'AUTOMATION 24/7',
      automationEmpty: 'No automation schedule yet. The first crawl prompt teaches the system what to watch.',
      automationEvery: (minutes) => `every ${minutes} min`,
      automationError: 'error',
      schedulePending: 'pending',
      scheduleInMinutes: (minutes) => `in ${minutes} min`,
      copilotErrorFallback: 'Copilot did not reply.',
    },
    connector: {
      panelTitle: 'Chrome Extension',
      panelSub: 'Each operator pairs their own Chrome to the workspace so Facebook runs on the real browser while the dashboard records stream and action logs.',
      statusReady: (online, total) => `${online}/${total} extensions ready`,
      setupToggle: 'Setup guide',
      stepInstallTitle: 'Step 1 — Install the Chrome Extension',
      stepInstallBody: 'Install the extension into the Chrome profile that is signed in to Facebook. THG never receives the Facebook password.',
      stepInstallStore: 'Install from Chrome Web Store',
      stepInstallBeta: 'Install internal beta build',
      stepInstallNoConfig: 'Web Store not configured',
      stepInstallStoreHint: 'Chrome Web Store will install and auto-update the extension.',
      stepInstallNoConfigHint: 'Set CHROME_EXTENSION_ID or CHROME_EXTENSION_STORE_URL to enable installation.',
      stepInstallBetaHint: 'Beta is enabled while Google reviews the new release.',
      stepPairTitle: 'Step 2 — Pairing code',
      stepPairBody: 'Each operator creates their own code. The code is single-use and expires in 10 minutes.',
      stepPairServerHint: 'Workspace server',
      stepPairCopyServer: 'Copy',
      stepPairShow: 'Show',
      stepPairHide: 'Hide',
      stepPairCopy: 'Copy code',
      stepPairRefresh: 'Generate new code',
      stepPairCreate: 'Generate pairing code',
      stepPairExpired: 'Code expired',
      stepPairRemaining: (label) => `Valid for ${label}`,
      stepPairExpiresAt: (when) => `Expires at ${when}`,
      stepPairHidden: '••••-••••',
      stepFacebookTitle: 'Step 3 — Open the signed-in Facebook tab',
      stepFacebookBody: 'Open facebook.com in the Chrome with the extension installed. When you press "Start" on an account, the extension streams the Facebook tab back to the dashboard.',
      stepFacebookSecurity: 'Chrome keeps the operator real device and IP.',
      statusEmpty: 'No Chrome connected yet. Install the extension, generate a pairing code, paste it into the extension popup, and open a signed-in Facebook tab.',
      statusOnline: 'Online',
      statusOffline: 'Offline',
      statusWaitingChrome: 'Waiting for Chrome',
      statusWaitingFacebook: 'Device online, waiting for Facebook tab',
      statusStreamReady: 'Extension is streaming',
      deviceMine: 'Your Chrome',
      deviceOther: (userId) => `Member #${userId} Chrome`,
      deviceAccountBound: (accountId) => `Bound to account #${accountId}`,
      deviceLastSeen: (when) => `Last seen ${when}`,
      deviceDisconnect: 'Disconnect',
      deviceDisconnecting: 'Disconnecting...',
    },
    leadsView: {
      eyebrowSales: 'SALES',
      filterAll: 'All',
      filtersLabel: 'FILTERS',
      searchLabel: 'SEARCH',
      searchPlaceholder: 'Search by name, group or role...',
      listTitle: 'LEAD LIST',
      listCount: (count) => `${count.toLocaleString('en-US')} leads match the current filter`,
      statTotal: 'TOTAL',
      statHot: 'HOT',
      statWarm: 'WARM',
      statAvgScore: 'AVG SCORE',
      statScore: 'SCORE',
      statLastSeen: 'LAST SEEN',
      contextTitle: 'LEAD CONTEXT',
      fieldSource: 'Source / group',
      fieldClassifier: 'Classifier',
      fieldNote: 'Signal / note',
      noteEmpty: 'No additional note recorded.',
      unknownGroup: 'Unknown group',
      unknownSource: 'Unknown source',
      defaultClassifier: 'AI classifier',
      openFacebook: 'Open on Facebook',
      syncAgain: 'Sync again',
      emptyTitle: 'No leads yet',
      emptyDesc: 'Leads will appear here once your business profile is set up.',
      errorTitle: 'Could not load leads',
      selectTitle: 'Select a lead',
      selectDesc: 'Pick a lead from the list to inspect the market signal and the next action.',
    },
  },
};
