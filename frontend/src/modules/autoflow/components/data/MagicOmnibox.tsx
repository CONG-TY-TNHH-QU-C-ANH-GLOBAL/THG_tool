'use client';

import { useState } from 'react';
import { Sparkles, Globe, AlertCircle, Loader2 } from 'lucide-react';
import {
  inferBusinessContext,
  type InferBusinessContextResult,
} from '../../services/settingsService';

interface MagicOmniboxProps {
  onInferred: (result: InferBusinessContextResult) => void;
  busy?: boolean;
}

const URL_HINT = /^(https?:\/\/)/i;

export default function MagicOmnibox({ onInferred, busy }: Readonly<MagicOmniboxProps>) {
  const [input, setInput] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const looksLikeURL = URL_HINT.test(input.trim());

  const run = async () => {
    const text = input.trim();
    if (!text || submitting) return;
    setSubmitting(true);
    setError(null);
    try {
      const payload = looksLikeURL ? { source_url: text } : { note: text };
      const result = await inferBusinessContext(payload);
      onInferred(result);
      setInput('');
    } catch (err) {
      setError(err instanceof Error ? err.message : 'AI không phản hồi.');
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="cyber-oracle-card" style={{
      position: 'relative',
      overflow: 'hidden',
      padding: 24,
      borderRadius: 16,
      background: 'linear-gradient(135deg, rgba(79,70,229,0.06), rgba(6,182,212,0.06))',
      border: '1px solid rgba(79,70,229,0.18)',
      boxShadow: '0 8px 24px -8px rgba(79,70,229,0.18), 0 2px 8px rgba(6,182,212,0.08)',
    }}>
      {(submitting || busy) && <ScanningLoader />}

      <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 6 }}>
        <Sparkles size={16} style={{ color: '#4F46E5' }} />
        <span style={{ fontSize: 11, fontWeight: 700, letterSpacing: '0.1em', color: '#4F46E5' }}>
          MAGIC OMNIBOX
        </span>
      </div>

      <h2 style={{ margin: 0, fontSize: 22, fontWeight: 700, color: 'var(--text)', letterSpacing: '-0.01em' }}>
        Để AI tự suy luận hồ sơ doanh nghiệp
      </h2>
      <p style={{ margin: '6px 0 16px', fontSize: 13.5, color: 'var(--text-mute)', lineHeight: 1.5 }}>
        Dán URL website / catalog hoặc viết một câu mô tả — hệ thống sẽ cào, đọc và điền sẵn 13 trường định vị cùng điểm tin cậy. Bạn chỉ cần soát lại.
      </p>

      <div style={{ display: 'flex', gap: 10, alignItems: 'stretch', flexWrap: 'wrap' }}>
        <div style={{
          flex: 1,
          minWidth: 280,
          display: 'flex',
          alignItems: 'center',
          gap: 10,
          padding: '12px 14px',
          background: 'var(--bg-elev)',
          border: '1px solid var(--line)',
          borderRadius: 12,
          boxShadow: 'inset 0 1px 2px rgba(0,0,0,0.04)',
        }}>
          {looksLikeURL ? <Globe size={16} style={{ color: '#06B6D4', flexShrink: 0 }} /> : <Sparkles size={16} style={{ color: '#4F46E5', flexShrink: 0 }} />}
          <input
            type="text"
            value={input}
            onChange={(e) => setInput(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter' && !e.shiftKey) {
                e.preventDefault();
                void run();
              }
            }}
            placeholder="https://thgfulfill.com/catalog   hoặc   THG Fulfill — dịch vụ POD fulfill toàn cầu cho seller Việt"
            disabled={submitting}
            style={{
              flex: 1,
              border: 0,
              outline: 'none',
              background: 'transparent',
              color: 'var(--text)',
              fontSize: 14,
              minWidth: 0,
            }}
          />
        </div>
        <button
          type="button"
          onClick={() => void run()}
          disabled={!input.trim() || submitting}
          style={{
            padding: '0 20px',
            borderRadius: 12,
            border: 0,
            cursor: input.trim() && !submitting ? 'pointer' : 'not-allowed',
            background: input.trim() && !submitting
              ? 'linear-gradient(135deg, #4F46E5, #06B6D4)'
              : 'var(--bg-elev-2)',
            color: input.trim() && !submitting ? '#FFFFFF' : 'var(--text-faint)',
            fontWeight: 600,
            fontSize: 13,
            display: 'flex',
            alignItems: 'center',
            gap: 8,
            boxShadow: input.trim() && !submitting ? '0 4px 12px rgba(79,70,229,0.3)' : 'none',
            transition: 'all 0.15s',
          }}
        >
          {submitting ? <Loader2 size={14} className="spin" /> : <Sparkles size={14} />}
          {submitting ? 'Đang phân tích...' : 'Phân tích bằng AI'}
        </button>
      </div>

      {error && (
        <div style={{
          display: 'flex',
          gap: 8,
          alignItems: 'flex-start',
          marginTop: 12,
          padding: '10px 12px',
          fontSize: 12.5,
          color: 'var(--hot)',
          background: 'rgba(220,40,40,0.08)',
          borderRadius: 10,
        }}>
          <AlertCircle size={14} style={{ flexShrink: 0, marginTop: 1 }} />
          <span>{error}</span>
        </div>
      )}

      <style jsx>{`
        @keyframes scanline {
          0%   { transform: translateX(-100%); }
          100% { transform: translateX(100%); }
        }
        @keyframes spin {
          to { transform: rotate(360deg); }
        }
        :global(.spin) { animation: spin 0.8s linear infinite; }
      `}</style>
    </div>
  );
}

function ScanningLoader() {
  return (
    <div style={{
      position: 'absolute',
      inset: 0,
      pointerEvents: 'none',
      overflow: 'hidden',
      borderRadius: 16,
    }}>
      <div style={{
        position: 'absolute',
        top: 0,
        bottom: 0,
        width: '60%',
        background: 'linear-gradient(90deg, transparent, rgba(6,182,212,0.18), rgba(79,70,229,0.18), transparent)',
        animation: 'scanline 1.6s ease-in-out infinite',
      }} />
    </div>
  );
}
