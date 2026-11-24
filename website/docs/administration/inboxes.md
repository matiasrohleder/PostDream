---
title: "Inboxes"
description: "Inboxes are PostDream's tenants - each with its own SMTP credentials, API token, retention and settings."
---

# Inboxes

An **inbox** is a tenant. It groups the emails of one project or environment and owns:

- a **name**, a **slug** and an optional description;
- an **SMTP username and password**: emails sent with them land in this inbox;
- an **API token** for tests and scripts, scoped to this inbox only;
- an optional **retention** override;
- **settings**, **webhooks**, **auto-tag rules** and **chaos rules**.

Manage them in **Settings → Inboxes**. Admins see and manage every inbox; members only see the inboxes assigned to
them, without the admin buttons.

![Inboxes](/img/screenshots/settings-inboxes.png)

## The default inbox

On first start PostDream creates the `default` inbox (slug configurable with `DEFAULT_INBOX`, password with
`DEFAULT_INBOX_SMTP_PASSWORD`). Emails sent **without** SMTP authentication land there when `SMTP_AUTH=optional`.

## Creating an inbox

Click **New inbox** and fill:

| Field | Notes |
|---|---|
| Name | For example `Acme Shop · staging` |
| Slug | Generated from the name if empty; used in URLs, search (`inbox:slug`) and the send API |
| Description | Optional |
| SMTP username | Defaults to the slug |
| SMTP password | Generated automatically if empty |
| Retention (hours) | Empty = the global default (`RETENTION_HOURS`) |

The inbox card then shows host, port, username, password and API token, each with show/copy buttons.

:::tip A good layout
One inbox per project and environment (`shop-staging`, `shop-qa`, `billing-qa`, `e2e`), with short retention for CI
inboxes and longer retention for manual QA.
:::

## Card actions

| Button | What it does |
|---|---|
| ✏️ Edit | Rename, change credentials or retention |
| 🔑 Regenerate credentials | New SMTP password **and** API token. Apps and tests using the old ones stop working |
| 🗑 Delete | Deletes the inbox **and all its emails** |
| **Show configuration examples** | [Snippets](../getting-started/connect-your-app.md#ready-made-snippets) with this inbox's credentials |
| **Settings** | See below |
| **Webhooks** | [Signed webhooks](../integrations/webhooks.md) |
| **Rules** | [Auto-tag rules](../features/tags-and-rules.md#auto-tag-rules) |
| **Chaos** | [SMTP fault injection](../integrations/chaos-testing.md); the card shows a warning while it is active |

## Inbox settings

![Inbox settings](/img/screenshots/inbox-settings.png)

| Setting | Effect |
|---|---|
| **Spam check** | Score every received email with the configured [spam engine](../features/analysis.md#spam-score) |
| **Allowed recipients (regex)** | SMTP rejects `RCPT TO` addresses that do not match. Empty = accept all. Stops an app from sending to unexpected domains |
| **Expected link domains** | The [link check](../features/analysis.md#links--images) warns about links to any other domain |
| **Allowed internal hosts** | Private hosts or CIDRs that webhooks and the link check may reach for this inbox |
| **Allowed release recipients (regex)** | Who emails may be [released](../features/release.md) to. Empty = releasing disabled for this inbox |
