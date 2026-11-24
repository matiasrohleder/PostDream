<p align="center">
  <img src="docs/logo.svg" alt="PostDream" width="150">
</p>

<h1 align="center">PostDream</h1>

<p align="center">
  <b>An SMTP sandbox: it catches every email your apps send so none of them ever reach a real inbox.</b><br>
  <i>In a sandbox, emails are just dreams: they look real, but they are caught before they reach the world.</i><br>
  Go · PostgreSQL · React · Docker · MIT
</p>

---

📖 **Documentation:** https://docs-postdream.up.railway.app (source in [`website/`](website/)).

Point your app's SMTP settings (local, QA, staging…) at PostDream and every message is **intercepted**
and shown in a web UI with a data grid, filters and previews. Nothing is ever relayed to a real recipient.

![Email grid](website/static/img/screenshots/mailbox.png)

<details>
<summary>More screenshots</summary>

| Message view | Email analysis | Dark mode |
|---|---|---|
| ![](website/static/img/screenshots/message.png) | ![](website/static/img/screenshots/analysis.png) | ![](website/static/img/screenshots/mailbox-dark.png) |

</details>

## Contents

- [Features](#features)
- [Quick start (5 minutes)](#quick-start-5-minutes)
- [Sending test emails](#sending-test-emails)
- [Configuring your application](#configuring-your-application)
- [Multitenancy: inboxes, users and roles](#multitenancy-inboxes-users-and-roles)
- [Security and SMTP passwords](#security-and-smtp-passwords)
- [Retention (automatic cleanup)](#retention-automatic-cleanup)
- [REST API (for automated tests)](#rest-api-for-automated-tests)
- [Search syntax](#search-syntax)
- [Tags and auto-tag rules](#tags-and-auto-tag-rules)
- [Webhooks](#webhooks)
- [Email analysis](#email-analysis)
- [Chaos testing](#chaos-testing)
- [POP3](#pop3)
- [Releasing emails to real recipients](#releasing-emails-to-real-recipients)
- [Metrics](#metrics)
- [Scaling and performance](#scaling-and-performance)
- [Environment variables](#environment-variables)
- [Deploying to a server](#deploying-to-a-server)
- [Development](#development)
- [Architecture](#architecture)

## Features

- 📥 **SMTP server** (port `1025`) that accepts any message and stores it in PostgreSQL. Supports `AUTH PLAIN`/`LOGIN` and optional STARTTLS.
- 🖥️ **Web UI** (port `8025`) with a sortable, paginated, multi-select **grid**; **real-time updates** (SSE) and notifications.
- 🔍 **Search syntax** (`from:` `to:` `subject:` `tag:` `is:unread` `has:attachment` `larger:2M` `after:2h`, negation, phrases, `inbox:` `header:Name=value` `spam:>5` `ip:` `smtp-user:`), with autocomplete and history; classic filters and date presets too.
- 🏷️ **Tags**: manual, bulk, from plus-addresses (`qa+signup@…` → `signup`), from an `X-Tags` header, and **auto-tag rules per inbox** written in the search syntax.
- 👀 **Message view**: HTML (in an isolated iframe with no scripts, with desktop/tablet/phone widths), plain text, headers, raw source, SMTP envelope data (MAIL FROM, RCPT TO, client IP, SMTP user) and downloadable attachments. `.eml` download.
- 🏢 **Multitenancy through inboxes**: each inbox has its own SMTP username/password and API token. *Member* users only see the inboxes assigned to them.
- 🔐 **Login required** by default (sessions stored in Postgres, `HttpOnly`/`SameSite=Strict` cookies, bcrypt, brute-force rate limiting).
- 🔑 **SMTP password** optional or mandatory (`SMTP_AUTH=required`) so nobody can flood your sandbox once it's public.
- 🧹 **Retention**: automatically deletes emails older than **72 h** (configurable globally and per inbox), with an optional per-inbox message cap.
- 🤖 **REST API** with per-inbox tokens, including a long-poll `latest?wait=30s` endpoint that's perfect for E2E tests ("wait for the password-reset email and give me the link"), a **send API** and an **OpenAPI 3.1** spec browsable in the UI.
- 🔔 **Signed webhooks per inbox** (HMAC-SHA256), with retries, a delivery log and a test button. Desktop notifications in the browser.
- 🧪 **Email analysis**: HTML/CSS compatibility with ~20 email clients ([caniemail](https://www.caniemail.com) data), Gmail clipping, link & image check (flags localhost/unexpected domains), deliverability checks (DKIM, one-click unsubscribe, alignment, headers) and spam score with **Rspamd or SpamAssassin**.
- 💥 **Chaos testing per inbox**: make AUTH/MAIL/RCPT/DATA fail with any code and probability, toggled from the UI or by a test with the inbox API token.
- 📬 **POP3** (each inbox is a mailbox with its own credentials), **opt-in release** to a real SMTP server with per-inbox recipient allow-lists and an audit log, **Prometheus metrics**.
- 📈 **Scales out**: several replicas behind a load balancer share live updates (Postgres `LISTEN/NOTIFY`) and the background job queue.
- 🌍 **Languages**: English, Español and Deutsch (auto-detected from the browser, switchable in the top bar and on the login page).
- 🌗 **Dark mode** that follows the operating system by default, with a manual light/dark override; keyboard shortcuts (`j`/`k`, `Del`, `Esc`); responsive layout.
- 📦 A single Go binary with the UI embedded, plus Postgres, all with `docker compose up`.

## Quick start

### One command, no setup

```bash
docker run --rm -p 8025:8025 -p 1025:1025 matiasrohleder/postdream:standalone
```

Open **http://localhost:8025** (no login) and point your app's SMTP at **localhost:1025**. The `standalone` image
brings its own PostgreSQL; add `-v postdream:/data` to keep the emails and `-e ACCESS_TOKEN=a-long-key` to protect the
UI and API with a single key. Ideal for your laptop, tests and CI.
See [Run it in one command](https://docs-postdream.up.railway.app/docs/getting-started/one-command).

### For a team (user accounts, external PostgreSQL)

Requirements: **Docker** with Docker Compose. You don't need to clone anything, nor install Go or Node.

Save this as `docker-compose.yml` and run `docker compose up -d`:

```yaml
services:
  db:
    image: postgres:17-alpine
    environment: { POSTGRES_USER: postdream, POSTGRES_PASSWORD: postdream, POSTGRES_DB: postdream }
    volumes: [pgdata:/var/lib/postgresql/data]
    healthcheck: { test: ["CMD-SHELL", "pg_isready -U postdream"], interval: 5s, retries: 20 }
  postdream:
    image: ghcr.io/matiasrohleder/postdream:latest
    depends_on: { db: { condition: service_healthy } }
    ports: ["8025:8025", "1025:1025"]
    environment:
      DATABASE_URL: postgres://postdream:postdream@db:5432/postdream?sslmode=disable
      ADMIN_PASSWORD: change-me-please
volumes:
  pgdata:
```

- UI: **http://localhost:8025** → sign in with `admin@postdream.local` / `ADMIN_PASSWORD`.
  If you leave `ADMIN_PASSWORD` empty, a random one is generated and printed in `docker compose logs postdream`.
- SMTP: **localhost:1025**

> The admin user is only created on first start (empty database). Change the password later in *Settings → My account*.

The image is multi-arch (`linux/amd64`, `linux/arm64`) and also published on Docker Hub as `matiasrohleder/postdream`.

### From source

```bash
git clone https://github.com/matiasrohleder/PostDream.git
cd PostDream
cp .env.example .env          # edit ADMIN_PASSWORD
docker compose up -d --build
```

`Makefile` shortcuts: `make up`, `make down`, `make logs`, `make send`, `make test`.

## Sending test emails

With the bundled script (Python standard library only):

```bash
python3 scripts/send-test-mail.py                     # 1 anonymous email → "default" inbox
python3 scripts/send-test-mail.py -n 20               # 20 emails
python3 scripts/send-test-mail.py --user project-x --password XXXX --to qa@acme.test
python3 scripts/send-test-mail.py --attach invoice.pdf --subject "Your invoice"
```

Or with any SMTP tool, e.g. [swaks](https://github.com/jetmore/swaks):

```bash
swaks --server localhost:1025 --to user@example.com --header "Subject: Hello"
```

## Configuring your application

| Setting   | Value                                      |
|-----------|--------------------------------------------|
| Host      | `localhost` (or the server's IP/domain)    |
| Port      | `1025`                                     |
| Security  | none / STARTTLS if you configured a cert   |
| Username  | the inbox's SMTP username (e.g. `default`) |
| Password  | the inbox's SMTP password                  |

*Settings → Inboxes → Show configuration examples* has copy-paste snippets
(.env, Node/nodemailer, Python, .NET, Laravel, swaks and the API) with the credentials already filled in.

If your app runs in another container on the same Docker network, use `postdream:1025` as the host.

## Multitenancy: inboxes, users and roles

- **Inbox** = tenant. It has a name, a slug, an **SMTP username + password**, an **API token** and an optional retention override.
- An email that authenticates over SMTP with an inbox's credentials **lands in that inbox**.
- An **unauthenticated** email lands in the `default` inbox (with `SMTP_AUTH=optional`) or is **rejected** (with `SMTP_AUTH=required`).
- **Users**:
  - `admin`: sees every inbox, creates/edits/deletes inboxes and users, rotates credentials.
  - `member`: only sees (and can clean up) the inboxes an admin assigned to them.

Typical setup: one inbox per project or environment (`project-x-qa`, `project-x-staging`), and each team only sees its own.

## Security and SMTP passwords

| `SMTP_AUTH` | Behavior                                                                  | Recommended for         |
|-------------|---------------------------------------------------------------------------|-------------------------|
| `off`       | AUTH is not offered; everything goes to the `default` inbox.               | Local only              |
| `optional`  | With AUTH → matching inbox. Without AUTH → `default` inbox. **(default)**   | Local / private network |
| `required`  | Without AUTH the server answers `502 Please authenticate first`.           | **Public server**       |

Other built-in protections:

- Login required in the UI (`AUTH_ENABLED=true` by default). Only disable it for 100% local use.
- Rate limiting: 10 failed logins per IP every 15 min; 10 failed SMTP AUTH attempts per IP every 10 min.
- Email HTML is rendered in a **sandboxed iframe with scripts disabled**; attachments are always downloaded, never rendered on the app's origin.
- Message size limit (`SMTP_MAX_MESSAGE_BYTES`, 25 MB) and recipient limit (`SMTP_MAX_RECIPIENTS`).
- User passwords are stored with bcrypt; session tokens are stored as SHA-256 hashes.
- Inbox SMTP passwords are intentionally stored in plain text (so the UI can show them and build the snippets). Use credentials that exist only for the sandbox.

## Retention (automatic cleanup)

Every `RETENTION_INTERVAL` (10 min) a background job deletes:

1. emails older than `RETENTION_HOURS` (**72 h = 3 days by default**), or the inbox's own retention if set;
2. if `MAX_MESSAGES_PER_INBOX > 0`, the oldest emails beyond that cap per inbox;
3. expired sessions.

You can also empty an inbox or delete the filtered results from the UI (**Empty** button) or through the API.

## REST API (for automated tests)

Authentication: a session cookie (UI) **or** `Authorization: Bearer <inbox api_token>` (scoped to that inbox).

The full, always up-to-date reference is the **OpenAPI 3.1** document at `/api/openapi.json`, also browsable in the UI
under *Settings → API* (a test fails the build if a route is not documented). The most used endpoints:

| Method | Path                                           | Description                                              |
|--------|------------------------------------------------|----------------------------------------------------------|
| GET    | `/api/messages?q=…`                            | Paginated list with filters / [search syntax](#search-syntax) |
| GET    | `/api/messages/latest?wait=30s`                | Newest email matching the filters; waits up to `wait`    |
| GET    | `/api/messages/{id}`                           | Details (marks as read; `?peek=1` to skip that)          |
| GET    | `/api/messages/{id}/html` · `/raw` · `/attachments/{aid}` | HTML (cid: inlined), RFC 5322 source, attachments |
| GET    | `/api/messages/{id}/html-check` · `/link-check` · `/deliverability` · `/spam` | [Analysis](#email-analysis) |
| PUT    | `/api/messages/{id}/tags` `{"tags":[…]}`        | Replace tags (`POST /api/messages/tags` for bulk add/remove) |
| POST   | `/api/send`                                    | Inject an email without SMTP (JSON fields or a raw `.eml`) |
| DELETE | `/api/messages?…filters`                       | Delete everything matching the filters                   |
| POST   | `/api/messages/delete` · `/api/messages/read`  | Bulk delete / mark read-unread                           |
| PUT    | `/api/inboxes/{id}/chaos`                      | [Chaos rules](#chaos-testing) (admins or the inbox token) |
| *      | `/api/inboxes…`, `/api/webhooks…`, `/api/rules`, `/api/users…` | Administration (admin session)              |

**Filters** (query string): `inbox`, `q`, `from`, `to`, `subject`, `since`, `until` (RFC 3339 or `YYYY-MM-DD`),
`has_attachments`, `unread`, `sort` (`received_at|subject|from|to|size|inbox`), `order` (`asc|desc`), `page`, `page_size` (max 500).

Example in an E2E test:

```bash
TOKEN=pd_xxxxxxxx
# clean up before the test
curl -X DELETE -H "Authorization: Bearer $TOKEN" http://localhost:8025/api/messages
# ... your app sends the "forgot password" email ...
curl -H "Authorization: Bearer $TOKEN" \
  "http://localhost:8025/api/messages/latest?to=user@example.com&subject=password&wait=30s" | jq -r .text_body

# or with the search syntax
curl -G -H "Authorization: Bearer $TOKEN" http://localhost:8025/api/messages/latest \
  --data-urlencode 'q=to:user@example.com subject:"reset your password"' --data-urlencode wait=30s

# inject an email without SMTP (same pipeline: tags, rules, webhooks)
curl -X POST -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' http://localhost:8025/api/send \
  -d '{"from":"app@example.com","to":["user@example.com"],"subject":"Hi","html":"<b>Hi</b>","tags":["demo"]}'
```

*Settings → Inboxes → Show configuration examples* also has a ready-to-paste Playwright test.

## Search syntax

The search box and the `q` parameter accept words, `"quoted phrases"` and filters. Prefix any term with `-` or `!` to
negate it. Everything is combined with AND.

| Filter | Meaning |
|---|---|
| `from:` `to:` `cc:` `bcc:` `reply-to:` `addressed:` | Address fields (`to:` also matches envelope recipients; `bcc:` = envelope recipients not in To/Cc) |
| `subject:` `message-id:` | Header contains |
| `tag:name` | Has the tag (case-insensitive) |
| `is:read` `is:unread` `is:tagged` `is:spam` | Status |
| `has:attachment` `has:inline` `has:html` | Content |
| `larger:2M` `smaller:500K` | Size |
| `after:2026/11/01` `before:2026-11-15` `after:2h` `before:3d` | Received date, or an age (`m`, `h`, `d`, `w`) |
| `inbox:slug` `smtp-user:name` `ip:10.0.` | Origin |
| `header:X-Env=staging` `header:List-Unsubscribe` | Any header (contains / exists) |
| `spam:>5` `spam:<1` | Spam score (needs a spam engine) |

Examples: `from:shop "order #" -is:read` · `tag:signup after:2h` · `has:attachment larger:1M -inbox:default`.

## Tags and auto-tag rules

- Tags are added by hand (message view, or select several emails → **Tag**) or through the API.
- **Plus-addressing**: mail to `qa+signup@acme.test` gets the tag `signup`. An `X-Tags: a, b` header adds `a` and `b`.
- **Rules** (*Settings → Inboxes → Rules*, admin): every new email matching the rule's search query gets the tag, e.g.
  `subject:"reset password"` → `password-reset`. A rule can apply to one inbox or to all of them.
- Click a tag in the grid to filter by it.

## Webhooks

*Settings → Inboxes → Webhooks* (admin). Each inbox can have several webhooks; every email received there triggers a
`POST` with this JSON body (`links` is present when `PUBLIC_URL` is set):

```json
{ "event": "message.received", "delivery_id": "dlv_…", "created_at": "…",
  "inbox": { "id": "…", "slug": "qa", "name": "QA" },
  "message": { "id": "…", "from": "…", "to": "…", "subject": "…", "tags": ["signup"], "size": 1234, … },
  "links": { "api": "https://mail.example.com/api/messages/…", "ui": "…", "raw": "…" } }
```

Requests are signed: `X-PostDream-Signature: t=<unix>,v1=<hex HMAC-SHA256(secret, "<t>.<raw body>")>`. Recompute it,
compare in constant time and reject old timestamps (the configuration examples include a Node.js verifier).
Failed deliveries (non-2xx or network errors) are retried with backoff (10 s, 30 s, 2 min, 10 min, 30 min) through a
durable queue in Postgres; the last 100 attempts per webhook are shown in the UI.

Outbound requests never reach **private addresses** (localhost, 10/8, 192.168/16, link-local, cloud metadata…) unless
the host or CIDR is listed in the inbox's *Allowed internal hosts* (or globally with `OUTBOUND_ALLOWED_HOSTS` /
`OUTBOUND_ALLOW_PRIVATE`). The IP is checked after DNS resolution, so DNS rebinding cannot bypass it.

## Email analysis

The **Analysis** tab of an email shows:

- **Email client compatibility**: every HTML/CSS feature used is checked against the [caniemail.com](https://www.caniemail.com)
  dataset (vendored, MIT; refresh with `python3 scripts/update-caniemail.py`), with a score per client and the clients
  that do not support each feature. Also warns about Gmail's 102 KB clipping, images without `alt`, HTTP images,
  `<script>`, forms, external CSS, missing `lang`/viewport and HTML-only mail.
- **Deliverability**: DKIM verification, From/envelope alignment, Message-ID, Date, MIME, RFC 8058 one-click
  unsubscribe (required by Gmail/Yahoo for bulk senders) and raw 8-bit headers. No external service needed.
- **Spam score** with **Rspamd** (`RSPAMD_URL`) or **SpamAssassin** (`SPAMASSASSIN_ADDR`): on demand, or automatically
  for every email of inboxes with *Spam check* enabled (runs in the background; searchable with `spam:>5`).
  `docker compose --profile spam up -d` starts an Rspamd container (`RSPAMD_URL=http://rspamd:11333`).
- **Links & images**: requests every link/image (HEAD, then GET), flags broken ones, links to `localhost`, plain HTTP and
  links outside the inbox's *Expected link domains* (catches production URLs in staging mail). Results are cached
  for 15 minutes; limited to 30 checks per 10 minutes per user/token.

## Chaos testing

*Settings → Inboxes → Chaos* makes the SMTP server fail on purpose **for that inbox only**: choose the stage
(AUTH, MAIL FROM, RCPT TO, DATA), the reply code (4xx temporary / 5xx permanent), the probability and an optional
message. Tests can toggle it with the inbox's own API token:

```bash
curl -X PUT -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  http://localhost:8025/api/inboxes/$INBOX_ID/chaos -d '{"rcpt":{"code":451,"probability":50}}'
# ...run the resilience test...
curl -X PUT -H "Authorization: Bearer $TOKEN" http://localhost:8025/api/inboxes/$INBOX_ID/chaos -d '{}'
```

## POP3

With `POP3_ADDR` set (`:1110` in `docker-compose.yml`), every inbox is a POP3 mailbox using **its SMTP credentials**,
so Thunderbird/Outlook can show one project's emails. It exposes the newest 1000 messages, `RETR` marks them as read
and `DELE` deletes them on `QUIT`. STARTTLS (`STLS`) is offered when `SMTP_TLS_CERT` is configured.

## Releasing emails to real recipients

Off by default — PostDream's promise is that nothing leaves the sandbox. To allow it:

1. Set `RELAY_SMTP_URL` (e.g. `smtp://user:pass@smtp.example.com:587`, `smtps://…:465`, `?tls=none` for plain SMTP).
2. In the inbox settings, set **Allowed release recipients**, a regular expression every recipient must match
   (e.g. `@acme\.test$`). Inboxes without it cannot release anything.

Then the send icon of an email (or `POST /api/messages/{id}/release`, UI session only) delivers it as-is with
`X-PostDream-Released-By/At` headers. Every attempt is recorded in an audit log (*Settings → Server*).

## Metrics

Prometheus metrics are served at `/metrics` on the HTTP port with `Authorization: Bearer $METRICS_TOKEN`, or without
authentication on a separate listener with `METRICS_ADDR=:9090` (keep it private). They include messages and bytes
received per inbox, SMTP rejections by reason, AUTH failures, webhook deliveries, chaos faults, spam checks, releases,
POP3 logins, HTTP status classes, plus gauges for stored messages/bytes per inbox and pending/failed jobs.
A ready-made Grafana dashboard is in [`docs/grafana-dashboard.json`](docs/grafana-dashboard.json).

## Scaling and performance

- Several PostDream replicas can share one database: live updates travel through Postgres `LISTEN/NOTIFY` and the
  background jobs (webhooks, spam analysis) are claimed with `FOR UPDATE SKIP LOCKED`, so put them behind a load
  balancer (SMTP and HTTP) as you like. A Helm chart lives in [`deploy/helm/postdream`](deploy/helm/postdream).
- Connection caps: `SMTP_MAX_CONNECTIONS` / `HTTP_MAX_CONNECTIONS` (1000 each by default).
- `backend/cmd/pdbench` is an SMTP load generator (`make bench`). On a laptop (Docker, Postgres 17) one instance
  ingests about **350 emails/s** of 5 KB and **150 emails/s** of 50 KB with 20 concurrent connections.

## Environment variables

| Variable                       | Default                    | Description                                                |
|--------------------------------|----------------------------|------------------------------------------------------------|
| `DATABASE_URL`                 | `postgres://postdream:…` | PostgreSQL connection string                               |
| `HTTP_ADDR`                    | `:8025`                    | UI/API listen address                                      |
| `SMTP_ADDR`                    | `:1025`                    | SMTP listen address                                        |
| `SMTP_HOSTNAME`                | `postdream`              | Name announced in the SMTP greeting                        |
| `SMTP_AUTH`                    | `optional`                 | `off` · `optional` · `required`                            |
| `SMTP_TLS_CERT`/`SMTP_TLS_KEY` | —                          | Enables STARTTLS with that certificate                     |
| `SMTP_MAX_MESSAGE_BYTES`       | `26214400`                 | Maximum size per email                                     |
| `SMTP_MAX_RECIPIENTS`          | `100`                      | Maximum recipients per email                               |
| `DEFAULT_INBOX`                | `default`                  | Slug of the inbox that receives anonymous mail             |
| `DEFAULT_INBOX_SMTP_PASSWORD`  | random                     | SMTP password of the default inbox when it's created       |
| `ACCESS_TOKEN`                 | —                          | With `AUTH_ENABLED=false`: one shared key for UI and API instead of user accounts |
| `AUTH_ENABLED`                 | `true`                     | Require login in the UI                                    |
| `ADMIN_EMAIL`                  | `admin@postdream.local`  | Initial admin (only with an empty database)                |
| `ADMIN_PASSWORD`               | random (logged)            | Initial admin password                                     |
| `SESSION_TTL`                  | `168h`                     | Session lifetime                                           |
| `COOKIE_SECURE`                | `false`                    | `true` when the UI is served over HTTPS                    |
| `TRUST_PROXY`                  | `false`                    | Use `X-Forwarded-For` for the client IP (rate limiting)    |
| `RETENTION_HOURS`              | `72`                       | How many hours emails are kept                             |
| `RETENTION_INTERVAL`           | `10m`                      | How often the cleanup runs                                 |
| `MAX_MESSAGES_PER_INBOX`       | `0`                        | Per-inbox message cap (0 = unlimited)                      |
| `LOG_LEVEL`                    | `info`                     | `debug` for more detail                                    |
| `SMTP_REQUIRE_TLS`             | `false`                    | Reject MAIL before STARTTLS (needs the TLS cert)           |
| `SMTP_ALLOWED_RECIPIENTS`      | —                          | Global regex every RCPT TO must match                      |
| `SMTP_MAX_CONNECTIONS` / `HTTP_MAX_CONNECTIONS` | `1000`    | Concurrent connection caps                                 |
| `ALLOWED_HOSTS`                | — (any)                    | Comma-separated Host headers accepted by the UI/API (DNS-rebinding protection) |
| `PUBLIC_URL`                   | —                          | Base URL used for links in webhook payloads                |
| `POP3_ADDR`                    | — (off)                    | POP3 listen address, e.g. `:1110`                          |
| `JOB_WORKERS`                  | `4`                        | Background workers (webhooks, spam analysis)               |
| `OUTBOUND_ALLOW_PRIVATE`       | `false`                    | Let webhooks/link checks reach private networks            |
| `OUTBOUND_ALLOWED_HOSTS`       | —                          | Hosts/CIDRs always reachable (`ci.internal,10.1.0.0/16`)   |
| `RSPAMD_URL` / `RSPAMD_PASSWORD` | —                        | Rspamd for spam scores, e.g. `http://rspamd:11333`         |
| `SPAMASSASSIN_ADDR`            | —                          | spamd `host:port` (used when Rspamd is not set)            |
| `RELAY_SMTP_URL`               | — (off)                    | SMTP server for releasing emails                           |
| `RELAY_FROM`                   | —                          | Envelope sender override for released emails               |
| `METRICS_TOKEN`                | —                          | Enables `/metrics` on the HTTP port behind this bearer token |
| `METRICS_ADDR`                 | —                          | Separate unauthenticated metrics listener, e.g. `:9090`    |

## Deploying to a server

Example on a VPS with Docker and a domain `mail.yourdomain.com`:

1. Clone the repo and create `.env`:
   ```env
   ADMIN_PASSWORD=a-long-password
   SMTP_AUTH=required
   COOKIE_SECURE=true
   TRUST_PROXY=true
   HTTP_PORT=127.0.0.1:8025   # only the HTTPS proxy exposes the UI
   POSTGRES_PASSWORD=another-long-password
   ```
2. `docker compose up -d --build`
3. Put an HTTPS reverse proxy in front of the UI. With [Caddy](https://caddyserver.com) (`/etc/caddy/Caddyfile`):
   ```caddyfile
   mail.yourdomain.com {
       reverse_proxy 127.0.0.1:8025 {
           flush_interval -1   # required for live updates (SSE)
       }
   }
   ```
   With nginx: `proxy_pass http://127.0.0.1:8025; proxy_buffering off;` and `proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;`.
4. Open the SMTP port (`1025`, or map it to `587` with `SMTP_PORT=587`) and 443 in the firewall. **Do not** expose Postgres.
5. (Optional) STARTTLS for SMTP: mount the certificate into the container and set `SMTP_TLS_CERT` and `SMTP_TLS_KEY`
   (add a `volumes:` entry to the `postdream` service in `docker-compose.yml`).
6. Sign in, create one inbox per project/environment and `member` users for each team.

**Docker image**: the GitHub Actions workflow (`.github/workflows/ci.yml`) runs the tests and publishes a multi-arch
(`linux/amd64`, `linux/arm64`) image to `ghcr.io/matiasrohleder/postdream` on every push to `main` and every `v*` tag.
On the server you can use that image instead of `build:`.

**Kubernetes**: `helm install postdream deploy/helm/postdream --set database.url=postgres://…` (see the chart's
`values.yaml`; Postgres is external, several replicas are supported).

**Testcontainers**: the standalone image needs no database container, e.g. with `testcontainers-node`:

```js
const pd = await new GenericContainer('ghcr.io/matiasrohleder/postdream:standalone')
  .withExposedPorts(1025, 8025).withWaitStrategy(Wait.forHttp('/api/health', 8025)).start()
```

**Backups**: data lives in the `pgdata` volume. `docker compose exec db pg_dump -U postdream postdream > backup.sql`.

## Development

```
backend/              Go (module github.com/matiasrohleder/PostDream)
  cmd/postdream/    main: config, migrations, bootstrap, servers
  cmd/pdbench/        SMTP load generator
  internal/smtpd/     SMTP server (emersion/go-smtp) + AUTH PLAIN/LOGIN, chaos, recipient rules
  internal/ingest/    the single pipeline every message goes through (SMTP and send API)
  internal/api/       JSON API, auth, SSE, OpenAPI spec, serves the embedded SPA
  internal/store/     PostgreSQL queries (pgx) and the search-syntax parser
  internal/db/        pool + embedded SQL migrations
  internal/events/    live updates (in-process + Postgres LISTEN/NOTIFY)
  internal/jobs/      durable background job queue (webhooks, spam analysis)
  internal/webhook/   signed webhook delivery
  internal/htmlcheck/ caniemail-based HTML/CSS compatibility
  internal/linkcheck/ link and image checker
  internal/deliverability/ DKIM and header checks
  internal/spam/      Rspamd / SpamAssassin clients
  internal/netguard/  SSRF-safe HTTP client for user-supplied URLs
  internal/pop3/      POP3 server
  internal/relay/     release to a real SMTP server
  internal/metrics/   Prometheus text exposition
  internal/mailparse/ MIME parsing (enmime)
  internal/retention/ periodic cleanup
  web/                embeds the compiled UI
frontend/             React 18 + Vite (no UI library)
  src/i18n/           translations (en, es, de)
scripts/              send-test-mail.py, update-caniemail.py
deploy/helm/          Helm chart
```

Run the backend from source (Go 1.26 + a Postgres; `docker-compose.dev.yml` starts one on `localhost:5433`):

```bash
make db-dev     # Postgres on localhost:5433
make dev-api    # API + SMTP on :8025 / :1025 (stop the Docker stack first: make down)
make dev-ui     # Vite on http://localhost:5173 with hot reload, proxying /api to :8025
```

VS Code launch/debug configurations live in `.vscode/` (git-ignored, local only).

Tests: `make test` (or `cd backend && go test ./...`). Fuzz targets: `go test -fuzz=FuzzParseSearch ./internal/store`,
`go test -fuzz=FuzzParse ./internal/mailparse`. Load test: `make bench`.

### Documentation site

The documentation lives in `website/` ([Docusaurus](https://docusaurus.io)): `cd website && npm ci && npm start`.
CI builds it on every push (broken links fail the build) and it is deployed from `main` on Railway. Screenshots are
generated from a seeded instance with `website/scripts/seed-demo.py` and `website/scripts/screenshots.py`.

### Adding a UI language

1. Copy `frontend/src/i18n/es.js` to `xx.js` and translate the values (keys are the English source strings).
2. Register it in `frontend/src/i18n/index.jsx` (`LANGUAGES` and `dictionaries`).

## Architecture

```
 your app ──SMTP:1025──▶ ┌───────────────────────────────┐        ┌──────────────┐
 mail client ─POP3:1110─▶│  postdream (Go), 1..N        │──pgx──▶│  PostgreSQL  │
 browser ───HTTP:8025──▶ │  SMTP · POP3 · API · SSE · UI │◀─NOTIFY─│ data + jobs  │
 E2E tests ─API token──▶ │  job workers · janitor        │        └──────────────┘
                         └──────┬───────────────┬────────┘
                    webhooks ◀──┘               └──▶ Rspamd / SpamAssassin · relay SMTP (optional)
```

Each Go process runs the SMTP and POP3 servers, the HTTP API (which also serves the React UI embedded in the binary),
the real-time event stream, the background job workers and the periodic cleanup. Every captured message goes through
one ingestion pipeline (parse → store → tags/rules → webhooks/spam jobs → live event). Migrations are applied
automatically on startup.

## Versioning

The project follows [Semantic Versioning](https://semver.org). Current version: **0.2.0**.
The version is injected at build time (`VERSION` build arg) and shown in the UI sidebar and `/api/health`.
