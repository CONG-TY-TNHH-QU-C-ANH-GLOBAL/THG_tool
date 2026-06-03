import { useEffect, useState } from 'react';
import { Row, Badge } from './ui';
import { theme, alpha, cardStyle, primaryBtn } from '../constants/styles';
import { listAccounts, type MemberAccount } from '../services/accountsService';
import { getDefaultAccountId, setDefaultAccountId } from '../services/executionContextService';
import { CheckCircle2, Circle, Info, UserCog } from 'lucide-react';

/**
 * Default Account picker — UI for the deterministic ExecutionContext (PR4).
 *
 * Per-user, per-org. Resolution order when an action runs:
 *   explicit account → THIS default → (exactly 1 owned → use it) → error.
 *
 * So the picker only MATTERS once a member owns ≥2 accounts: until they choose,
 * outbound actions fail with `execution_context_required`. With 0 or 1 account
 * there is nothing to disambiguate, and we say so instead of forcing a choice.
 * No auto-magic default is ever written server-side — the member picks explicitly.
 */
export default function DefaultAccountSettings() {
  const [accounts, setAccounts] = useState<MemberAccount[]>([]);
  const [defaultId, setDefaultId] = useState<number>(0);
  const [selected, setSelected] = useState<number>(0);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [msg, setMsg] = useState('');
  const [err, setErr] = useState('');

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const [accs, def] = await Promise.all([listAccounts(), getDefaultAccountId()]);
        if (cancelled) return;
        setAccounts(accs);
        setDefaultId(def);
        setSelected(def);
      } catch (e) {
        if (!cancelled) setErr(e instanceof Error ? e.message : 'Không tải được danh sách tài khoản.');
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => { cancelled = true; };
  }, []);

  const save = async () => {
    setSaving(true);
    setMsg('');
    setErr('');
    try {
      const saved = await setDefaultAccountId(selected);
      setDefaultId(saved);
      setMsg('Đã lưu tài khoản mặc định.');
    } catch (e) {
      setErr(e instanceof Error ? e.message : 'Không lưu được tài khoản mặc định.');
    } finally {
      setSaving(false);
    }
  };

  if (loading) {
    return (
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', minHeight: 160 }}>
        <div className="skeleton" style={{ width: 220, height: 14 }} />
      </div>
    );
  }

  const dirty = selected !== defaultId;
  const onlyOne = accounts.length === 1;
  const none = accounts.length === 0;

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
      <div style={cardStyle()}>
        <Row style={{ gap: 10, marginBottom: 8 }}>
          <UserCog size={16} color={theme.primaryLight} />
          <p style={{ color: theme.text, fontWeight: 600, fontSize: 13, flex: 1 }}>Tài khoản mặc định khi thực thi</p>
        </Row>
        <p style={{ color: theme.textFaint, fontSize: 12, lineHeight: 1.5, marginBottom: 4 }}>
          Khi bạn ra lệnh comment/nhắn tin mà không chỉ định tài khoản, hệ thống dùng tài khoản mặc định này.
          Mặc định chỉ cần thiết khi bạn sở hữu từ 2 tài khoản trở lên.
        </p>

        {/* Contextual guidance based on how many accounts the member owns. */}
        {none && (
          <Row style={{ gap: 8, marginTop: 12, padding: '10px 12px', borderRadius: 9, background: alpha(theme.primary, 8), border: `1px solid ${theme.border}` }}>
            <Info size={14} color={theme.textMuted} />
            <span style={{ color: theme.textMuted, fontSize: 12 }}>
              Bạn chưa có tài khoản nào. Ghép một tài khoản Facebook ở mục Trình duyệt trước.
            </span>
          </Row>
        )}
        {onlyOne && (
          <Row style={{ gap: 8, marginTop: 12, padding: '10px 12px', borderRadius: 9, background: alpha(theme.green, 10), border: `1px solid ${alpha(theme.green, 30)}` }}>
            <Info size={14} color={theme.green} />
            <span style={{ color: theme.textMuted, fontSize: 12 }}>
              Bạn chỉ có 1 tài khoản — hệ thống tự dùng tài khoản đó, không cần chọn mặc định.
            </span>
          </Row>
        )}
      </div>

      {!none && (
        <div style={cardStyle()}>
          <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
            {accounts.map(acc => {
              const isSel = selected === acc.id;
              const isDefault = defaultId === acc.id;
              return (
                <button
                  key={acc.id}
                  onClick={() => !onlyOne && setSelected(acc.id)}
                  disabled={onlyOne}
                  style={{
                    display: 'flex',
                    alignItems: 'center',
                    gap: 12,
                    textAlign: 'left',
                    padding: '12px 14px',
                    borderRadius: 10,
                    cursor: onlyOne ? 'default' : 'pointer',
                    background: isSel ? alpha(theme.primary, 10) : theme.surfaceAlt,
                    border: `1px solid ${isSel ? alpha(theme.primary, 40) : theme.border}`,
                  }}
                >
                  {isSel ? <CheckCircle2 size={18} color={theme.primaryLight} /> : <Circle size={18} color={theme.textFaint} />}
                  <div style={{ flex: 1, minWidth: 0 }}>
                    <Row style={{ gap: 8 }}>
                      <span style={{ color: theme.text, fontWeight: 600, fontSize: 13 }}>
                        {acc.fbDisplayName || acc.name || `Tài khoản #${acc.id}`}
                      </span>
                      {isDefault && <Badge label="Mặc định hiện tại" />}
                    </Row>
                    <p style={{ color: theme.textFaint, fontSize: 11, marginTop: 2, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                      {acc.fbUsername ? `@${acc.fbUsername}` : acc.email || acc.fbProfileUrl || `ID ${acc.id}`}
                    </p>
                  </div>
                  <span style={{ fontSize: 11, color: acc.browserLoggedIn ? theme.green : theme.textFaint, fontWeight: 500 }}>
                    {acc.browserLoggedIn ? '● Đã đăng nhập' : '○ Chưa đăng nhập'}
                  </span>
                </button>
              );
            })}
          </div>

          <Row style={{ gap: 10, justifyContent: 'flex-end', marginTop: 16 }}>
            {msg && <span style={{ color: theme.green, fontSize: 12 }}>{msg}</span>}
            {err && <span style={{ color: theme.red, fontSize: 12 }}>{err}</span>}
            <button
              onClick={save}
              disabled={onlyOne || !dirty || saving}
              style={primaryBtn({ padding: '9px 22px', opacity: !onlyOne && dirty && !saving ? 1 : 0.5 })}
            >
              {saving ? 'Đang lưu...' : 'Lưu mặc định'}
            </button>
          </Row>
        </div>
      )}

      {err && none && <span style={{ color: theme.red, fontSize: 12 }}>{err}</span>}
    </div>
  );
}
