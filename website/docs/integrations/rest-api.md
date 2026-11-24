---
title: "REST API"
description: "Authentication, the most used endpoints, filters, long-polling and the send API. Full reference in OpenAPI 3.1."
---

# REST API

Everything the UI does goes through a JSON API under `/api`. The full, always up-to-date reference is the
**OpenAPI 3.1** document served by every instance at `/api/openapi.json`, also browsable in the UI under
**Settings → API**. The build fails if a route is missing from it, so it never drifts.

## Authentication

| Method | Who uses it | Scope |
|---|---|---|
| **Session cookie** | The UI, after `POST /api/auth/login` | Whatever the user can see (admin: everything) |
| **`Authorization: Bearer <inbox api_token>`** | Tests, scripts, CI | **Only that inbox**. Never returns SMTP passwords or tokens |

Find an inbox's token on its card in **Settings → Inboxes**. Regenerating credentials changes it.

```bash
TOKEN=pd_xxxxxxxxxxxxxxxx
curl -H "Authorization: Bearer $TOKEN" http://localhost:8025/api/messages
```

## Most used endpoints

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/messages?q=…` | Paginated list with filters or [search syntax](../features/search.md) |
| `GET` | `/api/messages/latest?wait=30s` | Newest matching email; **waits** for it (up to 2 minutes) |
| `GET` | `/api/messages/{id}` | Full message (marks it read; `?peek=1` does not) |
| `GET` | `/api/messages/{id}/html` · `/raw` · `/attachments/{aid}` | HTML with `cid:` inlined, RFC 5322 source, attachment |
| `GET` | `/api/messages/{id}/html-check` · `/link-check` · `/deliverability` · `/spam` | [Analysis](../features/analysis.md) |
| `PUT` | `/api/messages/{id}/tags` | Replace tags: `{"tags": ["a","b"]}` |
| `POST` | `/api/messages/tags` | Bulk add/remove: `{"ids": […], "add": […], "remove": […]}` |
| `POST` | `/api/send` | Inject an email without SMTP |
| `DELETE` | `/api/messages?…` | Delete everything that matches the filters |
| `POST` | `/api/messages/delete` · `/api/messages/read` | Bulk delete / mark read or unread |
| `PUT` | `/api/inboxes/{id}/chaos` | [Chaos rules](chaos-testing.md) (admins or the inbox token) |
| `GET` | `/api/events` | Live updates (Server-Sent Events) |
| `GET` | `/api/health` | Health check and version (public) |
| `*` | `/api/inboxes…`, `/api/webhooks…`, `/api/rules`, `/api/users…` | Administration (admin session) |

## Filters

Besides `q`, list endpoints accept:

| Parameter | Values |
|---|---|
| `inbox` | Inbox id |
| `from`, `to`, `subject` | Contains |
| `since`, `until` | RFC 3339 or `YYYY-MM-DD` |
| `has_attachments`, `unread` | `true` / `false` |
| `sort` | `received_at`, `subject`, `from`, `to`, `size`, `inbox` |
| `order` | `asc`, `desc` |
| `page`, `page_size` | `page_size` up to 500 |

## Waiting for an email

`GET /api/messages/latest` returns the newest email matching the filters. With `wait`, the request stays open until
one arrives or the time runs out (then `404`). That is all an end-to-end test needs:

```bash
curl -G -H "Authorization: Bearer $TOKEN" http://localhost:8025/api/messages/latest \
  --data-urlencode 'q=to:user@example.com subject:"reset your password"' --data-urlencode wait=30s \
  | jq -r .text_body
```

More in [E2E testing](e2e-testing.md).

## Send an email without SMTP

```bash
curl -X POST http://localhost:8025/api/send \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' -d '{
    "from": "App <app@example.com>",
    "to": ["user@example.com"],
    "subject": "Hello",
    "html": "<b>It works!</b>",
    "text": "It works!",
    "headers": {"X-Env": "staging"},
    "tags": ["demo"],
    "attachments": [{"filename": "hello.txt", "content_type": "text/plain", "content": "SGVsbG8="}]
  }'
```

With a token the email goes to the token's inbox; with a session pass `inbox` (id or slug). Instead of fields you can
send a complete message in `raw`. It goes through the same pipeline as SMTP: tags, rules, webhooks and live updates.

## Live events

`GET /api/events` is a Server-Sent Events stream: the UI uses it to show new emails instantly. You can consume it too
(for example with `EventSource` in a browser test) to react the moment an email arrives.
