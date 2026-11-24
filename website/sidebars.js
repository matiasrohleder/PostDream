// @ts-check
/** @type {import('@docusaurus/plugin-content-docs').SidebarsConfig} */
const sidebars = {
  docs: [
    'intro',
    {type: 'category', label: 'Getting started', collapsed: false, items: [
      'getting-started/one-command',
      'getting-started/quick-start',
      'getting-started/sending-test-emails',
      'getting-started/connect-your-app',
    ]},
    {type: 'category', label: 'Using the UI', collapsed: false, items: [
      'features/mailbox',
      'features/message-view',
      'features/search',
      'features/tags-and-rules',
      'features/analysis',
      'features/release',
      'features/preferences',
    ]},
    {type: 'category', label: 'Administration', items: [
      'administration/inboxes',
      'administration/users-and-roles',
      'administration/security',
      'administration/retention',
      'administration/server',
    ]},
    {type: 'category', label: 'Integrations', items: [
      'integrations/rest-api',
      'integrations/e2e-testing',
      'integrations/webhooks',
      'integrations/chaos-testing',
      'integrations/pop3',
    ]},
    {type: 'category', label: 'Operations', items: [
      'operations/deployment',
      'operations/railway',
      'operations/configuration',
      'operations/metrics',
      'operations/scaling',
      'operations/architecture',
    ]},
    'development',
  ],
};

export default sidebars;
