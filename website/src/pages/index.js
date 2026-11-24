import Link from '@docusaurus/Link';
import useBaseUrl from '@docusaurus/useBaseUrl';
import useDocusaurusContext from '@docusaurus/useDocusaurusContext';
import Heading from '@theme/Heading';
import Layout from '@theme/Layout';

import styles from './index.module.css';

const features = [
  {
    icon: '📥',
    title: 'Catch every email',
    text: 'An SMTP server for local, QA, staging and CI. Nothing reaches a real inbox unless you release it.',
    to: '/docs/getting-started/connect-your-app',
  },
  {
    icon: '🔍',
    title: 'Find anything',
    text: 'Live grid with a powerful search syntax: from:, tag:, has:attachment, after:2h, header:… with autocomplete.',
    to: '/docs/features/search',
  },
  {
    icon: '🧪',
    title: 'Analyse before you ship',
    text: 'HTML/CSS support across ~20 email clients, broken links, DKIM, one-click unsubscribe and spam score.',
    to: '/docs/features/analysis',
  },
  {
    icon: '🤖',
    title: 'Built for tests',
    text: 'Per-inbox API tokens, a long-polling "wait for the email" endpoint and a send API. OpenAPI 3.1.',
    to: '/docs/integrations/e2e-testing',
  },
  {
    icon: '🔔',
    title: 'Webhooks & chaos',
    text: 'Signed webhooks with retries, and SMTP fault injection per inbox to test how your app handles failures.',
    to: '/docs/integrations/webhooks',
  },
  {
    icon: '🏢',
    title: 'Ready for teams',
    text: 'Inboxes as tenants, admin and member roles, retention, metrics, and replicas that share live updates.',
    to: '/docs/administration/inboxes',
  },
];

const gallery = [
  {img: 'message', title: 'Message view', to: '/docs/features/message-view'},
  {img: 'analysis', title: 'Email analysis', to: '/docs/features/analysis'},
  {img: 'webhooks', title: 'Webhooks', to: '/docs/integrations/webhooks'},
  {img: 'chaos', title: 'Chaos testing', to: '/docs/integrations/chaos-testing'},
];

function Hero() {
  const {siteConfig} = useDocusaurusContext();
  return (
    <header className={styles.hero}>
      <div className="container">
        <img className={styles.logo} src={useBaseUrl('/img/logo.svg')} alt="" width="96" height="128" />
        <Heading as="h1" className={styles.title}>{siteConfig.title}</Heading>
        <p className={styles.tagline}>{siteConfig.tagline}</p>
        <div className={styles.buttons}>
          <Link className="button button--primary button--lg" to="/docs/getting-started/quick-start">
            Quick start · 5 minutes
          </Link>
          <Link className="button button--secondary button--lg" to="/docs/">
            What is PostDream?
          </Link>
        </div>
        <pre className={styles.snippet}>
          <code>docker compose up -d --build   # UI :8025 · SMTP :1025</code>
        </pre>
        <img
          className={styles.screenshot}
          src={useBaseUrl('/img/screenshots/mailbox.png')}
          alt="The PostDream mailbox with captured emails"
          loading="eager"
        />
      </div>
    </header>
  );
}

export default function Home() {
  return (
    <Layout description="PostDream is an SMTP sandbox: it catches every email your apps send so none of them reach a real inbox.">
      <Hero />
      <main>
        <section className={styles.section}>
          <div className="container">
            <div className={styles.grid}>
              {features.map((f) => (
                <Link key={f.title} to={f.to} className={styles.card}>
                  <span className={styles.icon} aria-hidden="true">{f.icon}</span>
                  <Heading as="h3">{f.title}</Heading>
                  <p>{f.text}</p>
                </Link>
              ))}
            </div>
          </div>
        </section>
        <section className={styles.sectionAlt}>
          <div className="container">
            <Heading as="h2" className={styles.center}>A closer look</Heading>
            <div className={styles.gallery}>
              {gallery.map((g) => (
                <Link key={g.img} to={g.to} className={styles.shot}>
                  <img src={useBaseUrl(`/img/screenshots/${g.img}.png`)} alt={g.title} loading="lazy" />
                  <span>{g.title}</span>
                </Link>
              ))}
            </div>
          </div>
        </section>
      </main>
    </Layout>
  );
}
