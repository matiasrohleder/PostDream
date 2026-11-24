---
title: "Server page"
description: "Statistics, the effective configuration and the release audit log at a glance."
---

# Server page

**Settings → Server** shows how this PostDream is running. The configuration itself comes from environment
variables ([Configuration](../operations/configuration.md)); this page only displays it.

![Server page](/img/screenshots/server.png)

| Section | Content |
|---|---|
| Statistics | Emails, unread, received in the last 24 h and storage used (for the inboxes you can see) |
| Version | The running version, also in the sidebar and in `GET /api/health` |
| SMTP | Port, authentication mode, TLS and maximum email size |
| UI login | Required or disabled |
| Retention and default inbox | Global defaults |
| POP3 | Port, when enabled |
| Spam engine | Rspamd, SpamAssassin or none |
| Release / relay | Enabled when `RELAY_SMTP_URL` is set |
| Prometheus metrics | Enabled when a metrics token or listener is configured |
| Release audit log | Admins: every [release](../features/release.md) attempt |

## API documentation in the UI

**Settings → API** renders the OpenAPI 3.1 document of the running server, grouped by area, with every route,
parameter and schema. See [REST API](../integrations/rest-api.md).

![API docs page](/img/screenshots/api-docs.png)
