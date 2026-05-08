import { useRef, useState } from 'react';
import { Upload } from 'lucide-react';
import { alpha, primaryBtn, theme } from '../../constants/styles';

interface FileUploadPanelProps {
  isUploading: boolean;
  onUpload: (files: FileList | null) => void;
}

export default function FileUploadPanel({ isUploading, onUpload }: FileUploadPanelProps) {
  const [isDragging, setIsDragging] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);

  return (
    <div
      onDragOver={e => { e.preventDefault(); setIsDragging(true); }}
      onDragLeave={() => setIsDragging(false)}
      onDrop={e => { e.preventDefault(); setIsDragging(false); onUpload(e.dataTransfer.files); }}
      onClick={() => inputRef.current?.click()}
      style={{
        border: `2px dashed ${isDragging ? theme.primary : theme.border}`,
        borderRadius: 12,
        padding: '32px 20px',
        textAlign: 'center',
        cursor: 'pointer',
        background: isDragging ? alpha(theme.primary, 8) : theme.surface,
        transition: 'all 0.15s',
      }}
    >
      <input ref={inputRef} type="file" multiple style={{ display: 'none' }} onChange={e => onUpload(e.target.files)} />
      <div style={{ width: 48, height: 48, background: alpha(theme.primary, 14), border: `1px solid ${alpha(theme.primary, 28)}`, borderRadius: 12, display: 'flex', alignItems: 'center', justifyContent: 'center', margin: '0 auto 12px' }}>
        <Upload size={22} color={theme.primaryLight} />
      </div>
      <p style={{ color: theme.text, fontWeight: 500, fontSize: 14, marginBottom: 6 }}>
        {isUploading ? 'Đang tải lên...' : 'Kéo thả tệp hoặc click để tải lên'}
      </p>
      <p style={{ color: theme.textFaint, fontSize: 12 }}>PDF, DOCX, XLSX, TXT, CSV - tối đa 50MB/tệp</p>
      {!isUploading && (
        <button style={{ ...primaryBtn({ padding: '8px 20px', fontSize: 13 }), marginTop: 14, display: 'inline-flex', alignItems: 'center', gap: 6 }}>
          <Upload size={13} />Chọn tệp
        </button>
      )}
    </div>
  );
}
