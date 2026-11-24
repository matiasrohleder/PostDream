---
title: "Retention"
description: "How and when PostDream deletes old emails automatically."
---

# Retention (automatic cleanup)

A sandbox fills up fast. A background **janitor** runs every `RETENTION_INTERVAL` (10 minutes by default) and deletes:

1. emails older than the retention: the inbox's own **Retention (hours)** if set, otherwise `RETENTION_HOURS`
   (**72 hours** by default);
2. when `MAX_MESSAGES_PER_INBOX` is greater than 0, the oldest emails beyond that number in each inbox;
3. expired sessions, old failed background jobs and stale link-check cache entries.

The current default is shown at the bottom of the sidebar and in **Settings → Server**; each inbox card shows its
own value.

| Variable | Default | Meaning |
|---|---|---|
| `RETENTION_HOURS` | `72` | Default age limit for every inbox |
| `RETENTION_INTERVAL` | `10m` | How often the janitor runs |
| `MAX_MESSAGES_PER_INBOX` | `0` | Per-inbox cap (0 = no cap) |

## Deleting by hand

- **Empty** in the mailbox deletes every email of the current inbox, or only the ones matching the active search.
- Select emails → **Delete**, or `Del` on the open email.
- From the API: `DELETE /api/messages?q=…` deletes everything that matches, which is a common first step of a test
  ([E2E testing](../integrations/e2e-testing.md)).
