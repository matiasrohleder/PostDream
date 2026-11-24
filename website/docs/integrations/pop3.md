---
title: "POP3"
description: "Read an inbox from Thunderbird, Outlook or any mail client."
---

# POP3

With POP3 enabled, **every inbox is also a mailbox** you can add to a regular mail client, using the inbox's **SMTP
credentials**. Useful to see emails exactly as a desktop client renders them, or to give a stakeholder a familiar view
of one project's emails.

## Enabling it

Set `POP3_ADDR` (for example `:1110`; `docker-compose.yml` already does and publishes `POP3_PORT`). The port is shown
in **Settings → Server** and in the inbox's configuration examples.

## Client settings

| Setting | Value |
|---|---|
| Server | PostDream's host |
| Port | `1110` (or the published port) |
| Username / password | The inbox's SMTP username and password |
| Security | None, or STARTTLS (`STLS`) when `SMTP_TLS_CERT` is configured |

## Behavior

- The newest 1000 emails of the inbox are listed.
- Downloading an email (`RETR`) marks it as **read** in PostDream.
- Deleting (`DELE`) removes the email from PostDream when the client ends the session (`QUIT`).
- Failed logins count toward the same brute-force limit as SMTP.
