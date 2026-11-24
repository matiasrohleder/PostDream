---
title: "Configuration"
description: "Every environment variable PostDream reads, with its default."
---

# Configuration

PostDream is configured with environment variables only. `.env.example` in the repository is a commented starting
point for Docker Compose. **Settings → Server** shows the effective values of the running instance.

## Core

| Variable | Default | Description |
|---|---|---|
| `DATABASE_URL` | `postgres://postdream:…@localhost:5432/postdream` | PostgreSQL connection string |
| `HTTP_ADDR` | `:8025` | UI/API listen address |
| `SMTP_ADDR` | `:1025` | SMTP listen address |
| `POP3_ADDR` | — (off) | POP3 listen address, for example `:1110` |
| `LOG_LEVEL` | `info` | `debug` for more detail |
| `PUBLIC_URL` | — | Public base URL, used for links in webhook payloads |

## Users and the UI

| Variable | Default | Description |
|---|---|---|
| `AUTH_ENABLED` | `true` | User accounts and login. `false` in the [standalone image](../getting-started/one-command.md) |
| `ACCESS_TOKEN` | — | With `AUTH_ENABLED=false`: one shared key (8+ characters) for the UI and the API instead of user accounts. Ignored when `AUTH_ENABLED=true` |
| `ADMIN_EMAIL` | `admin@postdream.local` | Initial admin (only with an empty database) |
| `ADMIN_PASSWORD` | random, logged | Initial admin password |
| `SESSION_TTL` | `168h` | Session lifetime |
| `COOKIE_SECURE` | `false` | `true` when the UI is served over HTTPS |
| `TRUST_PROXY` | `false` | Use `X-Forwarded-For` for the client IP |
| `ALLOWED_HOSTS` | — (any) | Comma-separated `Host` headers accepted (DNS-rebinding protection) |

## SMTP

| Variable | Default | Description |
|---|---|---|
| `SMTP_HOSTNAME` | `postdream` | Name announced in the SMTP greeting |
| `SMTP_AUTH` | `optional` | `off` · `optional` · `required` ([Security](../administration/security.md#smtp-authentication)) |
| `SMTP_TLS_CERT` / `SMTP_TLS_KEY` | — | Enable STARTTLS with that certificate |
| `SMTP_REQUIRE_TLS` | `false` | Reject mail until the client uses STARTTLS |
| `SMTP_ALLOWED_RECIPIENTS` | — | Global regex every `RCPT TO` must match |
| `SMTP_MAX_MESSAGE_BYTES` | `26214400` | Maximum size per email (25 MB) |
| `SMTP_MAX_RECIPIENTS` | `100` | Maximum recipients per email |
| `DEFAULT_INBOX` | `default` | Slug of the inbox that receives anonymous mail |
| `DEFAULT_INBOX_SMTP_PASSWORD` | random | SMTP password of the default inbox when it is created |

## Retention

| Variable | Default | Description |
|---|---|---|
| `RETENTION_HOURS` | `72` | How many hours emails are kept |
| `RETENTION_INTERVAL` | `10m` | How often the cleanup runs |
| `MAX_MESSAGES_PER_INBOX` | `0` | Per-inbox message cap (0 = unlimited) |

## Webhooks, link check and background jobs

| Variable | Default | Description |
|---|---|---|
| `JOB_WORKERS` | `4` | Background workers (webhooks, spam analysis) |
| `OUTBOUND_ALLOW_PRIVATE` | `false` | Let webhooks and link checks reach private networks |
| `OUTBOUND_ALLOWED_HOSTS` | — | Hosts/CIDRs always reachable, for example `ci.internal,10.1.0.0/16` |

## Spam scoring

| Variable | Default | Description |
|---|---|---|
| `RSPAMD_URL` / `RSPAMD_PASSWORD` | — | Rspamd, for example `http://rspamd:11333` |
| `SPAMASSASSIN_ADDR` | — | spamd `host:port` (used when Rspamd is not set) |

## Releasing

| Variable | Default | Description |
|---|---|---|
| `RELAY_SMTP_URL` | — (off) | SMTP server for [releasing emails](../features/release.md) |
| `RELAY_FROM` | — | Envelope sender override for released emails |

## Metrics and limits

| Variable | Default | Description |
|---|---|---|
| `METRICS_TOKEN` | — | Enables `/metrics` on the HTTP port behind this bearer token |
| `METRICS_ADDR` | — | Separate unauthenticated metrics listener, for example `:9090` |
| `SMTP_MAX_CONNECTIONS` / `HTTP_MAX_CONNECTIONS` | `1000` | Concurrent connection caps |

## Docker Compose only

| Variable | Default | Description |
|---|---|---|
| `SMTP_PORT` / `HTTP_PORT` / `POP3_PORT` | `1025` / `8025` / `1110` | Ports published on the host |
| `POSTGRES_PASSWORD` | `postdream` | Password of the bundled Postgres |
| `VERSION` | current | Version baked into the image |
