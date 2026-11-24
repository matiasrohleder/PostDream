# PostDream – project guide

SMTP sandbox: a Go service captures every email sent to it (SMTP :1025, optional POP3 :1110), stores it in PostgreSQL
and serves a React UI + JSON API (HTTP :8025). Nothing is relayed unless an admin explicitly enables releasing
(`RELAY_SMTP_URL` + a per-inbox recipient regex). Current version: **0.2.0** (SemVer).

## Layout

- `backend/` – Go 1.26 module `github.com/matiasrohleder/PostDream`
  - `cmd/postdream/main.go` – config, migrations, bootstrap (first admin + `default` inbox), wires and starts
    SMTP/HTTP/POP3/metrics listeners, job workers and the janitor. `cmd/pdbench` – SMTP load generator
  - `internal/config` – all env vars (documented in README "Environment variables" and `.env.example`)
  - `internal/db` – pgx pool + embedded SQL migrations (`migrations/NNN_name.sql`, applied in order, advisory-locked)
  - `internal/store` – every SQL query lives here (users, sessions, inboxes + settings/chaos, messages, tags, rules,
    webhooks, jobs, link cache, relay log); `query.go` is the search-syntax parser → parameterized SQL
  - `internal/ingest` – the ONLY path for new messages (SMTP and `POST /api/send`): parse, auto-tags, insert, tag rules,
    enqueue webhook/spam jobs, publish event
  - `internal/smtpd` – go-smtp backend, AUTH PLAIN/LOGIN, routes mail to the inbox by SMTP credentials, chaos and
    recipient rules
  - `internal/api` – net/http handlers; `Routes()` in `api.go` is the single route table; `openapi.json` documents it
  - `internal/events` – SSE hub; broadcasts through Postgres LISTEN/NOTIFY so several replicas share live updates
  - `internal/jobs` – durable job queue (FOR UPDATE SKIP LOCKED) with retries/backoff
  - `internal/webhook`, `internal/htmlcheck` (vendored caniemail.json), `internal/linkcheck`, `internal/deliverability`,
    `internal/spam` (Rspamd/SpamAssassin), `internal/pop3`, `internal/relay`, `internal/metrics` (Prometheus text format)
  - `internal/netguard` – SSRF-safe HTTP client (blocks private IPs after DNS resolution)
  - `internal/mailparse` – enmime wrapper; must never fail hard and must strip NUL / invalid UTF-8
  - `internal/retention` – periodic purge (age, per-inbox cap, expired sessions, failed jobs, link cache)
  - `web/` – `go:embed` of the built UI (`web/dist`; only a placeholder `index.html` is committed)
- `frontend/` – React 18 + Vite, plain JS, no UI/component library, single `styles.css` with CSS variables
  - `src/i18n/` – `index.jsx` (provider, `useT`, `LanguageSelect`), `es.js`, `de.js`
  - `src/pages/` (Login, Mailbox, Settings, ApiDocs), `src/components/` (Filters = search box with autocomplete,
    Checks = analysis tab, InboxTools = webhooks/rules/settings/chaos modals, Tags), `src/lib/` (api, format,
    context: toasts/SSE/theme/notifications)
- `scripts/send-test-mail.py` – stdlib-only test sender (HTML + inline cid image + optional attachments);
  `scripts/update-caniemail.py` – refreshes the vendored caniemail data
- `docker-compose.yml` (full stack; `--profile spam` adds Rspamd), `docker-compose.dev.yml` (Postgres only on
  127.0.0.1:5433), multi-stage `Dockerfile` (default target = PostDream only; `--target standalone` = PostDream +
  embedded PostgreSQL via `deploy/standalone/entrypoint.sh`, no accounts), `deploy/helm/postdream` (Helm chart), `docs/grafana-dashboard.json`
- `website/` – Docusaurus documentation site (English, Markdown in `website/docs/`, sidebar in `sidebars.js`),
  served by Caddy (`website/Dockerfile`); `website/scripts/seed-demo.py` + `screenshots.py` regenerate the screenshots

## Commands

Go and Node may not be installed on the host; everything works through Docker.

```bash
make up                 # build + run full stack (UI http://localhost:8025, SMTP localhost:1025)
make down / make logs
make send               # 5 test emails
make test               # go test ./... inside golang:1.26-alpine
make bench              # SMTP load test against the running stack
# vet + gofmt check without local Go:
docker run --rm -v "$PWD/backend":/src -w /src golang:1.26-alpine sh -c 'test -z "$(gofmt -l .)" && go vet ./...'
# build the UI without local Node:
docker run --rm -u $(id -u):$(id -g) -e HOME=/tmp -v "$PWD/frontend":/app -w /app node:22-alpine sh -c 'npm ci && npm run build'
```

