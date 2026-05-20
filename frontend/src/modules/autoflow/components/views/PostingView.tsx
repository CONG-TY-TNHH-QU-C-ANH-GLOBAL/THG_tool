import { useEffect, useState } from 'react';
import { ExternalLink, RefreshCw, Trash2 } from 'lucide-react';
import { deleteAllOutboundPosts, deleteOutbox, getOutbox, OutboundMessage } from '../../services/outboxService';

interface PostingViewProps { orgId: string; isAdmin: boolean; }

// AUTONOMOUS-VERIFIED-EXECUTION (project goal, May-2026): no
// draft/approve gate. Filter only by execution-lifecycle state.
type PostFilter = 'all' | 'planned' | 'executing' | 'verified' | 'failed';

const FILTERS: { label: string; value: PostFilter }[] = [
  { label: 'Tất cả', value: 'all' },
  { label: 'Đã lên kế hoạch', value: 'planned' },
  { label: 'Đang thực thi', value: 'executing' },
  { label: 'Đã xác nhận', value: 'verified' },
  { label: 'Thất bại', value: 'failed' },
];

function matchesPostFilter(status: string, filter: PostFilter): boolean {
  switch (filter) {
    case 'all':
      return true;
    case 'planned':
      return status === 'approved';
    case 'executing':
      return status === 'sending';
    case 'verified':
      return status === 'sent';
    case 'failed':
      return status === 'failed' || status === 'rejected';
  }
}

function postStatusLabel(status: string): string {
  switch (status) {
    case 'approved': return 'ĐÃ LÊN KẾ HOẠCH';
    case 'sending':  return 'ĐANG THỰC THI';
    case 'sent':     return 'ĐÃ XÁC NHẬN';
    case 'failed':   return 'THẤT BẠI';
    case 'rejected': return 'TỪ CHỐI';
    default:         return status.toUpperCase();
  }
}

export default function PostingView({ orgId, isAdmin }: PostingViewProps) {
  const [messages, setMessages] = useState<OutboundMessage[]>([]);
  const [filter, setFilter] = useState<PostFilter>('all');
  const [loading, setLoading] = useState(true);
  const [msg, setMsg] = useState('');
  const [deletingAll, setDeletingAll] = useState(false);
  void orgId;

  const load = async () => {
    setLoading(true);
    try {
      const r = await getOutbox({ limit: 150 });
      setMessages((r.messages ?? []).filter(m => m.type === 'group_post' || m.type === 'profile_post'));
    } catch (err) {
      setMsg(err instanceof Error ? err.message : 'Không tải được outbox posting.');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { load(); }, []);

  const transition = async (id: number, action: 'delete') => {
    setMsg('');
    try {
      if (action === 'delete') await deleteOutbox(id);
      await load();
    } catch (err) {
      setMsg(err instanceof Error ? err.message : 'Không cập nhật được bài viết.');
    }
  };

  const handleDeleteAll = async () => {
    if (deletingAll) return;
    if (typeof window !== 'undefined') {
      const ok = window.confirm(`Xoá TẤT CẢ ${messages.length} bài posting trong hàng đợi? Không thể hoàn tác.`);
      if (!ok) return;
    }
    setDeletingAll(true);
    setMsg('');
    try {
      const res = await deleteAllOutboundPosts();
      await load();
      if (typeof window !== 'undefined') {
        window.alert(`Đã xoá ${res.deleted} bài posting.`);
      }
    } catch (err) {
      setMsg(err instanceof Error ? err.message : 'Không xoá được posting.');
    } finally {
      setDeletingAll(false);
    }
  };

  const filtered = filter === 'all' ? messages : messages.filter(m => matchesPostFilter(m.status, filter));

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
      <header style={{ display: 'flex', alignItems: 'center', gap: 16, flexWrap: 'wrap' }}>
        <div style={{ display: 'flex', gap: 8 }}>
          {FILTERS.map(f => (
            <button
              key={f.value}
              onClick={() => setFilter(f.value)}
              className={`filter-pill ${filter === f.value ? 'is-active' : ''}`}
            >
              {f.label}
            </button>
          ))}
        </div>
        <div style={{ flex: 1 }} />
        {isAdmin && (
          <button
            className="btn btn-ghost btn-sm"
            style={{ color: 'var(--danger)' }}
            disabled={deletingAll || messages.length === 0}
            onClick={() => void handleDeleteAll()}
            title="Xoá toàn bộ bài posting trong hàng đợi"
          >
            <Trash2 size={13} /> {deletingAll ? 'Đang xoá…' : 'Xoá tất cả'}
          </button>
        )}
        <button className="btn btn-ghost btn-sm" onClick={load}>
          <RefreshCw size={13} /> Làm mới
        </button>
      </header>

      {msg && <div className="banner banner-hot">{msg}</div>}

      {loading && (
        <div style={{ display: 'flex', justifyContent: 'center', padding: 40 }}>
          <RefreshCw size={24} className="spin" style={{ color: 'var(--text-mute)' }} />
        </div>
      )}

      {!loading && filtered.length === 0 && (
        <div className="empty" style={{ margin: 40 }}>
          <div className="eyebrow"><span className="dot" />OUTBOX</div>
          <h3>Chưa có bài post</h3>
          <p>Chưa có bài post trong hàng đợi thật.</p>
        </div>
      )}

      {!loading && (
        <div style={{ display: 'grid', gap: 16, gridTemplateColumns: 'repeat(auto-fill, minmax(320px, 1fr))' }}>
          {filtered.map(m => (
            <div key={m.id} className="card" style={{ display: 'flex', flexDirection: 'column' }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 12, flexWrap: 'wrap' }}>
                <span className="mono" style={{ background: 'var(--bg-elev)', color: 'var(--text)', padding: '2px 6px', borderRadius: 4, fontSize: 11 }}>
                  {m.target_name || 'Chưa có target'}
                </span>
                <span className="mono" style={{ color: 'var(--text-faint)', fontSize: 11 }}>{m.created_at?.slice(0, 10)}</span>
                <div style={{ flex: 1 }} />
                <span className={`tag ${m.status === 'sending' ? 'tag-warm' : m.status === 'sent' ? 'tag-ok' : m.status === 'approved' ? 'tag-cold' : 'tag-hot'}`}>
                  {postStatusLabel(m.status)}
                </span>
              </div>
              <p style={{ color: 'var(--text)', fontSize: 13.5, lineHeight: 1.6, whiteSpace: 'pre-wrap', flex: 1 }}>
                {m.content || '(Trống)'}
              </p>
              {m.context && <p style={{ color: 'var(--text-mute)', fontSize: 12, lineHeight: 1.5, marginTop: 12 }}>{m.context.slice(0, 240)}</p>}
              <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginTop: 16, paddingTop: 16, borderTop: '1px solid var(--line)' }}>
                <span className="mono" style={{ fontSize: 11, color: 'var(--text-faint)' }}>ACC #{m.account_id}</span>
                <button className="btn btn-ghost btn-sm" onClick={() => transition(m.id, 'delete')} style={{ color: 'var(--text-mute)' }}>
                  <Trash2 size={12} />
                </button>
                {m.target_url && (
                  <a
                    href={m.target_url}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="btn btn-ghost btn-sm"
                    style={{ marginLeft: 'auto', color: 'var(--accent)' }}
                  >
                    <ExternalLink size={12} />
                  </a>
                )}
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
