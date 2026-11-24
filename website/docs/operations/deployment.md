---
title: "Deployment"
description: "Run PostDream on a VPS with Docker Compose, from the published image, on Kubernetes or inside tests."
---

# Deployment

PostDream needs two things: the PostDream process (one binary, or the Docker image) and a PostgreSQL database.
Migrations run automatically on startup.

| Option | Good for |
|---|---|
| [Docker Compose on a VPS](#vps-with-docker-compose) | A team server with a domain |
| [Railway](railway.md) | Managed hosting with auto-deploy from GitHub |
| [Kubernetes (Helm)](#kubernetes-helm) | Clusters, several replicas |
| [Testcontainers](#testcontainers) | Throwaway instance per test run |

## VPS with Docker Compose

Example for `mail.yourdomain.com`:

1. Clone the repository and create `.env`:

   ```bash
   ADMIN_PASSWORD=a-long-password
   SMTP_AUTH=required
   COOKIE_SECURE=true
   TRUST_PROXY=true
   HTTP_PORT=127.0.0.1:8025   # only the HTTPS proxy exposes the UI
   POSTGRES_PASSWORD=another-long-password
   ```

2. `docker compose up -d --build`
3. Put an HTTPS reverse proxy in front of the UI. With [Caddy](https://caddyserver.com):

   ```
   mail.yourdomain.com {
       reverse_proxy 127.0.0.1:8025 {
           flush_interval -1   # required for live updates (SSE)
       }
   }
   ```

   With nginx:

   ```nginx
   location / {
       proxy_pass http://127.0.0.1:8025;
       proxy_buffering off;   # required for live updates (SSE)
       proxy_set_header Host $host;
       proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
   }
   ```

4. Open the SMTP port (`1025`, or map it to `587` with `SMTP_PORT=587`) and `443` in the firewall.
   **Never expose PostgreSQL.**
5. Optional STARTTLS for SMTP: mount a certificate into the container and set `SMTP_TLS_CERT` / `SMTP_TLS_KEY`.
6. Sign in, create one inbox per project or environment and `member` users for each team.

:::warning Live updates behind a proxy
Server-Sent Events need response buffering turned off in the proxy (`flush_interval -1` in Caddy,
`proxy_buffering off` in nginx). Otherwise new emails only show up after a refresh.
:::

## Docker image

CI publishes a multi-arch image (`linux/amd64`, `linux/arm64`) on every push to `main` and every `v*` tag, to the
GitHub Container Registry and to Docker Hub:

```bash
docker pull ghcr.io/matiasrohleder/postdream:latest
docker pull docker.io/matiasrohleder/postdream:latest
```

| Tag | Points to |
|---|---|
| `latest` | The newest release |
| `0.1.0`, `0.1` | A release / the newest patch of a minor version |
| `main` | The tip of the `main` branch (unreleased) |
| `sha-<commit>` | One exact commit |

Use it instead of `build:` in Compose, or anywhere that runs containers. It exposes `1025` (SMTP) and `8025` (HTTP)
and has a built-in health check on `/api/health`.

## Kubernetes (Helm)

```bash
helm install postdream deploy/helm/postdream \
  --set database.url='postgres://user:pass@postgres:5432/postdream?sslmode=disable'
```

The chart lives in `deploy/helm/postdream`; see its `values.yaml` for `replicaCount`, `env`, `secretEnv`, the
service ports, ingress and resources. PostgreSQL is external. Several replicas are supported
([Scaling](scaling.md)).

## Testcontainers

Start a disposable PostDream next to a Postgres container in your test setup, for example with `testcontainers-node`:

```js
const pg = await new PostgreSqlContainer('postgres:17-alpine').withNetwork(net).withNetworkAliases('db').start()
const pd = await new GenericContainer('ghcr.io/matiasrohleder/postdream:0.1.0').withNetwork(net)
  .withEnvironment({
    DATABASE_URL: `postgres://${pg.getUsername()}:${pg.getPassword()}@db:5432/${pg.getDatabase()}?sslmode=disable`,
    AUTH_ENABLED: 'false',
  })
  .withExposedPorts(1025, 8025)
  .withWaitStrategy(Wait.forHttp('/api/health', 8025))
  .start()
```

## Backups

All data lives in PostgreSQL (the `pgdata` volume with Compose):

```bash
docker compose exec db pg_dump -U postdream postdream > backup.sql
```
