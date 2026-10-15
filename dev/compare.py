r"""Side-by-side before/after PNG with numbered boxes marking what changed.

usage: python3 dev/compare.py <spec.json> [<spec.json>...]

Prerequisite: pip install --user playwright && python3 -m playwright install chromium

A spec names the two shots (dev/shots.py output), the composite to write, a title for
each half, and marks in pixels of the source PNG:

    {"before": "/tmp/shots/b2/mixed-light-1440.png",
     "after": "/tmp/shots/c1/mixed-light-1440.png",
     "out": "/tmp/shots/compare/mixed-light-1440.png",
     "title_before": "b2 (before)", "title_after": "c1 (after)",
     "marks": [{"n": 1, "side": "before", "x": 40, "y": 120, "w": 300, "h": 80},
               {"n": 1, "side": "after", "x": 40, "y": 120, "w": 300, "h": 110}]}
"""
import base64
import html
import json
import struct
import sys

from playwright.sync_api import sync_playwright

GAP = 24
TITLE = 40
BADGE = 28
# Magenta reads on both PatternFly's white and its dark-theme background.
MARK = "#ff00ff"

STYLE = f"""
body {{ margin: 0; background: #fff; display: flex; gap: {GAP}px; align-items: flex-start; }}
.title {{ height: {TITLE}px; line-height: {TITLE}px; padding: 0 12px; box-sizing: border-box;
  background: #151515; color: #fff; font: bold 18px sans-serif; white-space: nowrap; overflow: hidden; }}
.shot {{ position: relative; }}
.shot img {{ display: block; }}
.box {{ position: absolute; border: 3px solid {MARK}; box-sizing: border-box; }}
.badge {{ position: absolute; width: {BADGE}px; height: {BADGE}px; border-radius: 50%; background: {MARK};
  color: #fff; font: bold 16px/{BADGE}px sans-serif; text-align: center; box-shadow: 0 0 0 2px #000; }}
"""


def png_size(path):
    """path's (width, height) from its PNG header."""
    try:
        with open(path, "rb") as f:
            head = f.read(24)
    except OSError as e:
        sys.exit(f"compare.py: {path}: {e.strerror}")
    if head[:8] != b"\x89PNG\r\n\x1a\n":
        sys.exit(f"compare.py: {path}: not a PNG")
    return struct.unpack(">II", head[16:24])


def badge_pos(m):
    """Top-left of m's badge: above the box, else left of it, else inside its corner."""
    if m["y"] >= BADGE:
        return m["x"], m["y"] - BADGE
    if m["x"] >= BADGE:
        return m["x"] - BADGE, m["y"]
    return m["x"], m["y"]


def half(path, title, marks):
    """HTML for one titled shot with its marks."""
    src = "data:image/png;base64," + base64.b64encode(open(path, "rb").read()).decode()
    # A fixed width keeps a long title from widening the panel past the viewport.
    parts = [f'<div style="flex:none;width:{png_size(path)[0]}px"><div class="title">{html.escape(title)}</div><div class="shot"><img src="{src}">']
    for m in marks:
        bx, by = badge_pos(m)
        parts.append(f'<div class="box" style="left:{m["x"]}px;top:{m["y"]}px;width:{m["w"]}px;height:{m["h"]}px"></div>'
                     f'<div class="badge" style="left:{bx}px;top:{by}px">{m["n"]}</div>')
    return "".join(parts) + "</div></div>"


def load(spec_path):
    """The spec at spec_path with each image's size, exiting on a missing file or a mark outside its image."""
    with open(spec_path) as f:
        spec = json.load(f)
    sizes = {side: png_size(spec[side]) for side in ("before", "after")}
    for m in spec["marks"]:
        w, h = sizes.get(m["side"], (0, 0))
        if not (0 <= m["x"] and 0 <= m["y"] and 0 < m["w"] and 0 < m["h"]
                and m["x"] + m["w"] <= w and m["y"] + m["h"] <= h):
            sys.exit(f"compare.py: {spec_path}: mark {m} is outside the {m['side']} image ({w}x{h})")
    return spec, sizes


def main():
    if len(sys.argv) < 2:
        sys.exit(__doc__)
    specs = [load(p) for p in sys.argv[1:]]
    with sync_playwright() as p, p.chromium.launch() as browser:
        for spec, sizes in specs:
            (bw, bh), (aw, ah) = sizes["before"], sizes["after"]
            body = "".join(half(spec[side], spec[f"title_{side}"], [m for m in spec["marks"] if m["side"] == side])
                           for side in ("before", "after"))
            with browser.new_context(viewport={"width": bw + GAP + aw, "height": TITLE + max(bh, ah)}) as ctx:
                page = ctx.new_page()
                page.set_content(f"<!doctype html><style>{STYLE}</style>{body}")
                page.screenshot(path=spec["out"])
            print(f"== {spec['out']} {bw + GAP + aw}x{TITLE + max(bh, ah)}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
