---
title: "Sending test emails"
description: "Three ways to put emails into PostDream - the bundled script, any SMTP tool, or the send API."
---

# Sending test emails

## The bundled script

`scripts/send-test-mail.py` needs only the Python standard library. It sends realistic emails with an HTML part,
a text part, an inline `cid:` image and optional attachments.

```bash
python3 scripts/send-test-mail.py                     # 1 anonymous email → "default" inbox
python3 scripts/send-test-mail.py -n 20               # 20 emails
python3 scripts/send-test-mail.py --user acme-staging --password XXXX --to qa@acme.test
python3 scripts/send-test-mail.py --attach invoice.pdf --subject "Your invoice"
python3 scripts/send-test-mail.py --host mail.example.com --port 1025 --starttls
```

| Option | Meaning |
|---|---|
| `--host`, `--port` | SMTP server (default `localhost:1025`) |
| `--user`, `--password` | Inbox SMTP credentials; the email lands in that inbox |
| `--starttls` | Upgrade the connection with STARTTLS |
| `-n`, `--count` | How many emails to send |
| `--sender`, `--to`, `--subject` | Override the generated values |
| `--attach FILE` | Attach a file (repeatable) |
| `--text-only` | No HTML part |

> **📝 Note**
>
> The HTML of the test email contains a `<script>` tag on purpose. PostDream renders email HTML in a sandboxed iframe
> with scripts disabled, so it never runs.

## Any SMTP client

PostDream is a regular SMTP server, so any tool works. With [swaks](https://github.com/jetmore/swaks):

```bash
swaks --server localhost:1025 --to user@example.com --header "Subject: Hello"
swaks --server localhost:1025 --auth-user acme-staging --auth-password XXXX --to user@example.com
```

## Without SMTP: the send API

`POST /api/send` injects a message through the same pipeline as SMTP (tags, rules, webhooks and live updates all
apply). Handy when SMTP is not reachable, for example from a browser-based test.

```bash
curl -X POST http://localhost:8025/api/send \
  -H "Authorization: Bearer $INBOX_TOKEN" -H 'Content-Type: application/json' \
  -d '{"from":"app@example.com","to":["user@example.com"],"subject":"Hi","html":"<b>Hi</b>","tags":["demo"]}'
```

The body also accepts `cc`, `bcc`, `reply_to`, `text`, `headers`, `attachments` (base64) or a whole raw `.eml` in
`raw`. See the [REST API](../integrations/rest-api.md#send-an-email-without-smtp).

## Load testing

`backend/cmd/pdbench` is an SMTP load generator:

```bash
make bench   # 2000 emails of 5 KB over 20 connections against the running stack
```
