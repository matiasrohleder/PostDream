---
title: "Email analysis"
description: "Check email client compatibility, deliverability, spam score and broken links before your users do."
---

# Email analysis

The **Analysis** tab of any message answers the questions you would otherwise test by hand in a dozen email clients.
It has four sections.

![Email client compatibility](/img/screenshots/analysis.png)

## Email client compatibility

Every HTML element and CSS property the email uses is checked against the [caniemail.com](https://www.caniemail.com)
dataset (vendored in the repository, MIT licensed). You get:

- an overall **support score** and a bar per client (Gmail, Outlook, Apple Mail, Yahoo, Samsung Email, Thunderbird…);
- every **feature** detected, with the clients that do not support it or support it only partially;
- **Only problems / Show all** to focus on what needs fixing.

It also warns about things caniemail does not cover:

| Warning | Why it matters |
|---|---|
| HTML larger than **102 KB** (and a heads-up above 90 KB) | Gmail clips the message, hiding the rest, including your unsubscribe link and tracking pixel |
| Images without `alt` | Many clients block images by default |
| `http://` images | Mixed content, blocked or flagged by some clients |
| `<script>`, forms, external stylesheets | Stripped or unsupported by most clients |
| Missing `lang` or viewport meta | Accessibility and mobile rendering |
| HTML-only email (no text part) | Scores worse with spam filters |

Refresh the dataset with `python3 scripts/update-caniemail.py`; the date of the data is shown under the chart.

## Deliverability

Checks done locally, with no external service:

- **DKIM** signature verification (sandbox mail is often unsigned, which is reported as information);
- **alignment** between the envelope sender (MAIL FROM) and the From domain;
- valid **From**, **Message-ID**, **Date**, **Subject**, **Reply-To** and **MIME-Version** headers;
- both **HTML and plain-text** parts present;
- **RFC 8058 one-click unsubscribe** (`List-Unsubscribe` + `List-Unsubscribe-Post`), required by Gmail and Yahoo for
  bulk senders;
- raw 8-bit characters in headers.

## Spam score

With a spam engine configured, PostDream asks it to score the message and shows the score and every rule that
matched.

| Engine | Variable |
|---|---|
| [Rspamd](https://rspamd.com) | `RSPAMD_URL` (and `RSPAMD_PASSWORD` if needed). `docker compose --profile spam up -d` starts one at `http://rspamd:11333` |
| SpamAssassin (`spamd`) | `SPAMASSASSIN_ADDR=host:port`, used when Rspamd is not set |

Scoring runs on demand from this tab, or automatically for every email of an inbox with **Spam check** enabled in its
[settings](../administration/inboxes.md#inbox-settings). Automatic scores are computed in the background and
searchable: `spam:>5`, `is:spam`.

## Links & images

**Check links** requests every link and image of the email (HEAD, then GET) and flags:

- broken links (errors, 4xx/5xx);
- links to `localhost` or plain `http://`;
- links outside the inbox's **Expected link domains**, which catches production URLs in staging emails.

![Deliverability, spam and link check](/img/screenshots/link-check.png)

Requests go through an SSRF-safe client: private addresses are blocked unless allowed for the inbox
([Security](../administration/security.md#outbound-requests)). Results are cached for 15 minutes, and checks are
limited to 30 per 10 minutes per user or token.

## From the API

```bash
curl -H "Authorization: Bearer $TOKEN" http://localhost:8025/api/messages/$ID/html-check
curl -H "Authorization: Bearer $TOKEN" http://localhost:8025/api/messages/$ID/deliverability
curl -H "Authorization: Bearer $TOKEN" http://localhost:8025/api/messages/$ID/spam
curl -H "Authorization: Bearer $TOKEN" http://localhost:8025/api/messages/$ID/link-check
```

That makes it easy to fail a CI run when, for example, an email stops supporting Outlook or links to production.
