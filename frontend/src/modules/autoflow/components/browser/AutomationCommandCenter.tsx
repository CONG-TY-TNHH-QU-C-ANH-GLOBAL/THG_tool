import { Plus, RefreshCw, Workflow } from 'lucide-react';
import type { LocalConnector, LocalConnectorAction } from '../../types';
import { isDashboardStreamConnector } from './browserHelpers';

export function AutomationCommandCenter({
  workspaces,
  connectors,
  actions,
  running,
  loading,
  onRefresh,
  onNewSession,
}: {
  workspaces: Array<{ loggedIn: boolean; running: boolean; fbUserId?: string; browserState?: string }>;
  connectors: LocalConnector[];
  actions: LocalConnectorAction[];
  running: number;
  loading: boolean;
  onRefresh: () => void;
  onNewSession: () => void;
}) {
  const extensionOnline = connectors.filter(c => c.online && isDashboardStreamConnector(c)).length;
  const facebookReady = workspaces.filter(w => w.loggedIn || Boolean(w.fbUserId) || w.browserState === 'local_ready').length;
  const doneActions = actions.filter(a => a.status === 'done').length;
  const failedActions = actions.filter(a => a.status === 'failed').length;
  const pipeline = [
    { label: 'Leads thật', active: running > 0 || facebookReady > 0 },
    { label: 'Market Signal Gate', active: facebookReady > 0 },
    { label: 'Sales Voice Memory', active: true },
    { label: 'Conversation State', active: actions.length > 0 },
    { label: 'Auto Action', active: doneActions > 0 || actions.some(a => a.status === 'claimed' || a.status === 'pending') },
    { label: 'Telegram / Dashboard log', active: doneActions > 0 || failedActions > 0 },
  ];

  return (
    <section className="af-command-center">
      <div className="af-command-copy">
        <span className="af-command-kicker"><Workflow size={14} /> Production Automation Flow</span>
        <h2>Trung tâm điều phối Facebook Sales Intelligence</h2>
        <p>Leads thật → Market Signal Gate → Sales Voice Memory → Conversation State → Auto Action → Telegram/Dashboard log.</p>
      </div>
      <div className="af-command-metrics">
        <div><span>{workspaces.length}</span><small>Tài khoản Facebook</small></div>
        <div><span>{facebookReady}</span><small>Session sẵn sàng</small></div>
        <div><span>{extensionOnline}</span><small>Extension online</small></div>
        <div><span>{doneActions}/{actions.length}</span><small>Action gần đây</small></div>
      </div>
      <div className="af-command-actions">
        <button type="button" className="af-btn af-btn-ghost" onClick={onRefresh}>
          <RefreshCw size={14} /> Làm mới
        </button>
        <button type="button" className="af-btn af-btn-primary" onClick={onNewSession} disabled={loading}>
          {loading ? <RefreshCw size={14} className="spin" /> : <Plus size={14} />}
          {loading ? 'Đang mở' : 'Phiên Facebook mới'}
        </button>
      </div>
      <div className="af-pipeline-rail">
        {pipeline.map((step, index) => (
          <div key={step.label} className={`af-pipeline-step ${step.active ? 'is-active' : ''}`}>
            <span>{index + 1}</span>
            <p>{step.label}</p>
          </div>
        ))}
      </div>
    </section>
  );
}
