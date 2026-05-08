import { ExternalLink, ShieldCheck } from 'lucide-react';
import BetaPackageInfo from './BetaPackageInfo';
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
          Đây là gói beta do CI phát hành, không dùng thư mục extension local trong repo.
          Khi official được duyệt, hãy gỡ beta và cài lại từ Chrome Web Store.
        </p>

        <BetaPackageInfo />

        <div className={styles.actions}>
          <a className={styles.secondary} href="/autoflow">
            <ExternalLink size={17} />
            Mở dashboard
          </a>
        </div>

        <ol className={styles.steps}>
          <li>Tải file zip mới, giải nén ra một thư mục mới.</li>
          <li>Mở <code>chrome://extensions</code>, bật Developer mode.</li>
          <li>Remove bản beta cũ rồi cài lại từ thư mục beta mới vừa giải nén.</li>
          <li>Pair lại extension trong Browser dashboard.</li>
        </ol>

        <p className={styles.note}>
          ID beta có thể khác ID official; trong production ưu tiên bản Chrome Web Store.
        </p>
      </section>
    </main>
  );
}
