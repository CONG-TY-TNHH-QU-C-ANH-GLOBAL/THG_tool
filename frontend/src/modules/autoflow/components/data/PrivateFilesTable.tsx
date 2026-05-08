import { FileText, Trash2 } from 'lucide-react';
import type { FileRecord } from '../../types';
import { Row } from '../ui';
import { alpha, secondaryBtn, theme } from '../../constants/styles';

interface PrivateFilesTableProps {
  files: FileRecord[];
  onRemove: (id: number) => void;
}

export default function PrivateFilesTable({ files, onRemove }: PrivateFilesTableProps) {
  return (
    <div style={{ background: theme.surface, border: `1px solid ${theme.border}`, borderRadius: 12, overflow: 'hidden' }}>
      <div style={{ padding: '11px 16px', borderBottom: `1px solid ${theme.border}` }}>
        <Row style={{ gap: 8 }}>
          <FileText size={14} color={theme.primaryLight} />
          <p style={{ color: theme.text, fontWeight: 500, fontSize: 13 }}>Tệp dữ liệu riêng tư</p>
        </Row>
      </div>
      {files.length === 0 ? (
        <p style={{ color: theme.textMuted, fontSize: 13, padding: '24px 20px', textAlign: 'center' }}>Chưa có tệp nào trong workspace.</p>
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
                    <div style={{ width: 32, height: 32, background: alpha(theme.primary, 14), borderRadius: 8, display: 'flex', alignItems: 'center', justifyContent: 'center', flexShrink: 0 }}>
                      <FileText size={14} color={theme.primaryLight} />
                    </div>
                    <span style={{ color: theme.text, fontWeight: 500 }}>{f.name}</span>
                  </Row>
                </td>
                <td style={{ padding: '10px 14px', color: theme.textMuted }}>{f.size}</td>
                <td style={{ padding: '10px 14px', color: theme.textFaint }}>{f.date}</td>
                <td style={{ padding: '10px 14px' }}>
                  <button
                    onClick={() => onRemove(f.id)}
                    style={{ ...secondaryBtn({ padding: '6px 10px', fontSize: 11 }), color: theme.red, display: 'flex', alignItems: 'center', gap: 4 }}
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
  );
}
