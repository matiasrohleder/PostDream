---
title: "Message view"
description: "Everything you can see and do with a single captured email."
---

# Message view

Click an email in the grid to open it next to the list.

![Message view](/img/screenshots/message.png)

## Header

Subject, From, To/Cc, date, size, inbox and **tags** (add one with **+ Tag**, remove one with its ×).

Actions in the toolbar:

| Button | Action |
|---|---|
| ✈ **Release** | Send this email to real recipients ([only if enabled](release.md)) |
| ⬇ **.eml** | Download the original message |
| ✉ **Mark as unread** | Put it back as unread |
| 🗑 **Delete** | Delete it (`Del`) |

Opening an email marks it as read. API clients can read without marking with `?peek=1`.

## Tabs

### HTML

The HTML part, with `cid:` inline images resolved. It is rendered in a **sandboxed iframe with scripts disabled**,
so a malicious email cannot run code in your browser. Switch the preview width between **desktop**, **tablet** and
**phone** with the icons on the right.

### Text

The plain-text part, exactly as sent. Useful to check that your text alternative is not empty or broken.

![Text tab](/img/screenshots/message-text.png)

### Headers

Every header of the message, in order.

![Headers tab](/img/screenshots/message-headers.png)

### Raw

The complete RFC 5322 source.

### SMTP

What happened at the SMTP level, which is often different from the headers: **MAIL FROM**, every **RCPT TO**
(including Bcc recipients that are not in any header), the **client IP** and the **SMTP user** that authenticated.

![SMTP envelope tab](/img/screenshots/message-smtp.png)

### Analysis

HTML/CSS compatibility across email clients, deliverability checks, spam score and a link checker.
See [Email analysis](analysis.md).

## Attachments

Attachments are listed under the message and are **always downloaded**, never rendered inside PostDream's page,
so a hostile attachment cannot run in the app's origin.
