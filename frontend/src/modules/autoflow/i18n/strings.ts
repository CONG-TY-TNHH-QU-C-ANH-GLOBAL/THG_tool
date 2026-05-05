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
      leadsSub: 'Tín hiệu thị trường được lọc bởi business profile.',
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
  },
};
