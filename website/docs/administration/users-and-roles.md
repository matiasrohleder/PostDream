---
title: "Users and roles"
description: "Admins manage everything; members only see the inboxes assigned to them."
---

# Users and roles

People sign in to the UI with a **user** (email + password). There are two roles:

| Role | Can do |
|---|---|
| **admin** | See every inbox; create, edit and delete inboxes, users, webhooks, rules and chaos; regenerate credentials; read the release audit log |
| **member** | See only the inboxes assigned to them: read, search, tag, delete and empty their emails, see their credentials and configuration snippets |

API tokens are not users: a token belongs to an inbox and only reaches that inbox's emails
([REST API](../integrations/rest-api.md#authentication)).

## Managing users

**Settings → Users** (admins only) lists every user with role, visible inboxes and last login.

![Users](/img/screenshots/users.png)

- **New user**: email, name, password (at least 8 characters), role and, for members, the inboxes they can see.
- **Edit**: change any of those. Leave the password empty to keep it.
- **Delete**: removes the user and their sessions.

## The first admin

On first start (empty database) PostDream creates an admin from `ADMIN_EMAIL` and `ADMIN_PASSWORD`. If the password
is empty, a random one is printed in the logs. These variables are ignored afterwards; change the password in
**Settings → My account**.

## Running without login

`AUTH_ENABLED=false` disables the UI login entirely: everyone who can reach the UI is an admin. Only do this for a
100 % local setup or a throwaway container in tests (see [Testcontainers](../operations/deployment.md#testcontainers)).
