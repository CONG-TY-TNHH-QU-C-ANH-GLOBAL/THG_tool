import { Database } from 'lucide-react';
import { Row } from '../ui';
import { cardStyle, theme } from '../../constants/styles';

interface ContextSummaryPanelProps {
  privateFilesSummary: string;
  dataSourcesSummary: string;
}

export default function ContextSummaryPanel({ privateFilesSummary, dataSourcesSummary }: Readonly<ContextSummaryPanelProps>) {
  if (!privateFilesSummary && !dataSourcesSummary) return null;

  return (
    <div style={cardStyle()}>
      <Row style={{ gap: 8, marginBottom: 10 }}>
        <Database size={14} color={theme.primaryLight} />
        <p style={{ color: theme.text, fontWeight: 500, fontSize: 13 }}>Context đã đưa vào AI</p>
      </Row>
      {privateFilesSummary && (
        <>
          <p style={{ color: theme.textFaint, fontSize: 11, marginBottom: 6 }}>Uploaded files</p>
          <pre style={{ whiteSpace: 'pre-wrap', margin: 0, color: theme.textMuted, fontSize: 12, lineHeight: 1.55, maxHeight: 130, overflow: 'auto' }}>{privateFilesSummary}</pre>
        </>
      )}
      {dataSourcesSummary && (
        <>
          <p style={{ color: theme.textFaint, fontSize: 11, margin: privateFilesSummary ? '14px 0 6px' : '0 0 6px' }}>Connected sources</p>
          <pre style={{ whiteSpace: 'pre-wrap', margin: 0, color: theme.textMuted, fontSize: 12, lineHeight: 1.55, maxHeight: 160, overflow: 'auto' }}>{dataSourcesSummary}</pre>
        </>
      )}
    </div>
  );
}
