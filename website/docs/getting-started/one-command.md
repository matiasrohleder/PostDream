---
title: "Run it in one command"
description: "The standalone image: PostDream and its own database in a single container, with no login and no setup."
---

# Run it in one command

The **standalone** image bundles PostDream with its own PostgreSQL. There is nothing to configure, no database to
run and no admin user to create:

```bash
docker run --rm -p 8025:8025 -p 1025:1025 matiasrohleder/postdream:standalone
```

- **UI and API:** http://localhost:8025, open right away, no login.
- **SMTP:** `localhost:1025`, no credentials needed. Point your app there and every email shows up in the UI.

![The login-free mailbox](/img/screenshots/mailbox.png)

It is the fastest way to try PostDream, to run it next to your app while you develop, or to start a throwaway
instance in CI. The image is also on GHCR as `ghcr.io/matiasrohleder/postdream:standalone`.

## Keep your emails

Without a volume the container is disposable: removing it deletes every email. To keep them, mount a volume on
`/data`:

```bash
docker run -d --name postdream -p 8025:8025 -p 1025:1025 -v postdream:/data matiasrohleder/postdream:standalone
```

## Protect it with a key

The standalone image has no login by default, so anyone who can reach port 8025 can read the emails. That is fine on
your own machine. Anywhere else, set an **access token**:

```bash
docker run -d -p 8025:8025 -p 1025:1025 -e ACCESS_TOKEN=a-long-random-key matiasrohleder/postdream:standalone
```

- The UI asks only for that key: no email, no user accounts.
- The API accepts it as a bearer token, with full access:

  ```bash
  curl -H "Authorization: Bearer a-long-random-key" http://localhost:8025/api/messages
  ```

The key must have at least 8 characters. Changing it signs everybody out.

![Sign in with the access token](/img/screenshots/login-token.png)

## In Docker Compose and CI

Next to your app in `docker-compose.yml`:

```yaml
services:
  postdream:
    image: matiasrohleder/postdream:standalone
    ports: ["8025:8025"]
  app:
    build: .
    environment:
      SMTP_HOST: postdream
      SMTP_PORT: "1025"
```

As a GitHub Actions service for end-to-end tests:

```yaml
jobs:
  e2e:
    runs-on: ubuntu-latest
    services:
      postdream:
        image: matiasrohleder/postdream:standalone
        ports: ["8025:8025", "1025:1025"]
    steps:
      - uses: actions/checkout@v4
      - run: npm test   # SMTP on localhost:1025, API on http://localhost:8025/api
```

Then wait for emails with `GET /api/messages/latest?wait=30s`, as described in [E2E testing](../integrations/e2e-testing.md).

## What is different from the regular image

| | `standalone` | `latest` (regular) |
|---|---|---|
| Database | Built in, private to the container | Your own PostgreSQL (`DATABASE_URL`) |
| Login | None, or one `ACCESS_TOKEN` | User accounts with admin and member roles |
| SMTP authentication | Optional: everything lands in the `default` inbox unless credentials are used | Optional or required |
| POP3 | On, port `1110` | Off unless `POP3_ADDR` is set |
| Replicas | One | As many as you like |
| Made for | Your laptop, tests, CI | A shared server for a team |

Every other feature works the same: inboxes, search, tags, analysis, webhooks, chaos testing and the full
[REST API](../integrations/rest-api.md). All [configuration variables](../operations/configuration.md) apply too; for
example `-e SMTP_AUTH=required` or `-e RETENTION_HOURS=24`.

> **📝 Note: Using your own database**
>
> If `DATABASE_URL` is set, the standalone image skips its built-in PostgreSQL and connects to yours.

## Tags

| Tag | Points to |
|---|---|
| `standalone` | The newest release |
| `0.2.0-standalone`, `0.2-standalone` | A release / the newest patch of a minor version |
| `main-standalone` | The tip of the `main` branch (unreleased) |
