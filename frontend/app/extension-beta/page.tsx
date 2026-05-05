import type { Metadata } from 'next';
import Link from 'next/link';
import { ArrowLeft, Download, ExternalLink, Lock, Monitor, Puzzle, ShieldCheck, TestTube2, Workflow } from 'lucide-react';

import styles from './extension-beta.module.css';

export const dynamic = 'force-dynamic';

export const metadata: Metadata = {
  title: 'THG Extension Beta | THG AutoFlow',
  description:
    'Internal beta install lane for THG Chrome Extension while Chrome Web Store review is pending.',
};

const steps = [
  'Tải gói THG Chrome Extension beta từ link bảo mật bên dưới.',
  'Giải nén gói ra một thư mục riêng trên máy test.',
  'Mở chrome://extensions, bật Developer mode tạm thời, rồi bấm Load unpacked.',
  'Chọn thư mục extension vừa giải nén.',
  'Quay lại Browser dashboard, tạo mã kết nối và dán mã vào popup THG Extension.',
  'Mở facebook.com trong đúng Chrome đó để dashboard nhận stream và action log.',
];

const notes = [
  'Lane beta chỉ dành cho nội bộ trong lúc Google đang xét duyệt extension.',
  'Core pairing, stream, crawl, classify và action flow vẫn là production thật; chỉ khác bước cài đặt extension.',
  'Sau khi Chrome Web Store được duyệt, người dùng sẽ chuyển sang nút cài đặt 1 click và không cần lane beta nữa.',
];

export default function ExtensionBetaPage() {
  const packageURL = (process.env.CHROME_EXTENSION_BETA_PACKAGE_URL || '').trim();
  const extensionID = (process.env.CHROME_EXTENSION_ID || '').trim();

  return (
    <main className={styles.page}>
      <div className={styles.backdrop} aria-hidden="true" />

      <section className={styles.hero}>
        <div className={styles.heroNav}>
          <Link className={styles.backLink} href="/">
            <ArrowLeft size={14} />
            <span>Back to THG AutoFlow</span>
          </Link>

          <div className={styles.badge}>
            <TestTube2 size={14} />
            <span>Internal beta install lane</span>
          </div>
        </div>

        <div className={styles.heroCard}>
          <div className={styles.heroCopy}>
            <p className={styles.eyebrow}>Extension Beta</p>
            <h1>Test production before Chrome Web Store approval</h1>
            <p className={styles.lead}>
              This page is the temporary install lane for internal testers while
              Google is still reviewing the THG Chrome Extension. The production
              automation flow stays the same. Only the install step is manual.
            </p>
          </div>

          <div className={styles.summaryPanel}>
            <div className={styles.summaryRow}>
              <span>Use case</span>
              <strong>Internal production testing</strong>
            </div>
            <div className={styles.summaryRow}>
              <span>Review state</span>
              <strong>Waiting for Chrome Web Store approval</strong>
            </div>
            <div className={styles.summaryRow}>
              <span>Extension ID</span>
              <strong>{extensionID || 'Will appear after publisher setup'}</strong>
            </div>
            <div className={styles.summaryRow}>
              <span>Install mode</span>
              <strong>Manual beta only</strong>
            </div>
          </div>
        </div>
      </section>

      <section className={styles.grid}>
        <article className={styles.panel}>
          <div className={styles.panelHead}>
            <Download size={16} />
            <h2>Get the beta package</h2>
          </div>
          <p className={styles.bodyText}>
            Use this lane only for trusted internal testers. Once Google
            approves the extension, switch back to the normal Chrome Web Store
            install path in the Browser dashboard.
          </p>
          {packageURL ? (
            <a
              className={styles.primaryLink}
              href={packageURL}
              target="_blank"
              rel="noreferrer"
            >
              <Download size={14} />
              <span>Download THG Extension Beta</span>
            </a>
          ) : (
            <div className={styles.warnBox}>
              <Lock size={14} />
              <div>
                <strong>Beta package URL is not configured yet.</strong>
                <p>
                  Set <code>CHROME_EXTENSION_BETA_PACKAGE_URL</code> on the
                  production server so internal testers can download the beta
                  package from this page.
                </p>
              </div>
            </div>
          )}
        </article>

        <article className={styles.panel}>
          <div className={styles.panelHead}>
            <Workflow size={16} />
            <h2>Beta install steps</h2>
          </div>
          <ol className={styles.stepList}>
            {steps.map((step) => (
              <li key={step}>{step}</li>
            ))}
          </ol>
        </article>
      </section>

      <section className={styles.grid}>
        <article className={styles.panel}>
          <div className={styles.panelHead}>
            <ShieldCheck size={16} />
            <h2>What stays production-real</h2>
          </div>
          <ul className={styles.list}>
            <li>Workspace pairing and org-scoped connector tokens</li>
            <li>Browser dashboard streaming and account mapping</li>
            <li>Crawl, classify, leads, outbox and automation orchestration</li>
            <li>Approval, cooldown, dedup and audit guardrails</li>
          </ul>
        </article>

        <article className={styles.panel}>
          <div className={styles.panelHead}>
            <Monitor size={16} />
            <h2>Important notes</h2>
          </div>
          <ul className={styles.list}>
            {notes.map((note) => (
              <li key={note}>{note}</li>
            ))}
          </ul>
          <Link className={styles.secondaryLink} href="/privacy">
            <ExternalLink size={14} />
            <span>Review the privacy policy used for Chrome Web Store submission</span>
          </Link>
        </article>
      </section>

      <section className={styles.gridSingle}>
        <article className={styles.panel}>
          <div className={styles.panelHead}>
            <Puzzle size={16} />
            <h2>Switch-over after approval</h2>
          </div>
          <p className={styles.bodyText}>
            After Google approves the extension, disable the beta lane, keep the
            same connector APIs and Browser workflow, and let users install from
            Chrome Web Store. No automation rewrite is needed.
          </p>
        </article>
      </section>
    </main>
  );
}
