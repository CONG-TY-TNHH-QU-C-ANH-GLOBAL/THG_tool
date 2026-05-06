import { ArrowRight, MonitorPlay, Plus, RefreshCw } from 'lucide-react';

export function CyberEmptyState({ onCreate, loading }: { onCreate: () => void; loading: boolean }) {
  return (
    <div className="empty" style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 'var(--s-3)' }}>
      <div
        style={{
          width: 48,
          height: 48,
          borderRadius: 'var(--radius-md)',
          background: 'var(--accent-soft)',
          border: '1px solid var(--accent-glow)',
          color: 'var(--accent)',
          display: 'grid',
          placeItems: 'center',
        }}
      >
        <MonitorPlay size={22} />
      </div>
      <div className="eyebrow"><span className="dot" />Workspace</div>
      <h3 style={{ margin: 0, fontSize: 17, fontWeight: 600, color: 'var(--text)' }}>
        Chưa có tài khoản Facebook nào trong workspace
      </h3>
      <p style={{ margin: 0, maxWidth: 520, color: 'var(--text-mute)', fontSize: 13, lineHeight: 1.6 }}>
        Khởi tạo phiên Facebook đầu tiên để agent có browser riêng, session riêng và mọi dữ liệu automation đều gắn đúng workspace.
      </p>
      <button
        type="button"
        className="btn btn-primary"
        onClick={onCreate}
        disabled={loading}
        style={{ marginTop: 'var(--s-2)' }}
      >
        {loading ? <RefreshCw size={15} className="spin" /> : <Plus size={15} />}
        {loading ? 'Đang khởi tạo...' : 'Tạo phiên Facebook'}
        {!loading && <ArrowRight size={15} />}
      </button>
    </div>
  );
}
