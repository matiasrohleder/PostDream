#!/usr/bin/env python3
"""Refresh the vendored caniemail.com support data used by the HTML check.

Downloads https://www.caniemail.com/api/data.json (MIT licensed, (c) Rémi Parmentier)
and writes a compact version (latest result per client platform) to
backend/internal/htmlcheck/caniemail.json.

    python3 scripts/update-caniemail.py            # download
    python3 scripts/update-caniemail.py data.json  # use a local copy
"""
import json
import pathlib
import sys
import urllib.request

SRC = "https://www.caniemail.com/api/data.json"
OUT = pathlib.Path(__file__).resolve().parent.parent / "backend/internal/htmlcheck/caniemail.json"


def version_key(v):
    # Versions look like "2019-10", "16.80", "2013": compare numerically chunk by chunk.
    parts = []
    for chunk in v.replace("-", ".").split("."):
        parts.append(int(chunk) if chunk.isdigit() else -1)
    return parts


def main():
    if len(sys.argv) > 1:
        data = json.load(open(sys.argv[1], encoding="utf-8"))
    else:
        req = urllib.request.Request(SRC, headers={"User-Agent": "postdream-update-script"})
        with urllib.request.urlopen(req, timeout=60) as r:
            data = json.load(r)

    features = []
    for f in data["data"]:
        if f["category"] not in ("html", "css", "image"):
            continue
        support = {}
        for family, platforms in (f.get("stats") or {}).items():
            for platform, versions in platforms.items():
                if not versions:
                    continue
                latest = max(versions, key=version_key)
                support.setdefault(family, {})[platform] = versions[latest]
        features.append({
            "slug": f["slug"],
            "title": f["title"],
            "url": f["url"],
            "category": f["category"],
            "support": support,
            "notes": f.get("notes_by_num") or {},
        })

    out = {
        "source": SRC,
        "license": "MIT (c) Rémi Parmentier - https://github.com/hteumeuleu/caniemail",
        "updated": data.get("last_update_date", ""),
        "families": data["nicenames"]["family"],
        "platforms": data["nicenames"]["platform"],
        "features": features,
    }
    OUT.write_text(json.dumps(out, ensure_ascii=False, separators=(",", ":")), encoding="utf-8")
    print(f"wrote {OUT} ({len(features)} features, {OUT.stat().st_size // 1024} KB)")


if __name__ == "__main__":
    main()
