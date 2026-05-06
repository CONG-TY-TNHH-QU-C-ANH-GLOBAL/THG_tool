import { Download, ExternalLink, ShieldCheck } from 'lucide-react';
import styles from './extension-beta.module.css';

export const metadata = {
  title: 'THG Chrome Extension Beta',
};

export default function ExtensionBetaPage() {
  return (
    <main className={styles.page}>
      <section className={styles.panel}>
        <div className={styles.kicker}>
          <ShieldCheck size={16} />
          Internal beta fallback
        </div>
        <h1>THG Chrome Extension Beta</h1>
        <p className={styles.copy}>
          Dùng bản beta này trong lúc Chrome Web Store đang xét duyệt bản official mới.
          Khi official được duyệt, hãy gỡ beta và cài lại từ Chrome Web Store.
        </p>

        <div className={styles.actions}>
          <a className={styles.primary} href="/api/system/extension-beta-package">
            <Download size={18} />
            Tải beta package
          </a>
          <a className={styles.secondary} href="/autoflow">
            <ExternalLink size={17} />
            Mở dashboard
          </a>
        </div>

        <ol className={styles.steps}>
          <li>Tải file zip rồi giải nén ra một thư mục riêng.</li>
          <li>Mở <code>chrome://extensions</code>, bật Developer mode.</li>
          <li>Chọn Load unpacked và trỏ vào thư mục vừa giải nén.</li>
          <li>Pair lại extension trong Browser dashboard.</li>
        </ol>

        <p className={styles.note}>
          ID beta có thể khác ID official vì Load unpacked không dùng chữ ký Chrome Web Store.
        </p>
      </section>
    </main>
  );
}
