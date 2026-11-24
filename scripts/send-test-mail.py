#!/usr/bin/env python3
"""Send test e-mails to PostDream (only the Python standard library is needed).

Examples:
  python3 scripts/send-test-mail.py                         # 1 mail, anonymous -> "default" inbox
  python3 scripts/send-test-mail.py -n 20                   # 20 mails
  python3 scripts/send-test-mail.py --user default --password XXXX
  python3 scripts/send-test-mail.py --to qa@acme.test --subject "Reset password" --attach README.md
"""
import argparse
import random
import smtplib
import sys
from email.message import EmailMessage
from email.utils import formatdate, make_msgid
from pathlib import Path



def make_png(size=16, rgb=(99, 102, 241)):
    """Build a tiny solid-color PNG (used as an inline cid: image in the HTML part)."""
    import struct
    import zlib

    def chunk(tag, data):
        return struct.pack(">I", len(data)) + tag + data + struct.pack(">I", zlib.crc32(tag + data) & 0xFFFFFFFF)

    row = b"\x00" + bytes(rgb) * size
    return (b"\x89PNG\r\n\x1a\n" + chunk(b"IHDR", struct.pack(">IIBBBBB", size, size, 8, 2, 0, 0, 0))
            + chunk(b"IDAT", zlib.compress(row * size)) + chunk(b"IEND", b""))


PIXEL = make_png()

SUBJECTS = [
    "Welcome to the platform", "Reset your password", "Your order #{n} has shipped",
    "November invoice", "New sign-in detected", "You've been invited to the team",
    "Confirm your email", "Weekly summary", "Your verification code is {code}",
]
PEOPLE = ["ana@example.com", "bruno@example.org", "carla@acme.test", "diego@qa.local", "eva@startup.io"]


def build(args, i):
    n = random.randint(1000, 9999)
    subject = args.subject or random.choice(SUBJECTS).format(n=n, code=random.randint(100000, 999999))
    to = args.to or random.choice(PEOPLE)
    msg = EmailMessage()
    msg["From"] = f"PostDream Demo <{args.sender}>"
    msg["To"] = to
    if random.random() < 0.3:
        msg["Cc"] = random.choice(PEOPLE)
    msg["Subject"] = subject if args.count == 1 else f"{subject} ({i + 1}/{args.count})"
    msg["Message-ID"] = make_msgid(domain="postdream.test")
    msg["Date"] = formatdate(localtime=True)
    text = f"Hi!\n\nThis is test email #{n} sent to {to}.\n\nCheers,\nThe QA team"
    msg.set_content(text)
    if not args.text_only:
        cid = make_msgid(domain="postdream.test")
        html = f"""<html><body style="font-family:sans-serif;background:#f6f7fb;padding:24px">
  <div style="max-width:560px;margin:auto;background:#fff;border-radius:12px;padding:24px">
    <h2 style="color:#6366f1">{subject}</h2>
    <p>Hi <b>{to}</b>, this is test email <code>#{n}</code>.</p>
    <p><a href="https://example.com/confirm?token={n}" style="background:#6366f1;color:#fff;padding:10px 16px;border-radius:8px;text-decoration:none">Confirm</a></p>
    <p>Inline image: <img src="cid:{cid[1:-1]}" width="16" height="16" alt="pixel"></p>
    <script>alert('this must never run')</script>
  </div></body></html>"""
        msg.add_alternative(html, subtype="html")
        msg.get_payload()[1].add_related(PIXEL, "image", "png", cid=cid)
    for path in args.attach or []:
        p = Path(path)
        msg.add_attachment(p.read_bytes(), maintype="application", subtype="octet-stream", filename=p.name)
    return msg


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--host", default="localhost")
    ap.add_argument("--port", type=int, default=1025)
    ap.add_argument("--user", help="SMTP username (inbox)")
    ap.add_argument("--password", help="SMTP password")
    ap.add_argument("--starttls", action="store_true")
    ap.add_argument("-n", "--count", type=int, default=1)
    ap.add_argument("--sender", default="noreply@demo.test")
    ap.add_argument("--to")
    ap.add_argument("--subject")
    ap.add_argument("--attach", action="append", help="file to attach (repeatable)")
    ap.add_argument("--text-only", action="store_true")
    args = ap.parse_args()

    with smtplib.SMTP(args.host, args.port, timeout=15) as s:
        if args.starttls:
            s.starttls()
        if args.user:
            s.login(args.user, args.password or "")
        for i in range(args.count):
            msg = build(args, i)
            s.send_message(msg)
            print(f"sent: {msg['Subject']} -> {msg['To']}")
    return 0


if __name__ == "__main__":
    try:
        sys.exit(main())
    except smtplib.SMTPException as e:
        print(f"SMTP error: {e}", file=sys.stderr)
        sys.exit(1)
