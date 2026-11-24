---
title: "Webhooks"
description: "Signed HTTP callbacks for every email received in an inbox, with retries and a delivery log."
---

# Webhooks

A webhook makes PostDream `POST` a JSON payload to your URL every time an email lands in an inbox. Use it to trigger
a CI step, post to chat, or feed another test harness.

## Setting one up

**Settings → Inboxes → Webhooks** (admins). Paste a URL and **Add webhook**. Each inbox can have several.

![Webhooks of an inbox](/img/screenshots/webhooks.png)

For each webhook you can:

| Button | Action |
|---|---|
| ▶ **Send test ping** | Delivers a signed ping now and shows the HTTP status and time |
| ✓ / ⏸ **Enable / Disable** | Pause deliveries without deleting the webhook |
| 🕑 **Deliveries** | The latest attempts with status, attempt number and error |
| 🗑 **Delete** | Remove it |
| 👁 / 📋 / 🔑 **Signing secret** | Show, copy or regenerate the secret (receivers must be updated) |

The badge shows the result of the last delivery.

## Payload

```json
{
  "event": "message.received",
  "delivery_id": "dlv_…",
  "created_at": "2026-11-23T21:06:20Z",
  "inbox": { "id": "…", "slug": "acme-staging", "name": "Acme Shop · staging" },
  "message": { "id": "…", "from": "…", "to": "…", "subject": "…", "tags": ["signup"], "size": 1234 },
  "links": { "api": "https://mail.example.com/api/messages/…", "ui": "…", "raw": "…" }
}
```

`links` is included when `PUBLIC_URL` is set. Tags from [auto-tag rules](../features/tags-and-rules.md) are already
applied when the webhook fires.

Request headers: `Content-Type: application/json`, `X-PostDream-Event`, `X-PostDream-Delivery` and
`X-PostDream-Signature`.

## Verifying the signature

```
X-PostDream-Signature: t=<unix seconds>,v1=<hex HMAC-SHA256(secret, "<t>.<raw body>")>
```

Recompute the HMAC over the **raw** body, compare in constant time and reject old timestamps:

```js
import crypto from 'node:crypto'

function verify(rawBody, header, secret, maxAgeSec = 300) {
  const parts = Object.fromEntries(header.split(',').map((p) => p.split('=')))
  if (Math.abs(Date.now() / 1000 - Number(parts.t)) > maxAgeSec) return false
  const expected = crypto.createHmac('sha256', secret).update(`${parts.t}.${rawBody}`).digest('hex')
  return crypto.timingSafeEqual(Buffer.from(expected), Buffer.from(parts.v1))
}
```

## Retries

A delivery fails on a network error or a non-2xx answer. Failed deliveries are retried with backoff
(**10 s, 30 s, 2 min, 10 min, 30 min**) through a durable job queue stored in PostgreSQL, so they survive restarts and
are shared between replicas. The last 100 attempts per webhook are kept; the UI and `GET /api/webhooks/{id}/deliveries` show the latest 50.

## Reaching internal services

Webhook URLs on private networks (localhost, 10.x, 192.168.x…) are **blocked** unless allowed: per inbox with
**Allowed internal hosts**, or server-wide with `OUTBOUND_ALLOWED_HOSTS`. See
[Security → Outbound requests](../administration/security.md#outbound-requests).
