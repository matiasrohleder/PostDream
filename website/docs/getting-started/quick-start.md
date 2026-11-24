---
title: "Quick start"
description: "Run PostDream with Docker Compose and catch your first email in five minutes."
---

# Quick start

All you need is **Docker** with Docker Compose. Go and Node are not required.

## 1. Start the stack

You do not need to clone anything. Save this as `docker-compose.yml`:

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

and run:

```bash
docker compose up -d
```

This starts PostgreSQL and PostDream. Migrations run automatically on startup.

| Service | Address |
|---|---|
| Web UI and API | http://localhost:8025 |
| SMTP | `localhost:1025` |

:::tip From source
To build it yourself (and get POP3 on `1110` and the `make` shortcuts), clone the repository instead:

```bash
git clone https://github.com/matiasrohleder/PostDream.git
cd PostDream
cp .env.example .env          # set ADMIN_PASSWORD
docker compose up -d --build
```
:::

## 2. Sign in

Open http://localhost:8025 and sign in with `ADMIN_EMAIL` / `ADMIN_PASSWORD` (the email defaults to
`admin@postdream.local`).

![Login page](/img/screenshots/login.png)

:::tip No password set?
If `ADMIN_PASSWORD` is empty, a random password is generated and printed once in the logs:
`docker compose logs postdream`. The admin user is only created on the very first start (empty database);
change the password later in **Settings → My account**.
:::

## 3. Send an email

```bash
# with the repository cloned
python3 scripts/send-test-mail.py -n 3
# or with any SMTP client, for example swaks
swaks --server localhost:1025 --to user@example.com --header "Subject: Hello"
```

The three emails show up in the grid **immediately**, without refreshing: the UI keeps a live connection
(the green **Live** dot in the top bar).

![Emails arriving in the grid](/img/screenshots/mailbox.png)

## 4. Open one

Click a row to open the message on the right: HTML preview, text, headers, raw source, SMTP envelope data
and the **Analysis** tab.

![Message view](/img/screenshots/message.png)

## Useful commands

| Command | What it does |
|---|---|
| `make up` / `make down` | Build and start / stop the stack (data stays in the `pgdata` volume) |
| `make logs` | Follow PostDream's logs |
| `make send` | Send 5 test emails |
| `make test` | Run the Go tests inside Docker |
| `make bench` | SMTP load test against the running stack |
| `make psql` | Open a Postgres shell |

## Next steps

- [Send test emails](sending-test-emails.md) with the script, `swaks` or the API.
- [Connect your app](connect-your-app.md) with ready-made snippets for Node, Python, .NET and Laravel.
