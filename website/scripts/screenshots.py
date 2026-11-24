#!/usr/bin/env python3
"""Capture the documentation screenshots from a running PostDream filled by seed-demo.py.

  pip install playwright && playwright install chromium   (or pass --chrome /path/to/chrome)
  python3 website/scripts/screenshots.py --url http://localhost:8025

Images are written to website/static/img/screenshots/. Each shot is independent: a failure is reported
and the rest still run.
"""
import argparse
import sys
import traceback
from pathlib import Path

from playwright.sync_api import sync_playwright

OUT = Path(__file__).resolve().parent.parent / "static" / "img" / "screenshots"


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--url", default="http://localhost:8025")
    ap.add_argument("--email", default="admin@postdream.local")
    ap.add_argument("--password", default="admin12345")
    ap.add_argument("--chrome", help="Chrome/Chromium executable (default: Playwright's bundled Chromium)")
    ap.add_argument("--only", nargs="*", help="only these shot names")
    args = ap.parse_args()
    OUT.mkdir(parents=True, exist_ok=True)
    base = args.url.rstrip("/")
    failures = []

    with sync_playwright() as p:
        browser = p.chromium.launch(executable_path=args.chrome) if args.chrome else p.chromium.launch()

        def context(scheme="light", width=1440, height=900, mobile=False):
            ctx = browser.new_context(viewport={"width": width, "height": height}, device_scale_factor=1,
                                      color_scheme=scheme, locale="en-US", is_mobile=mobile, has_touch=mobile)
            ctx.add_init_script("try{localStorage.setItem('pd-lang','en')}catch(e){}")
            return ctx

        def login(page):
            page.goto(base + "/login")
            page.get_by_label("Email").fill(args.email)
            page.get_by_label("Password").fill(args.password)
            page.get_by_role("button", name="Sign in").click()
            page.wait_for_url(base + "/")
            page.wait_for_selector("table tbody tr")

        def shot(page, name, full=False, locator=None):
            path = OUT / f"{name}.png"
            if locator is not None:
                locator.screenshot(path=str(path))
            else:
                page.screenshot(path=str(path), full_page=full)
            print("saved", path.name)

        def open_message(page, subject_part):
            # Click the "From" cell: the subject cell holds tag chips that filter the grid instead.
            page.locator("table tbody tr", has_text=subject_part).first.locator("td").nth(2).click()
            page.wait_for_timeout(700)

        def run(name, fn, **ctx_kwargs):
            if args.only and name not in args.only:
                return
            ctx = context(**ctx_kwargs)
            page = ctx.new_page()
            try:
                fn(page, name)
            except Exception:
                failures.append(name)
                traceback.print_exc()
                page.screenshot(path=f"/tmp/failed-{name}.png")
            finally:
                ctx.close()

        # --- shots ------------------------------------------------------------------------------------------

        def s_login(page, name):
            page.goto(base + "/login")
            page.wait_for_selector("form")
            shot(page, name)

        def s_mailbox(page, name):
            login(page)
            page.wait_for_timeout(500)
            shot(page, name)

        def s_search(page, name):
            login(page)
            box = page.get_by_placeholder("Search", exact=False).first
            box.click()
            box.fill("tag:orders")
            box.press("Enter")
            page.wait_for_timeout(600)
            box.click()
            box.press("End")
            box.type(" is:")
            page.wait_for_timeout(500)
            shot(page, name)

        def s_message(page, name):
            login(page)
            open_message(page, "Welcome to Acme")
            page.wait_for_timeout(800)
            shot(page, name)

        def s_message_tab(tab):
            def f(page, name):
                login(page)
                open_message(page, "Welcome to Acme")
                page.get_by_role("button", name=tab, exact=True).first.click()
                page.wait_for_timeout(1500)
                shot(page, name)
            return f

        def s_search_help(page, name):
            login(page)
            page.get_by_title("Search syntax").click()
            page.wait_for_timeout(500)
            shot(page, name)

        def s_release(page, name):
            login(page)
            open_message(page, "Welcome to Acme")
            page.get_by_title("Release to real recipients").click()
            page.wait_for_timeout(600)
            shot(page, name)

        def s_links(page, name):
            login(page)
            open_message(page, "Welcome to Acme")
            page.get_by_role("button", name="Analysis").first.click()
            page.wait_for_timeout(1200)
            page.get_by_role("button", name="Check links").first.click()
            page.wait_for_timeout(6000)
            btn = page.get_by_text("Links & images").first
            btn.scroll_into_view_if_needed()
            shot(page, name)

        def s_bulk(page, name):
            login(page)
            boxes = page.locator("table tbody tr input[type=checkbox]")
            for i in range(3):
                boxes.nth(i).check()
            page.get_by_role("button", name="Tag").first.click()
            page.wait_for_timeout(400)
            shot(page, name)

        def s_settings(path, button=None, inbox=None, wait=600):
            def f(page, name):
                login(page)
                page.goto(base + path)
                page.wait_for_timeout(wait)
                if button:
                    scope = page.locator(".card, section, li, tr, div", has_text=inbox).last if inbox else page
                    if inbox:
                        card = page.get_by_text(inbox, exact=True).last.locator(
                            f"xpath=ancestor::*[.//button[contains(normalize-space(.), '{button}')]][1]")
                        card.get_by_role("button", name=button).first.click()
                    else:
                        scope.get_by_role("button", name=button).first.click()
                    page.wait_for_timeout(900)
                shot(page, name)
            return f

        def s_dark(page, name):
            login(page)
            open_message(page, "Reset your password")
            page.wait_for_timeout(800)
            shot(page, name)

        def s_mobile(page, name):
            login(page)
            shot(page, name)

        run("login", s_login)
        run("mailbox", s_mailbox)
        run("search", s_search)
        run("search-help", s_search_help)
        run("message", s_message)
        run("message-smtp", s_message_tab("SMTP"))
        run("release", s_release)
        run("link-check", s_links)
        run("message-text", s_message_tab("Text"))
        run("message-headers", s_message_tab("Headers"))
        run("analysis", s_message_tab("Analysis"))
        run("bulk-tag", s_bulk)
        run("mailbox-dark", s_dark, scheme="dark")
        run("mobile", s_mobile, width=390, height=844, mobile=True)
        run("settings-inboxes", s_settings("/settings/inboxes"))
        run("setup-snippets", s_settings("/settings/inboxes", "Show configuration examples", "Acme Shop · staging"))
        run("inbox-settings", s_settings("/settings/inboxes", "Settings", "Acme Shop · staging"))
        run("webhooks", s_settings("/settings/inboxes", "Webhooks", "Acme Shop · staging"))
        run("rules", s_settings("/settings/inboxes", "Rules", "Acme Shop · staging"))
        run("chaos", s_settings("/settings/inboxes", "Chaos", "E2E tests"))
        run("users", s_settings("/settings/users"))
        run("account", s_settings("/settings/account"))
        run("preferences", s_settings("/settings/preferences"))
        run("api-docs", s_settings("/settings/api", wait=1200))
        run("server", s_settings("/settings/server"))
        browser.close()

    if failures:
        print("FAILED:", ", ".join(failures), file=sys.stderr)
        sys.exit(1)


if __name__ == "__main__":
    main()
