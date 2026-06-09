'use client';

import { useCallback, useEffect, useState } from 'react';
import { CheckCircle2, Copy, ExternalLink, RefreshCw, X } from 'lucide-react';
import type { SystemInfo } from '../../services/systemService';
import { createLocalConnectorPairingCode } from '../../services/connectorsService';
import { getAccountReadiness } from '../../services/accountHealthService';

function storeUrl(s: SystemInfo | null): string {
  const direct = (s?.chrome_extension_store_url || '').trim();
  if (direct) return direct;
  const id = (s?.chrome_extension_id || '').trim();
  return id ? `https://chromewebstore.google.com/detail/thg-chrome-extension/${id}` : '';
}
function betaUrl(s: SystemInfo | null): string {
  return (s?.chrome_extension_beta_url || s?.chrome_extension_beta_package_url || '').trim();
}

interface Props {
  systemInfo: SystemInfo | null;
  onClose: () => void;
  onConnected: () => void;
}

const STEPS = ['Cài công cụ THG', 'Kết nối Chrome', 'Đăng nhập Facebook', 'Sẵn sàng'];

export function FacebookConnectionWizard({ systemInfo, onClose, onConnected }: Props) {
  const [step, setStep] = useState(1);
  const [code, setCode] = useState('');
  const [generating, setGenerating] = useState(false);
  const [checking, setChecking] = useState(false);
  const [baselineReady, setBaselineReady] = useState<number | null>(null);

  const readyCount = useCallback(async () => {
    const accounts = await getAccountReadiness().catch(() => []);
    return accounts.filter(a => a.capabilities.some(c => c.can)).length;
  }, []);

  useEffect(() => { void readyCount().then(setBaselineReady); }, [readyCount]);

  const genCode = async () => {
    setGenerating(true);
    try {
      const r = await createLocalConnectorPairingCode(`Chrome ${new Date().toLocaleDateString('vi-VN')}`);
      setCode(r.code);
    } finally {
      setGenerating(false);
    }
  };

  const checkReady = async () => {
    setChecking(true);
    try {
      const now = await readyCount();
      if (baselineReady === null || now > baselineReady) {
        setStep(4);
        onConnected();
      }
    } finally {
      setChecking(false);
    }
  };

  return (
    <div onClick={onClose} style={{ position: 'fixed', inset: 0, background: 'rgba(10,12,18,0.55)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 50, padding: 16 }}>
      <div onClick={e => e.stopPropagation()} className="card" style={{ width: 560, maxWidth: '100%', maxHeight: '90vh', overflow: 'auto', padding: 'var(--s-5)' }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', gap: 12 }}>
          <div>
            <h3 style={{ fontSize: 19, fontWeight: 700, margin: 0 }}>Kết nối Facebook bán hàng</h3>
            <p style={{ fontSize: 12.5, color: 'var(--text-mute)', marginTop: 4 }}>THG không lưu mật khẩu Facebook · chạy trên thiết bị &amp; IP của chính bạn.</p>
          </div>
          <button type="button" className="btn btn-ghost btn-sm" onClick={onClose}><X size={15} /></button>
        </div>

        <div style={{ display: 'flex', gap: 6, margin: '16px 0' }}>
          {STEPS.map((label, i) => (
            <div key={label} style={{ flex: 1, textAlign: 'center' }}>
              <div style={{ height: 4, borderRadius: 2, background: i + 1 <= step ? 'var(--ok)' : 'var(--line)' }} />
              <div style={{ fontSize: 10.5, color: i + 1 <= step ? 'var(--text)' : 'var(--text-faint)', marginTop: 5 }}>{label}</div>
            </div>
          ))}
        </div>

        {step === 1 && (
          <Section title="Cài công cụ THG trên Chrome" desc="Cài vào Chrome đang đăng nhập Facebook mà bạn muốn dùng.">
            <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
              {storeUrl(systemInfo) && <a className="btn btn-primary btn-sm" href={storeUrl(systemInfo)} target="_blank" rel="noreferrer"><ExternalLink size={13} /> Cài từ Chrome Web Store</a>}
              {betaUrl(systemInfo) && <a className="btn btn-ghost btn-sm" href={betaUrl(systemInfo)} target="_blank" rel="noreferrer"><ExternalLink size={13} /> Cài bản nội bộ</a>}
            </div>
            <Nav onNext={() => setStep(2)} nextLabel="Đã cài, tiếp tục" />
          </Section>
        )}

        {step === 2 && (
          <Section title="Kết nối Chrome này với workspace" desc="Tạo mã kết nối dùng một lần. Mã hết hạn sau 10 phút. Dán mã vào công cụ THG trên Chrome.">
            {!code ? (
              <button type="button" className="btn btn-primary btn-sm" onClick={() => void genCode()} disabled={generating}>
                {generating ? <RefreshCw size={13} className="spin" /> : null} Tạo mã kết nối
              </button>
            ) : (
              <div style={{ display: 'flex', alignItems: 'center', gap: 10, background: 'var(--bg-elev-2)', borderRadius: 8, padding: '12px 14px' }}>
                <span className="mono" style={{ fontSize: 22, fontWeight: 700, letterSpacing: 3 }}>{code}</span>
                <button type="button" className="btn btn-ghost btn-sm" onClick={() => void navigator.clipboard?.writeText(code)}><Copy size={13} /> Sao chép</button>
              </div>
            )}
            <Nav onBack={() => setStep(1)} onNext={() => setStep(3)} nextDisabled={!code} nextLabel="Đã dán mã, tiếp tục" />
          </Section>
        )}

        {step === 3 && (
          <Section title="Đăng nhập Facebook cần dùng" desc="Mở Facebook trong Chrome này. Sau khi đăng nhập, THG sẽ tự xác minh tài khoản và hiển thị trạng thái sẵn sàng.">
            <a className="btn btn-ghost btn-sm" href="https://www.facebook.com" target="_blank" rel="noreferrer"><ExternalLink size={13} /> Mở Facebook</a>
            <Nav onBack={() => setStep(2)} onNext={() => void checkReady()} nextDisabled={checking} nextLabel={checking ? 'Đang kiểm tra...' : 'Tôi đã đăng nhập, kiểm tra'} />
          </Section>
        )}

        {step === 4 && (
          <Section title="Tài khoản đã sẵn sàng tự động hoá" desc="Agent có thể tìm lead, bình luận, inbox và đăng bài theo quyền được cấp.">
            <div style={{ display: 'flex', alignItems: 'center', gap: 10, color: 'var(--ok)' }}>
              <CheckCircle2 size={28} />
              <span style={{ fontSize: 14, fontWeight: 600, color: 'var(--text)' }}>Kết nối thành công</span>
            </div>
            <div style={{ display: 'flex', justifyContent: 'flex-end', marginTop: 16 }}>
              <button type="button" className="btn btn-primary btn-sm" onClick={onClose}>Xong</button>
            </div>
          </Section>
        )}
      </div>
    </div>
  );
}

function Section({ title, desc, children }: { title: string; desc: string; children: React.ReactNode }) {
  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
      <div>
        <h4 style={{ fontSize: 15, fontWeight: 600, margin: 0 }}>{title}</h4>
        <p style={{ fontSize: 12.5, color: 'var(--text-mute)', marginTop: 4 }}>{desc}</p>
      </div>
      {children}
    </div>
  );
}

function Nav({ onBack, onNext, nextLabel, nextDisabled }: { onBack?: () => void; onNext: () => void; nextLabel: string; nextDisabled?: boolean }) {
  return (
    <div style={{ display: 'flex', justifyContent: 'space-between', marginTop: 16 }}>
      {onBack ? <button type="button" className="btn btn-ghost btn-sm" onClick={onBack}>Quay lại</button> : <span />}
      <button type="button" className="btn btn-primary btn-sm" onClick={onNext} disabled={nextDisabled}>{nextLabel}</button>
    </div>
  );
}