With local toolchains: `make db-dev`, `make dev-api`, `make dev-ui` (Vite on :5173 proxies `/api` to :8025).
Local `.env` (git-ignored) holds `ADMIN_PASSWORD`; default dev admin is `admin@postdream.local` / `admin12345`.

## Conventions

- **Language**: code, comments, docs, commit messages and API errors in English. UI text goes through i18n.
- **i18n**: keys are the English source strings: `const t = useT(); t('Delete {n} email(s)?', { n })`.
  Every new key must be added to **both** `src/i18n/es.js` and `src/i18n/de.js`. Dates/relative times use
  `getLocale()` via `lib/format.js`. Quick coverage check: grep all `t('…')` keys and diff against the dictionaries.
- **Theme**: `auto` (default) follows the OS via `prefers-color-scheme`; `light`/`dark` set `data-theme` on `<html>`.
  Add colors only as CSS variables in `:root` and both dark blocks of `styles.css`.
- **Multitenancy/authorization**: an inbox is the tenant. Always scope message queries through
  `principal.allowedInboxes()` / `canAccess()` in `internal/api`; API-token principals are limited to one inbox and
  must never see `smtp_password`/`api_token`. Admin-only routes use the `admin(...)` helper in `api.go`.
  Auth modes (`config.AuthMode()`): `users` (accounts), `token` (AUTH_ENABLED=false + ACCESS_TOKEN, one shared key for
  UI cookie and API bearer), `none` (open).
- **Security**: email HTML is only rendered in the UI via `iframe srcdoc` with `sandbox` (no `allow-scripts`);
  attachments are always served with `Content-Disposition: attachment` + CSP `sandbox`. Keep it that way.
- **New API routes**: add them to `Routes()` and to `internal/api/openapi.json` (OpenAPI 3.1);
  `TestOpenAPICoversRoutes` fails otherwise. New per-inbox features go through `canAccess`/`canManageInbox`.
- **New ways to receive mail** must call `ingest.Pipeline.Ingest` so tags, rules, webhooks and events stay consistent.
- **Outbound HTTP to user-supplied URLs** (webhooks, link check…) must use `netguard.Client` with the inbox's
  `allowed_hosts`; never `http.DefaultClient`.
- **Background work** that must survive restarts goes through `jobs.Runner` (register a handler in `main.go`).
- **Schema changes**: add a new `internal/db/migrations/00N_description.sql`; never edit an applied migration.
- **SQL**: parameterized queries only; user text in LIKE filters goes through `likeEscape`.
- Go code must pass `gofmt` and `go vet`; keep tests next to the package (`*_test.go`).
- **Docs travel with the code**: a user-visible change (feature, env var, endpoint, UI) updates the matching page in
  `website/docs/` (and its screenshots when the UI changes) in the same PR; `npm run build` in `website/` must pass.
- No AI/tool attribution anywhere (no "made with", no co-author trailers in commits or PRs).

## Verifying changes

1. `make test` and the gofmt/vet command above.
2. `docker compose up -d --build`, then `python3 scripts/send-test-mail.py -n 3` and check the UI.
3. UI: check the Analysis tab, Settings → Inboxes (Webhooks/Rules/Settings/Chaos) and Settings → API.
4. API smoke test: login with `curl -c ck -H 'Content-Type: application/json' -d '{"email":"admin@postdream.local","password":"admin12345"}' localhost:8025/api/auth/login`, then `curl -b ck localhost:8025/api/messages`.

## Release

Bump the version in `docker-compose.yml` (`VERSION` default), `Dockerfile` (`ARG VERSION`), `frontend/package.json`,
`backend/cmd/postdream/main.go` (`version` fallback), README "Versioning" and this file; tag `vX.Y.Z`.
CI (`.github/workflows/ci.yml`) tests and pushes `ghcr.io/matiasrohleder/postdream` and, when the `DOCKERHUB_USERNAME`/
`DOCKERHUB_TOKEN` secrets exist, Docker Hub; a `v*` tag also moves `latest` and creates the GitHub release. The standalone target is published
with `-standalone` suffixed tags plus `standalone` (newest release); CI smoke-tests it before publishing.
