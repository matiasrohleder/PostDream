---
title: "Tags and auto-tag rules"
description: "Label emails by hand, from plus-addresses, from an X-Tags header, or automatically with rules."
---

# Tags and auto-tag rules

Tags are short labels shown as colored chips in the grid and the message view. Use them to group emails by flow
(`signup`, `password-reset`, `invoice`), by test run, or anything else. Search them with `tag:name`.

## Five ways to tag an email

| How | Example |
|---|---|
| **By hand** | **+ Tag** in the message view |
| **In bulk** | Select emails in the grid → **Tag** → add or remove tags |
| **Plus-addressing** | An email to `qa+signup@acme.test` gets the tag `signup` |
| **`X-Tags` header** | `X-Tags: onboarding, beta` adds `onboarding` and `beta` |
| **Auto-tag rules** | Every new email matching a search query gets a tag |

The API can also set tags: `PUT /api/messages/{id}/tags`, `POST /api/messages/tags` (bulk), or `tags` in
`POST /api/send`.

## Auto-tag rules

**Settings → Inboxes → Rules** (admins). A rule is a [search query](search.md) plus a tag. Every new email that
matches the query gets the tag as soon as it is received, before webhooks fire, so webhook payloads already include it.

![Auto-tag rules](/img/screenshots/rules.png)

| Query | Tag |
|---|---|
| `subject:"reset your password"` | `password-reset` |
| `subject:order` | `orders` |
| `has:attachment from:billing` | `invoice` |
| `header:X-Priority=1` | `urgent` |

A rule applies to one inbox, or to **all inboxes** when the checkbox is ticked. The 🔍 button next to a rule shows
the emails that match it right now. Rules only tag **new** emails; to tag existing ones, search with the same query,
select the results and use bulk **Tag**.

## Tag colors

Each tag always gets the same color (derived from its name), so `signup` looks the same everywhere.
