import { Check, Puzzle, KeyRound, Globe, ShieldCheck, MapPin, Clock, Lock } from 'lucide-react';

/**
 * ConnectorSetupStepper — PR-M4 connector onboarding.
 *
 * A trust-forward progress stepper that auto-advances so the member SEES they
 * succeeded: step 1 (install) → step 2 (pair device) → step 3 (Facebook login),
 * then a clear "connected & secure" success state. Plus a privacy strip making
 * the security posture explicit (no password stored, own device/IP, one-time
 * code). Restrained on purpose — it reuses the app's tokens (no new fonts), the
 * one accent is the security/done green.
 */
interface StepDef {
  label: string;
  hint: string;
  icon: React.ReactNode;
}

const STEPS: StepDef[] = [
  { label: 'Cài extension', hint: 'Chrome cá nhân', icon: <Puzzle size={15} /> },
  { label: 'Ghép nối thiết bị', hint: 'Mã 1 lần', icon: <KeyRound size={15} /> },
  { label: 'Đăng nhập Facebook', hint: 'Tab đã login', icon: <Globe size={15} /> },
];

interface ConnectorSetupStepperProps {
  /** 1..3 = the active (in-progress) step; 4 = all done / connected. */
  currentStep: number;
  /** True once an online connector is logged into Facebook (terminal success). */
  facebookReady: boolean;
  /** Live FB identity to show in the success banner, when known. */
  onlineIdentity?: string;
}

export function ConnectorSetupStepper({ currentStep, facebookReady, onlineIdentity }: ConnectorSetupStepperProps) {
  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--s-4)' }}>
      {/* ── Progress rail ─────────────────────────────────────────────── */}
      <div style={{ display: 'flex', alignItems: 'flex-start', gap: 0 }}>
        {STEPS.map((step, i) => {
          const n = i + 1;
          const done = facebookReady || currentStep > n;
          const active = !facebookReady && currentStep === n;
          const accent = done ? 'var(--ok)' : active ? 'var(--accent)' : 'var(--text-faint)';
          return (
            <div key={step.label} style={{ flex: 1, display: 'flex', flexDirection: 'column', alignItems: 'center', position: 'relative' }}>
              {/* connector line to next step */}
              {i < STEPS.length - 1 && (
                <span
                  style={{
                    position: 'absolute', top: 17, left: '50%', width: '100%', height: 2,
                    background: facebookReady || currentStep > n ? 'var(--ok)' : 'var(--line)',
                    transition: 'background 0.4s ease',
                  }}
                />
              )}
              <div
                style={{
                  width: 36, height: 36, borderRadius: '50%', display: 'grid', placeItems: 'center',
                  zIndex: 1, flexShrink: 0, color: done ? '#fff' : accent,
                  background: done ? 'var(--ok)' : active ? 'var(--accent-soft)' : 'var(--bg-elev)',
                  border: `2px solid ${accent}`,
                  boxShadow: active ? '0 0 0 4px var(--accent-soft)' : 'none',
                  transition: 'all 0.3s ease',
                }}
              >
                {done ? <Check size={17} strokeWidth={3} /> : step.icon}
              </div>
              <div style={{ marginTop: 8, textAlign: 'center' }}>
                <div style={{ fontSize: 12, fontWeight: 600, color: done || active ? 'var(--text)' : 'var(--text-mute)' }}>
                  {step.label}
                </div>
                <div style={{ fontSize: 10.5, color: 'var(--text-faint)', marginTop: 1 }}>{step.hint}</div>
              </div>
            </div>
          );
        })}
      </div>

      {/* ── Success state ─────────────────────────────────────────────── */}
      {facebookReady && (
        <div
          style={{
            display: 'flex', alignItems: 'center', gap: 10,
            padding: '12px 14px', borderRadius: 'var(--radius-md)',
            background: 'var(--ok-soft, rgba(34,197,94,0.10))',
            border: '1px solid var(--ok)',
          }}
        >
          <ShieldCheck size={18} style={{ color: 'var(--ok)', flexShrink: 0 }} />
          <div style={{ lineHeight: 1.4 }}>
            <div style={{ fontSize: 13, fontWeight: 700, color: 'var(--text)' }}>
              Đã kết nối an toàn{onlineIdentity ? ` · ${onlineIdentity}` : ''}
            </div>
            <div style={{ fontSize: 11.5, color: 'var(--text-mute)' }}>
              Phiên Facebook chạy ngay trên thiết bị của bạn — sẵn sàng tự động hóa.
            </div>
          </div>
        </div>
      )}

      {/* ── Privacy / security strip ──────────────────────────────────── */}
      <div
        style={{
          display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(180px, 1fr))', gap: 'var(--s-2)',
          padding: 'var(--s-3)', borderRadius: 'var(--radius-md)',
          background: 'var(--bg-elev)', border: '1px solid var(--line)',
        }}
      >
        {[
          { icon: <Lock size={13} />, text: 'THG không lưu mật khẩu Facebook' },
          { icon: <MapPin size={13} />, text: 'Chạy trên thiết bị & IP của chính bạn' },
          { icon: <Clock size={13} />, text: 'Mã ghép nối dùng 1 lần, hết hạn 10 phút' },
        ].map((item, i) => (
          <div key={i} style={{ display: 'flex', alignItems: 'center', gap: 7, fontSize: 11.5, color: 'var(--text-mute)' }}>
            <span style={{ color: 'var(--accent)', flexShrink: 0, display: 'inline-flex' }}>{item.icon}</span>
            {item.text}
          </div>
        ))}
      </div>
    </div>
  );
}
