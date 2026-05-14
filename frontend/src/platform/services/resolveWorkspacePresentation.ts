import type { PlatformService } from './types';

export type PresentationTone = 'success' | 'info' | 'warning' | 'danger' | 'neutral';

export interface WorkspacePresentation {
  badge: { label: string; tone: PresentationTone };
  primaryAction: { label: string; href: string; enabled: boolean } | null;
  canEnter: boolean;
  canCreate: boolean;
  canReviewInvite: boolean;
  canRetry: boolean;
  description?: string;
}

interface Strings {
  active: string;
  initializing: string;
  inviteRequired: string;
  workspaceSuspended: string;
  billing: string;
  regionLocked: string;
  adminBlocked: string;
  notInitialised: string;
  serviceSuspended: string;
  comingSoon: string;
  open: string;
  resume: string;
  createPrefix: string;
  createSuffix: string;
  review: string;
  contact: string;
}

const STRINGS: Record<'vi' | 'en', Strings> = {
  vi: {
    active: 'Đang hoạt động',
    initializing: 'Đang khởi tạo',
    inviteRequired: 'Chờ chấp nhận lời mời',
    workspaceSuspended: 'Workspace tạm dừng',
    billing: 'Vấn đề thanh toán',
    regionLocked: 'Chưa hỗ trợ khu vực',
    adminBlocked: 'Bị chặn',
    notInitialised: 'Chưa khởi tạo',
    serviceSuspended: 'Service tạm dừng',
    comingSoon: 'Sắp ra mắt',
    open: 'Mở workspace',
    resume: 'Tiếp tục khởi tạo',
    createPrefix: 'Tạo workspace ',
    createSuffix: '',
    review: 'Xem lời mời',
    contact: 'Liên hệ hỗ trợ',
  },
  en: {
    active: 'Active',
    initializing: 'Initialising',
    inviteRequired: 'Pending invite',
    workspaceSuspended: 'Workspace suspended',
    billing: 'Billing issue',
    regionLocked: 'Region locked',
    adminBlocked: 'Blocked',
    notInitialised: 'Not initialised',
    serviceSuspended: 'Service suspended',
    comingSoon: 'Coming soon',
    open: 'Open workspace',
    resume: 'Resume initialisation',
    createPrefix: 'Create ',
    createSuffix: ' Workspace',
    review: 'Review invite',
    contact: 'Contact support',
  },
};

export function resolveWorkspacePresentation(svc: PlatformService, lang: 'vi' | 'en'): WorkspacePresentation {
  const s = STRINGS[lang];
  const baseHref = `/services/${svc.slug}/workspaces`;
  const firstWord = svc.label.split(' ')[0];

  // Service-level gates first — overrule everything below.
  if (svc.status === 'unavailable') {
    return {
      badge: { label: s.comingSoon, tone: 'neutral' },
      primaryAction: null,
      canEnter: false, canCreate: false, canReviewInvite: false, canRetry: false,
    };
  }
  if (svc.status === 'suspended') {
    return {
      badge: { label: s.serviceSuspended, tone: 'warning' },
      primaryAction: null,
      canEnter: false, canCreate: false, canReviewInvite: false, canRetry: false,
      description: svc.reason,
    };
  }

  // Access gates — overrule workspace-state for entry decisions.
  if (svc.access === 'billing_blocked') {
    return {
      badge: { label: s.billing, tone: 'danger' },
      primaryAction: { label: s.contact, href: '/services', enabled: true },
      canEnter: false, canCreate: false, canReviewInvite: false, canRetry: true,
      description: svc.accessReason,
    };
  }
  if (svc.access === 'region_locked') {
    return {
      badge: { label: s.regionLocked, tone: 'warning' },
      primaryAction: null,
      canEnter: false, canCreate: false, canReviewInvite: false, canRetry: false,
      description: svc.accessReason,
    };
  }
  if (svc.access === 'admin_blocked') {
    return {
      badge: { label: s.adminBlocked, tone: 'danger' },
      primaryAction: null,
      canEnter: false, canCreate: false, canReviewInvite: false, canRetry: false,
      description: svc.accessReason,
    };
  }
  if (svc.access === 'invite_required') {
    return {
      badge: { label: s.inviteRequired, tone: 'info' },
      primaryAction: { label: s.review, href: `${baseHref}/new`, enabled: true },
      canEnter: false, canCreate: false, canReviewInvite: true, canRetry: false,
      description: svc.accessReason,
    };
  }

  // Workspace lifecycle.
  switch (svc.workspaceState) {
    case 'ready':
      return {
        badge: { label: s.active, tone: 'success' },
        primaryAction: { label: s.open, href: `${baseHref}/${svc.workspaceId}`, enabled: Boolean(svc.workspaceId) },
        canEnter: true, canCreate: false, canReviewInvite: false, canRetry: false,
      };
    case 'initializing':
      return {
        badge: { label: s.initializing, tone: 'info' },
        primaryAction: { label: s.resume, href: `${baseHref}/new`, enabled: true },
        canEnter: false, canCreate: true, canReviewInvite: false, canRetry: false,
      };
    case 'suspended':
      return {
        badge: { label: s.workspaceSuspended, tone: 'warning' },
        primaryAction: null,
        canEnter: false, canCreate: false, canReviewInvite: false, canRetry: true,
        description: svc.reason,
      };
    case 'none':
    default:
      return {
        badge: { label: s.notInitialised, tone: 'neutral' },
        primaryAction: { label: `${s.createPrefix}${firstWord}${s.createSuffix}`, href: `${baseHref}/new`, enabled: true },
        canEnter: false, canCreate: true, canReviewInvite: false, canRetry: false,
      };
  }
}
