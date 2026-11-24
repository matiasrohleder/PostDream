---
title: "Search syntax"
description: "Find any email with filters such as from:, tag:, is:unread, has:attachment, larger:, after: and header:."
---

# Search syntax

The search box above the grid, and the `q` parameter of the API, accept free words, `"quoted phrases"` and
**filters**. Terms are combined with AND. Prefix any term with `-` or `!` to negate it.

![Search with autocomplete](/img/screenshots/search.png)

The box **autocompletes** filter names, tags and inboxes as you type, remembers your recent searches, and the
**?** button opens a cheat sheet. Clicking a tag in the grid searches for it.

![Search help](/img/screenshots/search-help.png)

## Filters

| Filter | Meaning |
|---|---|
| `from:` `to:` `cc:` `bcc:` `reply-to:` `addressed:` | Address fields. `to:` also matches envelope recipients; `bcc:` matches envelope recipients that are not in To/Cc; `addressed:` matches any of them |
| `subject:` `message-id:` | Header contains |
| `tag:name` | Has the tag (case-insensitive) |
| `is:read` `is:unread` `is:tagged` `is:spam` | Status |
| `has:attachment` `has:inline` `has:html` | Content |
| `larger:2M` `smaller:500K` | Size |
| `after:2026/11/01` `before:2026-11-15` | Received before/after a date |
| `after:2h` `before:3d` | Received within an age: `m`, `h`, `d`, `w` |
| `inbox:slug` | In that inbox |
| `smtp-user:name` `ip:10.0.` | Who sent it: SMTP user, client IP prefix |
| `header:X-Env=staging` | Header contains a value |
| `header:List-Unsubscribe` | Header exists |
| `spam:>5` `spam:<1` | Spam score (needs a [spam engine](analysis.md#spam-score)) |

## Examples

| Query | Finds |
|---|---|
| `from:shop "order #" -is:read` | Unread emails from the shop with "order #" anywhere |
| `tag:signup after:2h` | Sign-up emails of the last two hours |
| `has:attachment larger:1M -inbox:default` | Big attachments outside the default inbox |
| `to:qa@acme.test subject:"reset your password"` | The password-reset email of one test user |
| `header:X-Env=staging spam:>5` | Staging emails that look like spam |

## Classic filters

The **Filters** button opens a form with From, To/Cc, Subject, a date range, with/without attachments and quick
presets (**Last hour**, **Today**, **3 days**). The **All / Unread / Read** selector next to the search box filters by
status. Everything combines with the search query.

## In the API

```bash
curl -G -H "Authorization: Bearer $TOKEN" http://localhost:8025/api/messages \
  --data-urlencode 'q=to:user@example.com subject:"reset your password" after:10m'
```

The same `q` works for `GET /api/messages`, `GET /api/messages/latest`, `DELETE /api/messages` and in
[auto-tag rules](tags-and-rules.md#auto-tag-rules).
