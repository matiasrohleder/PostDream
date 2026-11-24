#!/usr/bin/env python3
"""Fill a fresh PostDream with realistic demo data for the documentation screenshots.

Only the Python standard library is needed. Run it against an EMPTY instance (local, never production):

  python3 website/scripts/seed-demo.py --url http://localhost:8025 --password admin12345

It creates inboxes, a member user, auto-tag rules, webhooks (pointing at a tiny local receiver it starts),
chaos rules and ~30 emails sent both over SMTP and through the send API.
"""
import argparse
import base64
import http.cookiejar
import http.server
import json
import random
import smtplib
import threading
import time
import urllib.request
from email.message import EmailMessage
from email.utils import formatdate, make_msgid

random.seed(7)


class Client:
    def __init__(self, base):
        self.base = base.rstrip("/")
        self.opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))

    def call(self, method, path, body=None):
        data = json.dumps(body).encode() if body is not None else None
        req = urllib.request.Request(self.base + path, data=data, method=method,
                                     headers={"Content-Type": "application/json"})
        with self.opener.open(req) as r:
            raw = r.read()
            return json.loads(raw) if raw else None


def start_webhook_receiver(port):
    class H(http.server.BaseHTTPRequestHandler):
        def do_POST(self):
            self.rfile.read(int(self.headers.get("Content-Length", 0)))
            self.send_response(204)
            self.end_headers()

        def log_message(self, *a):
            pass

    srv = http.server.ThreadingHTTPServer(("127.0.0.1", port), H)
    threading.Thread(target=srv.serve_forever, daemon=True).start()


def card(title, body_html, button=None, color="#6366f1"):
    btn = ""
    if button:
        label, href = button
        btn = (f'<p style="margin:28px 0"><a href="{href}" style="background:{color};color:#ffffff;padding:12px 20px;'
               f'border-radius:8px;text-decoration:none;font-weight:600">{label}</a></p>')
    return f"""<!doctype html><html lang="en"><head><meta name="viewport" content="width=device-width">
<style>.muted{{color:#6b7280;font-size:13px}} @media (max-width:600px){{.box{{padding:16px!important}}}}</style></head>
<body style="margin:0;background:#f4f5fb;font-family:Helvetica,Arial,sans-serif;color:#111827">
<div style="max-width:560px;margin:32px auto;background:#ffffff;border-radius:14px;overflow:hidden;border:1px solid #e5e7eb">
  <div style="background:{color};padding:18px 28px;color:#fff;font-weight:700;font-size:18px">Acme Shop</div>
  <div class="box" style="padding:28px">
    <h1 style="font-size:22px;margin:0 0 12px">{title}</h1>
    {body_html}
    {btn}
    <p class="muted">You are receiving this because you have an account at Acme Shop.
      <a href="https://acme-shop.example/unsubscribe?u=42">Unsubscribe</a></p>
  </div>
</div></body></html>"""


