---
title: "Chaos testing"
description: "Make the SMTP server fail on purpose, for one inbox, to test how your app handles errors."
---

# Chaos testing

Real SMTP servers fail: mailboxes are full, greylisting returns `451`, messages are too big. **Chaos testing** makes
PostDream fail the same way **for one inbox only**, so you can test retries, error messages and queues without
breaking anyone else's emails.

![Chaos rules](/img/screenshots/chaos.png)

## Rules

**Settings → Inboxes → Chaos**. For each SMTP stage you can enable a fault:

| Stage | When it fails |
|---|---|
| **AUTH** | Login with the inbox credentials |
| **MAIL FROM** | Start of every transaction |
| **RCPT TO** | Every recipient |
| **DATA** | After the message body is uploaded |

…with a **reply code** (`4xx` = temporary, clients should retry; `5xx` = permanent), a **probability** (0–100 %) and an
optional **message**.

While chaos is active, the inbox card shows a warning so nobody wonders why emails are failing.

## From a test

Tests can switch chaos on and off with the **inbox's own API token**, no admin session needed:

```bash
# 50 % of recipients fail with 451
curl -X PUT -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  http://localhost:8025/api/inboxes/$INBOX_ID/chaos -d '{"rcpt":{"code":451,"probability":50}}'

# ...run the resilience test...

# back to normal
curl -X PUT -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  http://localhost:8025/api/inboxes/$INBOX_ID/chaos -d '{}'
```

Omitting a stage disables it. Each stage accepts `code`, `probability` and `message`.

## Ideas

- `RCPT TO` → `550` at 100 %: does your app mark the address as bounced?
- `DATA` → `552` at 100 %: does it handle "message too large" with a useful error?
- `MAIL FROM` → `421` at 30 %: does your queue retry with backoff and eventually deliver?
