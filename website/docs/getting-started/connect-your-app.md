---
title: "Connect your app"
description: "SMTP settings and copy-paste snippets for Node.js, Python, .NET, Laravel and more."
---

# Connect your app

Your application does not need to know about PostDream: it sends email over SMTP as usual, only to a different
server.

## SMTP settings

| Setting | Value |
|---|---|
| Host | `localhost`, the server's domain, or `postdream` from another container on the same Docker network |
| Port | `1025` (or whatever you published) |
| Security | None, or STARTTLS if the server has a certificate |
| Username | The inbox's SMTP username (for example `acme-staging`) |
| Password | The inbox's SMTP password |

The credentials decide **which inbox** the email lands in. Without credentials it lands in the `default` inbox,
or is rejected if the server runs with `SMTP_AUTH=required`
(see [Security](../administration/security.md#smtp-authentication)).

## Ready-made snippets

Every inbox has a **Show configuration examples** button (**Settings → Inboxes**) with snippets that already contain
that inbox's host, port and credentials: `.env`, Node.js (nodemailer), Python, .NET, Laravel, swaks, the REST API,
a Playwright test, the send API, POP3 and a webhook signature verifier.

![Configuration examples for an inbox](/img/screenshots/setup-snippets.png)

## Examples

### Environment variables

```bash
SMTP_HOST=localhost
SMTP_PORT=1025
SMTP_USER=acme-staging
SMTP_PASSWORD=your-inbox-password
SMTP_SECURE=false
```

### Node.js (nodemailer)

```js
import nodemailer from 'nodemailer'

const transport = nodemailer.createTransport({
  host: process.env.SMTP_HOST, port: 1025, secure: false,
  auth: { user: process.env.SMTP_USER, pass: process.env.SMTP_PASSWORD },
})
await transport.sendMail({ from: 'app@example.com', to: 'user@example.com', subject: 'Hi', html: '<b>Hi</b>' })
```

### Python

```python
import smtplib
from email.message import EmailMessage

msg = EmailMessage()
msg["From"], msg["To"], msg["Subject"] = "app@example.com", "user@example.com", "Hi"
msg.set_content("Hello!")
with smtplib.SMTP("localhost", 1025) as s:
    s.login("acme-staging", "your-inbox-password")
    s.send_message(msg)
```

### .NET (MailKit)

```csharp
using var client = new SmtpClient();
await client.ConnectAsync("localhost", 1025, SecureSocketOptions.None);
await client.AuthenticateAsync("acme-staging", "your-inbox-password");
await client.SendAsync(message);
await client.DisconnectAsync(true);
```

### Laravel

```bash
MAIL_MAILER=smtp
MAIL_HOST=localhost
MAIL_PORT=1025
MAIL_USERNAME=acme-staging
MAIL_PASSWORD=your-inbox-password
MAIL_ENCRYPTION=null
```

## Tips

- **One inbox per project and environment** (`shop-staging`, `shop-qa`) keeps things tidy and lets you give each team
  access to only its own emails. See [Inboxes](../administration/inboxes.md).
- **Tag emails from your app** with plus-addressing (`qa+signup@acme.test` is tagged `signup`) or an
  `X-Tags: signup, onboarding` header. See [Tags and rules](../features/tags-and-rules.md).
- If PostDream runs on a public server, use `SMTP_AUTH=required` so nobody else can fill your sandbox.