def invoice_pdf():
    # Smallest valid-looking PDF, enough for an attachment row.
    return b"%PDF-1.4\n1 0 obj<<>>endobj\ntrailer<<>>\n%%EOF\n"


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--url", default="http://localhost:8025")
    ap.add_argument("--email", default="admin@postdream.local")
    ap.add_argument("--password", default="admin12345")
    ap.add_argument("--smtp-host", default="localhost")
    ap.add_argument("--smtp-port", type=int, default=1025)
    ap.add_argument("--webhook-port", type=int, default=9099)
    args = ap.parse_args()

    start_webhook_receiver(args.webhook_port)
    c = Client(args.url)
    c.call("POST", "/api/auth/login", {"email": args.email, "password": args.password})

    staging = c.call("POST", "/api/inboxes", {"name": "Acme Shop · staging", "slug": "acme-staging",
                                              "description": "Staging environment of the shop", "retention_hours": 168})
    qa = c.call("POST", "/api/inboxes", {"name": "Billing · QA", "slug": "billing-qa",
                                         "description": "Invoices and receipts sent by the billing service"})
    e2e = c.call("POST", "/api/inboxes", {"name": "E2E tests", "slug": "e2e",
                                          "description": "Used by the Playwright suite in CI", "retention_hours": 24})

    c.call("PATCH", f"/api/inboxes/{staging['id']}/settings", {
        "link_domains": ["staging.acme-shop.example", "acme-shop.example"],
        "allowed_hosts": ["127.0.0.1"],
        "relay_recipients": r"@acme\.test$",
    })
    c.call("PATCH", f"/api/inboxes/{qa['id']}/settings", {"allowed_recipients": r"@(acme\.test|example\.org)$"})
    c.call("PUT", f"/api/inboxes/{e2e['id']}/chaos", {
        "rcpt": {"code": 451, "probability": 20, "message": "Mailbox temporarily unavailable"},
        "data": {"code": 552, "probability": 5, "message": "Message size exceeds limit"},
    })

    c.call("POST", "/api/rules", {"inbox_id": None, "query": 'subject:"reset your password"', "tag": "password-reset"})
    c.call("POST", "/api/rules", {"inbox_id": staging["id"], "query": "subject:order", "tag": "orders"})
    c.call("POST", "/api/rules", {"inbox_id": qa["id"], "query": "has:attachment", "tag": "invoice"})

    hook = c.call("POST", f"/api/inboxes/{staging['id']}/webhooks",
                  {"url": f"http://127.0.0.1:{args.webhook_port}/postdream", "events": ["message.received"],
                   "active": True})
    c.call("POST", f"/api/inboxes/{staging['id']}/webhooks",
           {"url": "https://ci.acme.example/hooks/mail", "events": ["message.received"], "active": False})

    c.call("POST", "/api/users", {"email": "lucia@acme.test", "name": "Lucía (QA)", "password": "member-pass-123",
                                  "role": "member", "inbox_ids": [staging["id"], e2e["id"]]})
    c.call("POST", "/api/users", {"email": "tomas@acme.test", "name": "Tomás (Billing)", "password": "member-pass-123",
                                  "role": "member", "inbox_ids": [qa["id"]]})

    people = ["ana@acme.test", "bruno@example.org", "carla@acme.test", "diego@acme.test", "eva@example.org"]

    def send(inbox, subject, html, to=None, tags=None, text=None, attachments=None, headers=None, sender=None):
        body = {"inbox": inbox["slug"], "from": sender or "Acme Shop <no-reply@staging.acme-shop.example>",
                "to": [to or random.choice(people)], "subject": subject, "html": html,
                "text": text or "Open this email in an HTML capable client.", "tags": tags or []}
        if attachments:
            body["attachments"] = attachments
        if headers:
            body["headers"] = headers
        c.call("POST", "/api/send", body)

    for n in range(3):
        to = people[n]
        send(staging, "Reset your password",
             card("Reset your password", f"<p>Hi {to.split('@')[0].title()}, someone asked to reset your password.</p>"
                  "<p>The link expires in 30 minutes.</p>",
                  ("Choose a new password", f"https://staging.acme-shop.example/reset?token={random.randint(10**6, 10**7)}")),
             to=to)
    for n in range(4):
        num = random.randint(10000, 99999)
        send(staging, f"Your order #{num} has shipped",
             card(f"Order #{num} is on its way", "<p>Your package left our warehouse today.</p>"
                  "<table style='width:100%;border-collapse:collapse;font-size:14px'>"
                  "<tr><td>Mechanical keyboard</td><td style='text-align:right'>$129.00</td></tr>"
                  "<tr><td>USB-C cable</td><td style='text-align:right'>$12.00</td></tr></table>"
                  '<img src="http://cdn.acme-shop.example/track.gif" width="1" height="1">',
                  ("Track package", f"https://acme-shop.example/orders/{num}")))
    send(staging, "Welcome to Acme Shop 👋",
         card("Welcome aboard!", "<p>Thanks for signing up. Here are three things to try first:</p>"
              "<ul><li>Complete your profile</li><li>Save your favorite products</li><li>Invite a friend</li></ul>"
              '<p><a href="http://localhost:3000/onboarding">Start the tour</a></p>',
              ("Confirm your email", "https://staging.acme-shop.example/confirm?t=abc")),
         to="qa+signup@acme.test", headers={"X-Tags": "onboarding"})
    send(staging, "New sign-in from Chrome on Windows",
         card("New sign-in detected", "<p>We noticed a new sign-in to your account from <b>Buenos Aires, AR</b>.</p>",
              ("Review activity", "https://staging.acme-shop.example/security"), color="#0d9488"))

    for month in ["September", "October", "November"]:
        num = random.randint(1000, 9999)
        send(qa, f"{month} invoice INV-{num}",
             card(f"Your {month} invoice", f"<p>Invoice <b>INV-{num}</b> for <b>$49.00</b> is attached.</p>",
                  ("Pay now", f"https://billing.acme-shop.example/pay/{num}"), color="#f97316"),
             sender="Acme Billing <billing@acme-shop.example>", to="tomas@acme.test",
             attachments=[{"filename": f"INV-{num}.pdf", "content_type": "application/pdf",
                           "content": base64.b64encode(invoice_pdf()).decode()}])
    send(qa, "Payment failed: action required",
         card("We could not charge your card", "<p>Please update your payment method within 7 days.</p>",
              ("Update card", "https://billing.acme-shop.example/card"), color="#e11d48"),
         sender="Acme Billing <billing@acme-shop.example>", to="bruno@example.org")

    for n in range(5):
        send(e2e, f"Your verification code is {random.randint(100000, 999999)}",
             card("Verify it's you", "<p>Enter this code to continue. It expires in 10 minutes.</p>"),
             to=f"e2e+run{n}@acme.test", tags=["ci"])

    # A few plain SMTP messages into the default inbox (anonymous) to show the SMTP path and envelope data.
    for n in range(4):
        msg = EmailMessage()
        msg["From"] = "Legacy cron <cron@old-server.local>"
        msg["To"] = random.choice(people)
        msg["Subject"] = random.choice(["Nightly report", "Disk usage warning", "Backup finished", "Weekly summary"])
        msg["Message-ID"] = make_msgid(domain="old-server.local")
        msg["Date"] = formatdate(localtime=True)
        msg.set_content("Report generated automatically.\n\nAll jobs finished.")
        with smtplib.SMTP(args.smtp_host, args.smtp_port) as s:
            s.send_message(msg)
        time.sleep(0.2)

    # Mark a few as read so the grid shows both states.
    page = c.call("GET", "/api/messages?page_size=100")
    ids = [m["id"] for m in page["items"]]
    c.call("POST", "/api/messages/read", {"ids": ids[8:20], "read": True})
    time.sleep(2)  # let webhook jobs run
    print(json.dumps({"staging": staging["id"], "qa": qa["id"], "e2e": e2e["id"], "webhook": hook["id"],
                      "messages": len(ids)}))


if __name__ == "__main__":
    main()
