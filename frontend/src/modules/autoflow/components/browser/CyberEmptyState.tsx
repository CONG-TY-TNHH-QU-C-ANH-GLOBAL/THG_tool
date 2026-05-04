import { ArrowRight, Cpu, Plus, RefreshCw } from 'lucide-react';
import { theme } from '../../constants/styles';
export function CyberEmptyState({ onCreate, loading }: { onCreate: () => void; loading: boolean }) {
  return (
    <div style={{
      position: 'relative',
      overflow: 'hidden',
      border: '1px solid #22d3ee55',
      background: 'linear-gradient(135deg, #07111f 0%, #111827 46%, #111520 100%)',
      borderRadius: 10,
      padding: 26,
      minHeight: 230,
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'center',
      boxShadow: '0 0 0 1px #0e749044 inset, 0 18px 60px #00000040',
    }}>
      <div style={{ position: 'absolute', inset: 0, backgroundImage: 'linear-gradient(#22d3ee12 1px, transparent 1px), linear-gradient(90deg, #22d3ee10 1px, transparent 1px)', backgroundSize: '28px 28px', opacity: 0.55 }} />
      <div style={{ position: 'absolute', top: 0, left: 0, right: 0, height: 1, background: 'linear-gradient(90deg, transparent, #67e8f9, transparent)' }} />
      <div style={{ position: 'relative', textAlign: 'center', maxWidth: 560 }}>
        <div style={{ width: 46, height: 46, margin: '0 auto 14px', borderRadius: 12, background: '#0e749033', border: '1px solid #22d3ee66', display: 'grid', placeItems: 'center', boxShadow: '0 0 24px #06b6d455' }}>
          <Cpu size={22} color="#67e8f9" />
        </div>
        <p style={{ color: '#67e8f9', fontSize: 11, fontWeight: 800, marginBottom: 8 }}>CYBERTECH SIGNAL</p>
        <h3 style={{ color: theme.textWhite, fontSize: 18, fontWeight: 800, marginBottom: 8 }}>Workspace chÆ°a cÃ³ tÃ i khoáº£n Facebook</h3>
        <p style={{ color: theme.textMuted, fontSize: 13, lineHeight: 1.6, marginBottom: 18 }}>
          Khá»Ÿi táº¡o phiÃªn Facebook Ä‘áº§u tiÃªn Ä‘á»ƒ agent cÃ³ browser riÃªng, session riÃªng vÃ  dá»¯ liá»‡u automation Ä‘Æ°á»£c gáº¯n Ä‘Ãºng workspace.
        </p>
        <button
          onClick={onCreate}
          disabled={loading}
          style={{ display: 'inline-flex', alignItems: 'center', gap: 8, padding: '10px 18px', background: '#0891b2', border: '1px solid #67e8f966', borderRadius: 8, color: '#fff', fontSize: 13, fontWeight: 700, cursor: loading ? 'wait' : 'pointer', opacity: loading ? 0.65 : 1, boxShadow: '0 10px 30px #0891b244' }}
        >
          {loading ? <RefreshCw size={15} className="spin" /> : <Plus size={15} />}
          {loading ? 'Äang khá»Ÿi táº¡o' : 'Táº¡o Facebook workspace'}
          {!loading && <ArrowRight size={15} />}
        </button>
      </div>
    </div>
  );
}

