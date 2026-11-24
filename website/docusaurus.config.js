// @ts-check
import {themes as prismThemes} from 'prism-react-renderer';

const repo = 'https://github.com/matiasrohleder/PostDream';

/** @type {import('@docusaurus/types').Config} */
const config = {
  title: 'PostDream',
  tagline: 'An SMTP sandbox that catches every email your apps send, so none of them ever reach a real inbox.',
  favicon: 'img/logo.svg',

  future: {
    v4: true,
    faster: true,
  },

  // Overridable so previews and forks can build with their own domain.
  url: process.env.DOCS_URL || 'https://docs-postdream.up.railway.app',
  baseUrl: '/',

  organizationName: 'matiasrohleder',
  projectName: 'PostDream',

  onBrokenLinks: 'throw',
  onBrokenAnchors: 'throw',
  markdown: {
    // .md files are plain CommonMark (no JSX), so braces and angle brackets in prose are safe; .mdx opts in.
    format: 'detect',
    mermaid: true,
    hooks: {onBrokenMarkdownLinks: 'throw'},
  },

  i18n: {
    defaultLocale: 'en',
    locales: ['en'],
  },

  presets: [
    [
      'classic',
      /** @type {import('@docusaurus/preset-classic').Options} */
      ({
        docs: {
          routeBasePath: '/docs',
          sidebarPath: './sidebars.js',
          editUrl: `${repo}/tree/main/website/`,
          showLastUpdateTime: false,
        },
        blog: false,
        theme: {
          customCss: './src/css/custom.css',
        },
      }),
    ],
  ],

  themes: [
    '@docusaurus/theme-mermaid',
    [
      '@easyops-cn/docusaurus-search-local',
      /** @type {import('@easyops-cn/docusaurus-search-local').PluginOptions} */
      ({
        hashed: true,
        indexBlog: false,
        docsRouteBasePath: '/docs',
        highlightSearchTermsOnTargetPage: true,
      }),
    ],
  ],

  themeConfig:
    /** @type {import('@docusaurus/preset-classic').ThemeConfig} */
    ({
      image: 'img/screenshots/mailbox.png',
      colorMode: {
        respectPrefersColorScheme: true,
      },
      navbar: {
        title: 'PostDream',
        logo: {alt: 'PostDream', src: 'img/logo.svg'},
        items: [
          {type: 'docSidebar', sidebarId: 'docs', position: 'left', label: 'Docs'},
          {to: '/docs/features/search', label: 'Search syntax', position: 'left'},
          {to: '/docs/integrations/rest-api', label: 'API', position: 'left'},
          {href: repo, label: 'GitHub', position: 'right'},
        ],
      },
      footer: {
        style: 'dark',
        links: [
          {
            title: 'Docs',
            items: [
              {label: 'Introduction', to: '/docs/'},
              {label: 'Quick start', to: '/docs/getting-started/quick-start'},
              {label: 'Configuration', to: '/docs/operations/configuration'},
            ],
          },
          {
            title: 'Use it for',
            items: [
              {label: 'E2E tests', to: '/docs/integrations/e2e-testing'},
              {label: 'Webhooks', to: '/docs/integrations/webhooks'},
              {label: 'Email analysis', to: '/docs/features/analysis'},
            ],
          },
          {
            title: 'More',
            items: [
              {label: 'GitHub', href: repo},
              {label: 'Changelog', href: `${repo}/commits/main`},
            ],
          },
        ],
        copyright: `PostDream · Go · PostgreSQL · React`,
      },
      prism: {
        theme: prismThemes.github,
        darkTheme: prismThemes.dracula,
        additionalLanguages: ['bash', 'json', 'csharp', 'php', 'yaml', 'nginx'],
      },
      mermaid: {
        theme: {light: 'neutral', dark: 'dark'},
      },
    }),
};

export default config;
