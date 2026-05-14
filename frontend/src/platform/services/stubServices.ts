import type { ComponentType } from 'react';
import { createElement } from 'react';
import type { ServiceModule } from './types';

// Coming Soon services — registered locally so the platform projection
// shows the multi-service vision (FB + Taobao + 1688) on first paint.
// The authoritative /api/platform/services response from the backend
// mirrors these (see internal/platform/services/bootstrap.go); local
// stubs are the optimistic fallback used while that call is in flight.
//
// resolveStatus → 'unavailable' so resolveWorkspacePresentation renders
// the "Sắp ra mắt" badge and disables the click, exactly matching the
// backend.

const ComingSoonView: ComponentType = () =>
  createElement(
    'div',
    {
      style: {
        flex: 1,
        display: 'grid',
        placeItems: 'center',
        padding: 24,
      },
    },
    createElement(
      'div',
      { className: 'card', style: { maxWidth: 420, padding: 28, textAlign: 'center' } },
      createElement('h2', { style: { fontSize: 18, marginBottom: 8 } }, 'Sắp ra mắt'),
      createElement(
        'p',
        { style: { color: 'var(--text-mute)', fontSize: 13 } },
        'Service này đang được phát triển. Theo dõi roadmap trong workspace của bạn.',
      ),
    ),
  );

export const taobaoServiceModule: ServiceModule = {
  descriptor: {
    slug: 'taobao',
    internalName: 'taobao-sourcing',
    publicLabel: 'Taobao Sourcing',
    category: 'automation',
    rolloutStage: 'alpha',
    availability: 'public',
    version: 1,
    displayOrder: 20,
  },
  views: {
    createWorkspace: ComingSoonView,
    workspace: ComingSoonView,
  },
  resolveStatus: () => 'unavailable',
  resolveWorkspace: () => ({
    state: 'none',
    trace: { source: 'stub', resolver: 'taobao.resolveWorkspace', confidence: 'legacy' },
  }),
  resolveCapabilities: () => ({
    multiWorkspace: false,
    browserAutomation: true,
    aiAgents: true,
  }),
  resolveAccess: () => ({
    access: 'granted',
    trace: { source: 'stub', resolver: 'taobao.resolveAccess', confidence: 'legacy' },
  }),
};

export const alibaba1688ServiceModule: ServiceModule = {
  descriptor: {
    slug: 'alibaba_1688',
    internalName: '1688-sourcing',
    publicLabel: '1688 Sourcing',
    category: 'automation',
    rolloutStage: 'alpha',
    availability: 'public',
    version: 1,
    displayOrder: 30,
  },
  views: {
    createWorkspace: ComingSoonView,
    workspace: ComingSoonView,
  },
  resolveStatus: () => 'unavailable',
  resolveWorkspace: () => ({
    state: 'none',
    trace: { source: 'stub', resolver: 'alibaba_1688.resolveWorkspace', confidence: 'legacy' },
  }),
  resolveCapabilities: () => ({
    multiWorkspace: false,
    browserAutomation: true,
    aiAgents: true,
  }),
  resolveAccess: () => ({
    access: 'granted',
    trace: { source: 'stub', resolver: 'alibaba_1688.resolveAccess', confidence: 'legacy' },
  }),
};
