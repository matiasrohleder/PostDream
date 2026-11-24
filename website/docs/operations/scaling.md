---
title: "Scaling and performance"
description: "Run several replicas on one database; connection caps and benchmark numbers."
---

# Scaling and performance

## Several replicas

PostDream instances are stateless; everything lives in PostgreSQL. You can run as many replicas as you like against
one database and put them behind a load balancer for both SMTP and HTTP:

- **Live updates** travel between replicas through Postgres `LISTEN/NOTIFY`, so a browser connected to replica A sees
  an email received by replica B.
- **Background jobs** (webhooks, spam analysis) are stored in a table and claimed with `FOR UPDATE SKIP LOCKED`, so
  each job runs once, on whichever replica is free, and survives restarts.
- **Migrations** are applied under an advisory lock, so replicas starting together do not race.

The [Helm chart](deployment.md#kubernetes-helm) supports `replicaCount` out of the box.

## Limits

| Variable | Default | Purpose |
|---|---|---|
| `SMTP_MAX_CONNECTIONS` | `1000` | Concurrent SMTP connections per replica |
| `HTTP_MAX_CONNECTIONS` | `1000` | Concurrent HTTP connections per replica (SSE clients included) |
| `SMTP_MAX_MESSAGE_BYTES` | 25 MB | Size per email |
| `JOB_WORKERS` | `4` | Background workers per replica |

## Benchmarks

`backend/cmd/pdbench` is an SMTP load generator (`make bench`). On a laptop (Docker, Postgres 17), one instance
ingests about **350 emails/s** of 5 KB and **150 emails/s** of 50 KB with 20 concurrent connections.

```bash
go run ./cmd/pdbench -addr localhost:1025 -n 2000 -c 20 -size 5000
```
