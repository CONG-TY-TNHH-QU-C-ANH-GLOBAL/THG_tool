import CreateFacebookWorkspace from './components/CreateFacebookWorkspace';
import FacebookWorkspaceApp from './components/FacebookWorkspaceApp';
import type { ServiceModule } from '../../platform/services/types';

// Encode the canonical workspace ID for Facebook. Today's storage uses
// numeric org_id; the canonical contract id is "ws_<n>". This helper is
// the single place that knows about the proxy — everywhere else reads the
// resolved value off the contract.
export function facebookWorkspaceIdOf(orgId: number | undefined | null): string | undefined {
  return orgId && orgId > 0 ? `ws_${orgId}` : undefined;
}

export const facebookServiceModule: ServiceModule = {
  descriptor: {
    slug: 'facebook',
    internalName: 'facebook-automation',
    publicLabel: 'Facebook Automation',
    category: 'automation',
    rolloutStage: 'ga',
    availability: 'public',
    version: 1,
    displayOrder: 10,
  },
  views: {
    createWorkspace: CreateFacebookWorkspace,
    workspace: FacebookWorkspaceApp,
  },
  // Resolvers — every semantic value flows through these. Today the FB module
  // synthesises from auth state; PR 2 swaps these for thin pass-throughs that
  // read GET /api/platform/services. Consumers never depend on the implementation.
  resolveStatus: () => 'available',
  resolveWorkspace: (user) => {
    const trace = { source: 'org_id_proxy', resolver: 'facebook.resolveWorkspace', confidence: 'legacy' as const };
    const workspaceId = facebookWorkspaceIdOf(user?.org_id);
    if (!workspaceId) return { state: 'none', trace };
    return { state: 'ready', workspaceId, trace };
  },
  resolveCapabilities: () => ({
    // Capability = "the FB service supports this feature". NOT a permission.
    // Whether the user may actually run it is resolveAccess + RBAC.
    multiWorkspace: false,
    browserAutomation: true,
    aiAgents: true,
  }),
  resolveAccess: (user) => {
    const trace = { source: 'auth_state', resolver: 'facebook.resolveAccess', confidence: 'legacy' as const };
    if (!user) return { access: 'admin_blocked', reason: 'not authenticated', trace };
    return { access: 'granted', trace };
  },
};
