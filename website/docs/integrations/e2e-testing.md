---
title: "E2E testing"
description: "Wait for the email your app sends, read the link or code in it, and assert on content, tags and analysis."
---

# E2E testing

The typical test: *the user signs up, receives an email, clicks the link*. PostDream makes the middle step one HTTP
call.

## The pattern

1. **Use a dedicated inbox** for the test suite (for example `e2e`) and give the app under test its SMTP credentials.
2. **Clean up** before the test: `DELETE /api/messages` with the inbox token.
3. **Trigger** the email in your app.
4. **Wait** for it with `GET /api/messages/latest?wait=30s` and a query that identifies it.
5. **Read** what you need (`text_body`, `html_body`, `subject`, `tags`, attachments) and continue the test.

:::tip Unique addresses
Use a unique recipient per test, such as `user+${Date.now()}@example.com`. It makes queries unambiguous when tests run
in parallel, and plus-addressing tags the email automatically.
:::

## Playwright

```js
import { test, expect } from '@playwright/test'

const MC = 'http://localhost:8025/api'
const auth = { Authorization: `Bearer ${process.env.POSTDREAM_TOKEN}` }

test('sign-up sends a welcome email', async ({ page, request }) => {
  const email = `user+${Date.now()}@example.com`
  await page.goto('/signup')
  await page.fill('#email', email)
  await page.click('text=Create account')

  // Long-polls until the email arrives (max 30s).
  const res = await request.get(`${MC}/messages/latest`, {
    headers: auth, params: { q: `to:${email} subject:welcome`, wait: '30s' },
  })
  expect(res.ok()).toBeTruthy()
  const msg = await res.json()
  const link = msg.text_body.match(/https?:\/\/\S+/)[0]
  await page.goto(link)
})
```

Every inbox's **Show configuration examples** has this test with the token already filled in.

## Shell / any language

```bash
TOKEN=pd_xxxxxxxx
# clean up before the test
curl -X DELETE -H "Authorization: Bearer $TOKEN" http://localhost:8025/api/messages
# ... your app sends the "forgot password" email ...
curl -H "Authorization: Bearer $TOKEN" \
  "http://localhost:8025/api/messages/latest?to=user@example.com&subject=password&wait=30s" | jq -r .text_body
```

## Going further

- **Assert on quality, not only content.** Fail the test when the email links to production
  (`/link-check` with **Expected link domains**), when it is missing a text part (`/deliverability`), or when client
  support drops (`/html-check`).
- **Test failure handling** by turning on [chaos testing](chaos-testing.md) from the test itself with the same token.
- **Inject emails** your app would receive, without SMTP, with `POST /api/send`.
- **React instead of polling** with a [webhook](webhooks.md) when another system should act on the email.

## Running PostDream inside the test

Start it next to the suite with Testcontainers and `AUTH_ENABLED=false`; see
[Deployment → Testcontainers](../operations/deployment.md#testcontainers).
