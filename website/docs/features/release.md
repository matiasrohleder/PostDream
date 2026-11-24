---
title: "Releasing emails"
description: "Opt-in forwarding of a captured email to a real SMTP server, with recipient allow-lists and an audit log."
---

# Releasing emails to real recipients

PostDream's promise is that **nothing leaves the sandbox**. Releasing is the one exception, and it is off by default:
it lets someone forward a captured email to a real mailbox, for example to look at it on a phone or in Outlook.

## Turning it on

Two things are required, one by the server operator and one by an admin:

1. **Server:** set `RELAY_SMTP_URL` to a real SMTP server.

   | Value | Meaning |
   |---|---|
   | `smtp://user:pass@smtp.example.com:587` | SMTP with STARTTLS |
   | `smtps://user:pass@smtp.example.com:465` | Implicit TLS |
   | `smtp://host:25?tls=none` | Plain SMTP |

   Optionally set `RELAY_FROM` to override the envelope sender.

2. **Inbox:** in the inbox's **Settings**, fill **Allowed release recipients**, a regular expression every recipient
   must match, for example `@acme\.test$`. Inboxes without it cannot release anything.

## Releasing

Open an email and click the ✈ button. Pick the recipients (only addresses matching the inbox's pattern are accepted)
and **Release**.

![Release dialog](/img/screenshots/release.png)

The email is delivered **as-is**, with two extra headers: `X-PostDream-Released-By` and
`X-PostDream-Released-At`.

It is also available as `POST /api/messages/{id}/release`, for UI sessions only: an inbox API token cannot release.

## Audit log

Every attempt, successful or not, is recorded with the date, user, subject, recipients and result. Admins see it in
**Settings → Server → Release audit log** (and at `GET /api/relay/log`).
