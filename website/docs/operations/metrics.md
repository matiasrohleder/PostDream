---
title: "Metrics"
description: "Prometheus metrics and a ready-made Grafana dashboard."
---

# Metrics

PostDream exposes Prometheus metrics in the text format.

## Enabling them

| Option | How |
|---|---|
| On the HTTP port, protected | `METRICS_TOKEN=secret`, then scrape `/metrics` with `Authorization: Bearer secret` |
| Separate listener, no auth | `METRICS_ADDR=:9090`, then scrape `http://host:9090/metrics`. Keep that port private |

```yaml
scrape_configs:
  - job_name: postdream
    authorization: { credentials: secret }
    static_configs: [{ targets: ['postdream:8025'] }]
```

## What is measured

| Metric | Type | Labels / meaning |
|---|---|---|
| `postdream_messages_received_total` | counter | Messages captured, by inbox |
| `postdream_received_bytes_total` | counter | Bytes captured, by inbox |
| `postdream_smtp_rejected_total` | counter | SMTP transactions rejected, by reason |
| `postdream_smtp_auth_failures_total` | counter | Failed SMTP `AUTH` attempts |
| `postdream_pop3_logins_total` | counter | POP3 logins, by result |
| `postdream_webhook_deliveries_total` | counter | Webhook attempts, by result |
| `postdream_chaos_faults_total` | counter | Faults injected by chaos rules, by stage |
| `postdream_spam_checks_total` | counter | Spam analyses, by engine and result |
| `postdream_messages_released_total` | counter | Releases to a real SMTP server, by result |
| `postdream_retention_deleted_total` | counter | Messages deleted by the janitor |
| `postdream_http_requests_total` | counter | API requests, by status class |
| `postdream_messages_stored` | gauge | Messages stored, by inbox |
| `postdream_messages_unread` | gauge | Unread messages, by inbox |
| `postdream_stored_bytes` | gauge | Bytes stored, by inbox |
| `postdream_jobs_pending` / `postdream_jobs_failed` | gauge | Background jobs waiting or retrying / that gave up |
| `postdream_database_up` | gauge | 1 when the database answers |

## Grafana

Import `docs/grafana-dashboard.json` from the repository for a dashboard with throughput per inbox, rejections,
webhook health, storage and the job queue.
