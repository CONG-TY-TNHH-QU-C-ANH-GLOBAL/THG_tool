import type { Metadata } from 'next';
import Link from 'next/link';
import {
  ArrowLeft,
  BadgeCheck,
  Bot,
  Cookie,
  Eye,
  Lock,
  Mail,
  Server,
  ShieldCheck,
  Workflow,
} from 'lucide-react';

import styles from './privacy.module.css';

export const metadata: Metadata = {
  title: 'Privacy Policy | THG AutoFlow',
  description:
    'Privacy Policy for THG Chrome Extension and THG AutoFlow workspace services.',
};

const collectedData = [
  {
    icon: Cookie,
    title: 'Facebook account identifier',
    body:
      'We read the Facebook c_user cookie only to identify which signed-in Facebook account is connected to a THG workspace account slot.',
  },
  {
    icon: Eye,
    title: 'Visible Facebook tab state',
    body:
      'When a user connects a Facebook tab, THG receives the current tab URL, visible page metadata, screenshots or stream frames, and action status needed for the Browser dashboard.',
  },
  {
    icon: Workflow,
    title: 'Workspace action logs',
    body:
      'We store command history, automation results, crawl outputs, and approval events so organizations can audit what the system did and why.',
  },
  {
    icon: Server,
    title: 'Extension pairing state',
    body:
      'The extension stores a pairing token, server URL, and minimal local state required to reconnect the Chrome profile to the correct THG workspace.',
  },
];

const notCollected = [
  'Facebook passwords or login secrets',
  'Browsing activity outside user-authorized Facebook tabs and THG domains',
  'Payment card data through the extension',
  'Background collection from unrelated websites',
];

const usageRules = [
  'Connect a user-approved Facebook tab to a THG workspace dashboard',
  'Display browser stream, identity state, and automation status to the workspace',
  'Execute user-authorized crawl, comment, inbox, and posting workflows',
  'Enforce account mapping, audit logging, and organization security controls',
];

const userControls = [
  'Users can disconnect the Chrome Extension from a workspace at any time.',
  'Organizations can revoke device pairing, browser sessions, and workspace access from the dashboard.',
  'If Facebook shows checkpoint, verification, or CAPTCHA, THG requires direct human action in Facebook.',
  'THG does not sell personal data and does not use Facebook credentials for advertising resale.',
];

export default function PrivacyPage() {
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
            <ShieldCheck size={14} />
            <span>Production privacy disclosure</span>
          </div>
        </div>

        <div className={styles.heroCard}>
          <div className={styles.heroCopy}>
            <p className={styles.eyebrow}>Privacy Policy</p>
            <h1>THG Chrome Extension and THG AutoFlow</h1>
            <p className={styles.lead}>
              This policy explains what THG collects, what THG does not collect,
              and how browser-connected Facebook workflows are handled inside the
              THG workspace platform.
            </p>
          </div>

          <div className={styles.summaryPanel}>
            <div className={styles.summaryRow}>
              <span>Publisher</span>
              <strong>THG Fulfill</strong>
            </div>
            <div className={styles.summaryRow}>
              <span>Contact</span>
              <a href="mailto:thgfulfill.com@gmail.com">thgfulfill.com@gmail.com</a>
            </div>
            <div className={styles.summaryRow}>
              <span>Effective date</span>
              <strong>May 5, 2026</strong>
            </div>
            <div className={styles.summaryRow}>
              <span>Applies to</span>
              <strong>THG AutoFlow and THG Chrome Extension</strong>
            </div>
          </div>
        </div>
      </section>

      <section className={styles.grid}>
        <article className={styles.panel}>
          <div className={styles.panelHead}>
            <BadgeCheck size={16} />
            <h2>What THG collects</h2>
          </div>
          <div className={styles.cardGrid}>
            {collectedData.map(({ icon: Icon, title, body }) => (
              <div key={title} className={styles.dataCard}>
                <div className={styles.iconWrap}>
                  <Icon size={16} />
                </div>
                <h3>{title}</h3>
                <p>{body}</p>
              </div>
            ))}
          </div>
        </article>

        <article className={styles.panel}>
          <div className={styles.panelHead}>
            <Lock size={16} />
            <h2>What THG does not collect</h2>
          </div>
          <ul className={styles.list}>
            {notCollected.map((item) => (
              <li key={item}>{item}</li>
            ))}
          </ul>
        </article>
      </section>

      <section className={styles.grid}>
        <article className={styles.panel}>
          <div className={styles.panelHead}>
            <Bot size={16} />
            <h2>How the data is used</h2>
          </div>
          <ul className={styles.list}>
            {usageRules.map((item) => (
              <li key={item}>{item}</li>
            ))}
          </ul>
        </article>

        <article className={styles.panel}>
          <div className={styles.panelHead}>
            <ShieldCheck size={16} />
            <h2>User control and security</h2>
          </div>
          <ul className={styles.list}>
            {userControls.map((item) => (
              <li key={item}>{item}</li>
            ))}
          </ul>
        </article>
      </section>

      <section className={styles.gridSingle}>
        <article className={styles.panel}>
          <div className={styles.panelHead}>
            <Mail size={16} />
            <h2>Contact and policy questions</h2>
          </div>
          <p className={styles.bodyText}>
            If you need support, data clarification, or want to request removal
            of a connected workspace device, contact{' '}
            <a href="mailto:thgfulfill.com@gmail.com">
              thgfulfill.com@gmail.com
            </a>
            . Requests are reviewed by the THG team using organization-scoped
            audit logs and access controls.
          </p>
        </article>
      </section>
    </main>
  );
}
