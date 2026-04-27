import { useRef, useState } from 'react';
import { Row } from '../ui';
import { theme, cardStyle, primaryBtn } from '../../constants/styles';
import { useFiles } from '../../hooks/useFiles';
import { Upload, FileText, Trash2, Database } from 'lucide-react';

interface DataPrivateViewProps { orgId: string; }

export default function DataPrivateView({ orgId }: DataPrivateViewProps) {
  const { files, isUploading, upload, remove } = useFiles(orgId);
  const [isDragging, setIsDragging] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);

  const handleFiles = (fileList: FileList | null) => {
    if (!fileList) return;
    Array.from(fileList).forEach(f => upload(f));
  };

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3,1fr)', gap: 11 }}>
        {[
          { l: 'Tệp đã tải', v: files.length, c: '#fff' },
          { l: 'Tổng dung lượng', v: '3.7 MB', c: theme.primaryLight },
          { l: 'AI đã học', v: `${files.length} tệp`, c: '#4ade80' },
        ].map(s => (
          <div key={s.l} style={cardStyle()}>
            <p style={{ color: theme.textFaint, fontSize: 11, marginBottom: 4 }}>{s.l}</p>
            <p style={{ fontSize: 22, fontWeight: 700, color: s.c }}>{s.v}</p>
          </div>
        ))}
      </div>

      <div
        onDragOver={e => { e.preventDefault(); setIsDragging(true); }}
        onDragLeave={() => setIsDragging(false)}
        onDrop={e => { e.preventDefault(); setIsDragging(false); handleFiles(e.dataTransfer.files); }}
        onClick={() => inputRef.current?.click()}
        style={{
          border: `2px dashed ${isDragging ? theme.primary : theme.border}`,
          borderRadius: 12,
          padding: '32px 20px',
          textAlign: 'center',
          cursor: 'pointer',
          background: isDragging ? `${theme.primary}11` : theme.surface,
          transition: 'all 0.15s',
        }}
      >
        <input ref={inputRef} type="file" multiple style={{ display: 'none' }} onChange={e => handleFiles(e.target.files)} />
        <div style={{ width: 48, height: 48, background: `${theme.primary}22`, border: `1px solid ${theme.primary}44`, borderRadius: 12, display: 'flex', alignItems: 'center', justifyContent: 'center', margin: '0 auto 12px' }}>
          <Upload size={22} color={theme.primaryLight} />
        </div>
        <p style={{ color: theme.text, fontWeight: 500, fontSize: 14, marginBottom: 6 }}>
          {isUploading ? 'Đang tải lên...' : 'Kéo thả tệp hoặc click để tải lên'}
        </p>
        <p style={{ color: theme.textFaint, fontSize: 12 }}>PDF, DOCX, XLSX, TXT — tối đa 50MB/tệp</p>
        {!isUploading && (
          <button style={{ ...primaryBtn({ padding: '8px 20px', fontSize: 13 }), marginTop: 14, display: 'inline-flex', alignItems: 'center', gap: 6 }}>
            <Upload size={13} />Chọn tệp
          </button>
        )}
      </div>

      <div style={{ background: theme.surface, border: `1px solid ${theme.border}`, borderRadius: 12, overflow: 'hidden' }}>
        <div style={{ padding: '11px 16px', borderBottom: `1px solid ${theme.border}` }}>
          <Row style={{ gap: 8 }}>
            <Database size={14} color={theme.primaryLight} />
            <p style={{ color: theme.text, fontWeight: 500, fontSize: 13 }}>Tệp dữ liệu riêng tư</p>
          </Row>
        </div>
        {files.length === 0 ? (
          <p style={{ color: theme.textMuted, fontSize: 13, padding: '24px 20px', textAlign: 'center' }}>Chưa có tệp nào. Tải lên tệp sản phẩm để AI học.</p>
        ) : (
          <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 12 }}>
            <thead>
              <tr style={{ borderBottom: `1px solid ${theme.border}` }}>
                {['Tên tệp', 'Kích thước', 'Ngày tải', ''].map(h => (
                  <th key={h} style={{ padding: '9px 14px', textAlign: 'left', color: theme.textFaint, fontWeight: 500, fontSize: 11 }}>{h}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {files.map(f => (
                <tr key={f.id} style={{ borderBottom: `1px solid ${theme.borderAlt}` }}>
                  <td style={{ padding: '10px 14px' }}>
                    <Row style={{ gap: 8 }}>
                      <div style={{ width: 32, height: 32, background: `${theme.primary}22`, borderRadius: 8, display: 'flex', alignItems: 'center', justifyContent: 'center', flexShrink: 0 }}>
                        <FileText size={14} color={theme.primaryLight} />
                      </div>
                      <span style={{ color: theme.text, fontWeight: 500 }}>{f.name}</span>
                    </Row>
                  </td>
                  <td style={{ padding: '10px 14px', color: theme.textMuted }}>{f.size}</td>
                  <td style={{ padding: '10px 14px', color: theme.textFaint }}>{f.date}</td>
                  <td style={{ padding: '10px 14px' }}>
                    <button
                      onClick={() => remove(f.id)}
                      style={{ background: 'none', border: 'none', color: theme.textFaint, cursor: 'pointer', display: 'flex', alignItems: 'center', gap: 4, fontSize: 11 }}
                    >
                      <Trash2 size={13} />Xóa
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </div>
  );
}
