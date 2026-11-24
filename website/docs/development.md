---
title: "Development"
description: "Run PostDream from source, run the tests, add a UI language, and work on this documentation."
---

# Development

## Running from source

You need Go 1.26, Node 22 and a PostgreSQL. `docker-compose.dev.yml` starts one on `localhost:5433`.

```bash
make db-dev     # Postgres on localhost:5433
make dev-api    # API + SMTP on :8025 / :1025 (stop the Docker stack first: make down)
make dev-ui     # Vite on http://localhost:5173 with hot reload, proxying /api to :8025
```

The dev admin is `admin@postdream.local` / `admin12345`.

## Tests

```bash
make test                                       # go test ./... inside Docker
cd backend && go test ./...                     # with a local Go
go test -fuzz=FuzzParseSearch ./internal/store  # fuzz the search parser
go test -fuzz=FuzzParse ./internal/mailparse    # fuzz MIME parsing
make bench                                      # SMTP load test
```

CI runs vet, the race detector, govulncheck and the UI build on every push and pull request, then publishes the
Docker image from `main` and tags.

## Conventions worth knowing

- New API routes go into `Routes()` **and** `internal/api/openapi.json`; a test fails otherwise.
- Every new way of receiving mail must go through `ingest.Pipeline.Ingest`.
- Outbound HTTP to user-supplied URLs must use `netguard.Client`.
- Schema changes are new numbered migrations; applied migrations are never edited.
- UI text goes through i18n: keys are the English strings, and every key is added to `es.js` and `de.js`.

## Adding a UI language

1. Copy `frontend/src/i18n/es.js` to `xx.js` and translate the values (keys are the English source strings).
2. Register it in `frontend/src/i18n/index.jsx` (`LANGUAGES` and `dictionaries`).

## This documentation

The site lives in `website/` and is built with [Docusaurus](https://docusaurus.io).

```bash
cd website
npm ci
npm start        # http://localhost:3000 with hot reload
npm run build    # static site in website/build
```

Pages are Markdown files in `website/docs/`; the sidebar is `website/sidebars.js`.

### Regenerating the screenshots

The screenshots are generated from a real instance filled with demo data, so they are easy to refresh after a UI
change:

```bash
# 1. Start an EMPTY PostDream (never production) with ADMIN_PASSWORD=admin12345,
#    RELAY_SMTP_URL set (for the release dialog) and METRICS_TOKEN set.
# 2. Fill it with inboxes, users, rules, webhooks, chaos rules and emails
python3 website/scripts/seed-demo.py --url http://localhost:8025
# 3. Capture every screenshot into website/static/img/screenshots/
pip install playwright && playwright install chromium
python3 website/scripts/screenshots.py --url http://localhost:8025
```

`--only mailbox analysis` retakes just some of them.
