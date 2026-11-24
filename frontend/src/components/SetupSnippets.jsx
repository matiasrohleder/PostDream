import { useState } from 'react'
import { useToast } from '../lib/context.jsx'
import { copyText } from '../lib/format.js'
import Icon from './Icon.jsx'
import { useT } from '../i18n/index.jsx'

export default function SetupSnippets({ inbox, host, port, pop3Port }) {
  const origin = window.location.origin
  const u = inbox.smtp_username
  const p = inbox.smtp_password
  const snippets = {
    '.env': `SMTP_HOST=${host}
SMTP_PORT=${port}
SMTP_USER=${u}
SMTP_PASSWORD=${p}
SMTP_SECURE=false`,
    'Node.js': `import nodemailer from 'nodemailer'

const transport = nodemailer.createTransport({
  host: '${host}', port: ${port}, secure: false,
  auth: { user: '${u}', pass: '${p}' },
})
await transport.sendMail({
  from: 'app@example.com', to: 'user@example.com',
  subject: 'Hello from PostDream', html: '<b>It works!</b>',
})`,
    Python: `import smtplib
from email.message import EmailMessage

msg = EmailMessage()
msg["From"], msg["To"], msg["Subject"] = "app@example.com", "user@example.com", "Hello"
msg.set_content("It works!")
with smtplib.SMTP("${host}", ${port}) as s:
    s.login("${u}", "${p}")
    s.send_message(msg)`,
    '.NET': `using var client = new SmtpClient("${host}", ${port}) {
    Credentials = new NetworkCredential("${u}", "${p}"),
    EnableSsl = false
};
client.Send("app@example.com", "user@example.com", "Hello", "It works!");`,
    'Laravel': `MAIL_MAILER=smtp
MAIL_HOST=${host}
MAIL_PORT=${port}
MAIL_USERNAME=${u}
MAIL_PASSWORD=${p}
MAIL_ENCRYPTION=null`,
    'swaks': `swaks --server ${host}:${port} --auth-user ${u} --auth-password '${p}' \\
  --from app@example.com --to user@example.com --header "Subject: Hello"`,
    'API (tests)': `# Latest email for a recipient (waits up to 30s for it to arrive)
curl -H "Authorization: Bearer ${inbox.api_token}" \\
  "${origin}/api/messages/latest?to=user@example.com&wait=30s"

# List / search
curl -H "Authorization: Bearer ${inbox.api_token}" \\
  "${origin}/api/messages?q=password+reset&page_size=10"

# Search syntax: tags, sizes, dates, negation…
curl -G -H "Authorization: Bearer ${inbox.api_token}" "${origin}/api/messages" \\
  --data-urlencode 'q=tag:signup -is:read after:1h'

# Empty the inbox
curl -X DELETE -H "Authorization: Bearer ${inbox.api_token}" "${origin}/api/messages"`,
    'Playwright': `import { test, expect } from '@playwright/test'

const MC = '${origin}/api'
const auth = { Authorization: 'Bearer ${inbox.api_token}' }

test('sign-up sends a welcome email', async ({ page, request }) => {
  const email = \`user+\${Date.now()}@example.com\`
  await page.goto('/signup')
  await page.fill('#email', email)
  await page.click('text=Create account')

  // Long-polls until the email arrives (max 30s).
  const res = await request.get(\`\${MC}/messages/latest\`, {
    headers: auth, params: { q: \`to:\${email} subject:welcome\`, wait: '30s' },
  })
  expect(res.ok()).toBeTruthy()
  const msg = await res.json()
  const link = msg.text_body.match(/https?:\\/\\/\\S+/)[0]
  await page.goto(link)
})`,
    'Send API': `# Inject an email without SMTP (same pipeline: tags, rules, webhooks)
curl -X POST "${origin}/api/send" -H "Authorization: Bearer ${inbox.api_token}" \\
  -H "Content-Type: application/json" -d '{
    "from": "App <app@example.com>", "to": ["user@example.com"],
    "subject": "Hello", "html": "<b>It works!</b>", "tags": ["demo"]
  }'`,
    'POP3': `# Read this inbox from any mail client (Thunderbird, Outlook…)
Server:   ${host}
Port:     ${pop3Port || '1110'}${pop3Port ? '' : '  (POP3 is disabled: set POP3_ADDR)'}
Username: ${u}
Password: ${p}
Security: none / STARTTLS when SMTP_TLS_CERT is configured`,
    'Webhook (verify)': `// Node.js: verify the X-PostDream-Signature header of a webhook request
import crypto from 'node:crypto'

function verify(rawBody, header, secret, maxAgeSec = 300) {
  const parts = Object.fromEntries(header.split(',').map((p) => p.split('=')))
  if (Math.abs(Date.now() / 1000 - Number(parts.t)) > maxAgeSec) return false
  const expected = crypto.createHmac('sha256', secret).update(\`\${parts.t}.\${rawBody}\`).digest('hex')
  return crypto.timingSafeEqual(Buffer.from(expected), Buffer.from(parts.v1))
}`,
  }
  const [tab, setTab] = useState('.env')
  const toast = useToast()
  const t = useT()
  return (
    <div>
      <div className="tabs">
        {Object.keys(snippets).map((k) => (
          <button key={k} className={tab === k ? 'on' : ''} onClick={() => setTab(k)}>{k}</button>
        ))}
      </div>
      <div className="snippet-box">
        <button className="btn btn-small copy-btn" onClick={() => copyText(snippets[tab]).then(() => toast(t('Copied')))}>
          <Icon name="copy" size={14} /> {t('Copy')}
        </button>
        <pre>{snippets[tab]}</pre>
      </div>
    </div>
  )
}
