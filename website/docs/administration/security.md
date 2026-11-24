---
title: "Security"
description: "SMTP authentication modes, login protection, safe rendering, outbound request rules and hardening."
---

# Security

A sandbox that receives email from your apps should not become an open door. These are the protections PostDream
has and the settings that control them.

## SMTP authentication

| `SMTP_AUTH` | Behavior | Recommended for |
|---|---|---|
| `off` | `AUTH` is not offered; everything goes to the default inbox | Local only |
| `optional` *(default)* | With `AUTH` → matching inbox. Without → default inbox | Local or private network |
| `required` | Without `AUTH` the server answers `502 Please authenticate first` | **Any public server** |

Related settings:

- `SMTP_TLS_CERT` / `SMTP_TLS_KEY` enable **STARTTLS**; `SMTP_REQUIRE_TLS=true` rejects mail until the client upgrades.
- `SMTP_ALLOWED_RECIPIENTS` is a global regex every `RCPT TO` must match; inboxes can add their own.
- `SMTP_MAX_MESSAGE_BYTES` (25 MB) and `SMTP_MAX_RECIPIENTS` (100) cap each message.

## UI and API

- **Login required** by default (`AUTH_ENABLED=true`).
- Sessions are stored in Postgres; cookies are `HttpOnly` and `SameSite=Strict`, and `Secure` with `COOKIE_SECURE=true`
  (set it whenever the UI is served over HTTPS).
- Passwords are hashed with **bcrypt**; session tokens are stored as SHA-256 hashes.
- **Brute-force protection**: 10 failed logins per IP every 15 minutes; 10 failed SMTP or POP3 `AUTH` attempts per IP
  every 10 minutes. Behind a reverse proxy set `TRUST_PROXY=true` so the real client IP is used.
- `ALLOWED_HOSTS` limits the accepted `Host` headers, which protects against DNS rebinding.

## Rendering emails safely

- Email HTML is shown in an iframe with `sandbox` and **no scripts**.
- Attachments are always served with `Content-Disposition: attachment` and a sandboxing CSP, so they never render on
  PostDream's own origin.

## Outbound requests

Webhooks and the link check make HTTP requests to user-supplied URLs. They use an SSRF-safe client that **blocks
private addresses** (localhost, 10/8, 172.16/12, 192.168/16, link-local, cloud metadata…). The IP is checked **after
DNS resolution**, so DNS rebinding cannot get around it.

To allow specific internal targets:

| Scope | Setting |
|---|---|
| One inbox | **Allowed internal hosts** in the inbox settings |
| Whole server | `OUTBOUND_ALLOWED_HOSTS=ci.internal,10.1.0.0/16` |
| Everything private (not recommended) | `OUTBOUND_ALLOW_PRIVATE=true` |

## Things to know

- Inbox **SMTP passwords are stored in plain text** on purpose, so the UI can show them and build the snippets. Use
  credentials that exist only for the sandbox.
- Released emails do leave the sandbox: releasing needs both a server setting and a per-inbox allow-list
  ([Releasing emails](../features/release.md)).
- Do not expose PostgreSQL. Keep a separate metrics listener (`METRICS_ADDR`) private.

## Public server checklist

```bash
SMTP_AUTH=required
COOKIE_SECURE=true
TRUST_PROXY=true             # behind a reverse proxy / load balancer
ADMIN_PASSWORD=a-long-password
ALLOWED_HOSTS=mail.example.com
```

Report vulnerabilities as described in the repository's `SECURITY.md`.
