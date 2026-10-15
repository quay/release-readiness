r"""Measured accessibility checks of release-readiness pages, from the running page's computed styles.

usage: python3 dev/a11y.py [--width PX]... [--theme light|dark]... <base-url> <path>...

Prerequisite: pip install --user playwright && python3 -m playwright install chromium

Each path is checked at each width (default 1440) in each theme (default both; dark is
prefers-color-scheme: dark, which the app maps to PatternFly's pf-v6-theme-dark class):

- WCAG 2.2 AA contrast (1.4.3) of every visible text element: 4.5:1, or 3:1 for large text
  (24px, or 18.66px bold). The background is the first opaque ancestor background, with any
  translucent ones between composited over it. Text under an opacity, background-image or
  pseudo-element background is "unmeasured": listed, neither passed nor failed. Text in a
  disabled control is exempt.
- Non-text contrast (1.4.11) of status icons (svg in PF Icon, Label, Alert and helper text
  icons), their computed fill against the same background: 3:1.
- Target size (2.5.8): links, buttons, inputs and other focusable targets under 24x24 CSS px,
  unless inline in text or spaced so a 24px circle on each meets no other target.
- Horizontal overflow of the document and of PatternFly's scrolling main.
- Console errors and uncaught page errors.

It prints one summary line per page, width and theme, then one line per failure and per
unmeasured node, and exits 1 if anything failed.

    python3 dev/a11y.py --width 1440 --width 420 http://localhost:5174 /releases/quay-v3.18.1
"""
import argparse
import sys

from playwright.sync_api import Error, sync_playwright

MEASURE = r"""() => {
  const ctx = document.createElement('canvas').getContext('2d', {willReadFrequently: true});
  // Computed colours are rgb()/rgba() in Chromium unless the CSS used another colour space;
  // the canvas converts those.
  const rgba = s => {
    const m = s.match(/^rgba?\(([\d.]+),? ([\d.]+),? ([\d.]+)(?:,? \/? ?([\d.]+))?\)$/);
    if (m) return [+m[1], +m[2], +m[3], m[4] === undefined ? 1 : +m[4]];
    ctx.clearRect(0, 0, 1, 1);
    ctx.fillStyle = '#000'; ctx.fillStyle = s; ctx.fillRect(0, 0, 1, 1);
    const d = ctx.getImageData(0, 0, 1, 1).data;
    return [d[0], d[1], d[2], d[3] / 255];
  };
  const over = (top, bottom) => top.slice(0, 3).map((c, i) => top[3] * c + (1 - top[3]) * bottom[i]);
  const hex = c => '#' + c.slice(0, 3).map(v => Math.round(v).toString(16).padStart(2, '0')).join('');
  const lum = c => {
    const [r, g, b] = c.map(v => (v /= 255) <= 0.04045 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4);
    return 0.2126 * r + 0.7152 * g + 0.0722 * b;
  };
  const ratio = (a, b) => {
    const [x, y] = [lum(a), lum(b)].sort((p, q) => q - p);
    return (x + 0.05) / (y + 0.05);
  };
  // PatternFly's button ripple at rest: a centred circle whose transparent core (P% of a
  // background S% of the element) covers the whole element when P * S >= 100%.
  const restingRipple = cs => {
    const m = cs.backgroundImage.match(/^radial-gradient\(circle, rgba\(0, 0, 0, 0\) ([\d.]+)%, [^)]+\)[^)]*\)$/);
    const size = cs.backgroundSize.match(/^([\d.]+)% \1%$/);
    return m && size && cs.backgroundPosition === '50% 50%' && m[1] * size[1] >= 10000;
  };
  // Layers painted over e's own background, top first: an absolute pseudo-element filling its
  // containing block (PF Label's ::before) is one; any other pseudo background is not resolved.
  const pseudoLayers = (e, cs) => {
    const out = [];
    for (const p of ['::after', '::before']) {
      const ps = getComputedStyle(e, p);
      if (ps.content === 'none' || (ps.backgroundImage === 'none' && rgba(ps.backgroundColor)[3] === 0)) continue;
      if (ps.backgroundImage !== 'none' || ps.position !== 'absolute' || ps.inset !== '0px' || ps.opacity !== '1')
        return {why: 'background on ' + label(e) + p};
      out.push(rgba(ps.backgroundColor));
    }
    return {out};
  };
  const background = el => {
    const layers = [];
    for (let e = el; e; e = e.parentElement) {
      const cs = getComputedStyle(e);
      if (cs.opacity !== '1') return {why: 'opacity ' + cs.opacity + ' on ' + label(e)};
      if (cs.backgroundImage !== 'none' && !restingRipple(cs)) return {why: 'background-image on ' + label(e)};
      const pseudo = pseudoLayers(e, cs);
      if (pseudo.why) return pseudo;
      for (const c of [...pseudo.out, rgba(cs.backgroundColor)]) {
        if (c[3] > 0) layers.push(c);
        if (c[3] >= 1) return {bg: layers.reverse().reduce((acc, l) => over(l, acc), [0, 0, 0])};
      }
    }
    return {why: 'no opaque background'};
  };
  const label = e => e.tagName.toLowerCase() +
    [...e.classList].slice(0, 2).map(c => '.' + c).join('');
  // e's accessible-ish name: its own or its nearest ancestor's aria-label or text.
  const snippet = e => {
    for (let n = e; n; n = n.parentElement) {
      const t = (n.ariaLabel || n.innerText || '').trim();
      if (t) return JSON.stringify(t.replace(/\s+/g, ' ').slice(0, 40));
    }
    return '""';
  };
  const shown = e => e.checkVisibility({opacityProperty: true, visibilityProperty: true}) &&
    e.getBoundingClientRect().width > 1 && e.getBoundingClientRect().height > 1;

  const text = [], icons = [], unmeasured = [];
  const parents = new Set();
  const walker = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT);
  for (let n; (n = walker.nextNode());) {
    const el = n.parentElement;
    if (!n.data.trim() || !el || parents.has(el) || el.closest('script, style, noscript, svg')) continue;
    parents.add(el);
    if (!shown(el) || el.closest(':disabled, [aria-disabled="true"]')) continue;
    const cs = getComputedStyle(el);
    const b = background(el);
    if (b.why) { unmeasured.push({kind: 'text', label: label(el) + ' ' + snippet(el), why: b.why}); continue; }
    const fg = over(rgba(cs.color), b.bg);
    const px = parseFloat(cs.fontSize);
    const need = px >= 24 || (px >= 18.66 && +cs.fontWeight >= 700) ? 3 : 4.5;
    text.push({label: label(el) + ' ' + snippet(el), ratio: ratio(fg, b.bg), need, fg: hex(fg), bg: hex(b.bg)});
  }
  const status = '.pf-v6-c-icon__content, .pf-v6-c-label__icon, .pf-v6-c-alert__icon, .pf-v6-c-helper-text__item-icon';
  for (const svg of document.querySelectorAll(`:is(${status}) svg:not(svg svg)`)) {
    if (!shown(svg)) continue;
    const holder = svg.closest(status);
    const kind = [...holder.classList].find(c => c.startsWith('pf-m-'))?.slice(5) || label(holder).split('.')[1];
    const where = label(holder) + ' near ' + snippet(holder);
    const b = background(svg);
    if (b.why) { unmeasured.push({kind: 'icon', label: where, why: b.why}); continue; }
    const cs = getComputedStyle(svg);
    const fg = over(rgba(cs.fill.startsWith('rgb') ? cs.fill : cs.color), b.bg);
    icons.push({kind, label: where, ratio: ratio(fg, b.bg), need: 3, fg: hex(fg), bg: hex(b.bg)});
  }

  const targets = [...document.querySelectorAll('a[href], button, input:not([type="hidden"]), select, textarea, ' +
    'summary, [role="button"], [role="link"], [role="checkbox"], [role="radio"], [role="switch"], [role="tab"], ' +
    '[role="menuitem"], [tabindex]:not([tabindex="-1"])')]
    .filter(e => shown(e) && !e.disabled).map(e => ({e, r: e.getBoundingClientRect()}));
  const inline = e => {
    if (getComputedStyle(e).display !== 'inline' || !e.parentElement) return false;
    let rest = e.parentElement.textContent;
    for (const t of e.parentElement.querySelectorAll('a, button')) rest = rest.replace(t.textContent, '');
    return rest.trim() !== '';
  };
  const small = targets.filter(t => (t.r.width < 24 || t.r.height < 24) && !inline(t.e));
  const centre = r => [r.left + r.width / 2, r.top + r.height / 2];
  const hits = (t, u) => {
    if (t.e.contains(u.e) || u.e.contains(t.e)) return false;
    const [cx, cy] = centre(t.r), [ux, uy] = centre(u.r);
    const dx = Math.max(u.r.left - cx, 0, cx - u.r.right), dy = Math.max(u.r.top - cy, 0, cy - u.r.bottom);
    return dx * dx + dy * dy < 144 || (small.includes(u) && Math.hypot(cx - ux, cy - uy) < 24);
  };
  const tiny = [];
  for (const t of small) {
    const u = targets.find(u => u !== t && hits(t, u));
    if (u) tiny.push({label: label(t.e) + ' ' + snippet(t.e), w: t.r.width, h: t.r.height,
      near: label(u.e) + ' ' + snippet(u.e)});
  }

  const main = document.querySelector('main.pf-v6-c-page__main');
  const de = document.documentElement;
  return {
    dark: de.classList.contains('pf-v6-theme-dark'), text, icons, unmeasured, tiny,
    overflow: {doc: de.scrollWidth - de.clientWidth, main: main ? main.scrollWidth - main.clientWidth : 0},
  };
}"""


def check(browser, url, width, theme):
    """Measure url and print its summary and failures; return the failure count, or why it did not load."""
    with browser.new_context(viewport={"width": width, "height": 1000}, locale="en-US",
                             timezone_id="America/New_York", color_scheme=theme) as ctx:
        page = ctx.new_page()
        errors = []
        page.on("console", lambda m: errors.append(f"{m.text} {m.location.get('url', '')}") if m.type == "error" else None)
        page.on("pageerror", lambda e: errors.append(str(e)))
        try:
            resp = page.goto(url, wait_until="networkidle")
        except Error as e:
            return e.message.partition("\n")[0]
        if resp.status >= 400:
            return f"HTTP {resp.status}"
        page.wait_for_timeout(1500)
        m = page.evaluate(MEASURE)
    if m["dark"] != (theme == "dark"):
        return f"pf-v6-theme-dark is {'set' if m['dark'] else 'missing'} under a {theme} color scheme"
    low_text = [t for t in m["text"] if t["ratio"] < t["need"]]
    low_icons = [i for i in m["icons"] if i["ratio"] < i["need"]]
    overflow = [f"{k}+{v}px" for k, v in m["overflow"].items() if v > 0]
    low = lambda rs: f"{min(rs):.2f}" if rs else "-"
    by_kind = {}
    for i in m["icons"]:
        by_kind[i["kind"]] = min(by_kind.get(i["kind"], 99), i["ratio"])
    print(f"== {url} {width}px {theme}:"
          f" text={len(m['text'])} min={low([t['ratio'] for t in m['text']])} fail={len(low_text)}"
          f" unmeasured={len(m['unmeasured'])}"
          f" icons={len(m['icons'])} min={low(list(by_kind.values()))}"
          f" ({', '.join(f'{k} {v:.2f}' for k, v in sorted(by_kind.items()))}) fail={len(low_icons)}"
          f" small_targets={len(m['tiny'])} overflow={'+'.join(overflow) or 'none'} console_errors={len(errors)}")
    for kind, rows in (("text", low_text), ("icon", low_icons)):
        for r in rows:
            print(f"  FAIL {kind} {r['ratio']:.2f} < {r['need']} {r['fg']} on {r['bg']} {r['label']}")
    for t in m["tiny"]:
        print(f"  FAIL target {t['w']:.0f}x{t['h']:.0f} {t['label']} (24px circle meets {t['near']})")
    for o in overflow:
        print(f"  FAIL overflow {o}")
    for e in errors:
        print(f"  FAIL console {e.splitlines()[0] if e else e}")
    for u in m["unmeasured"]:
        print(f"  unmeasured {u['kind']} {u['label']}: {u['why']}")
    return len(low_text) + len(low_icons) + len(m["tiny"]) + len(overflow) + len(errors)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--width", action="append", type=int, metavar="PX", help="viewport width, repeatable (default 1440)")
    ap.add_argument("--theme", action="append", choices=("light", "dark"),
                    help="color scheme, repeatable (default both)")
    ap.add_argument("base", metavar="base-url")
    ap.add_argument("paths", metavar="path", nargs="+")
    args = ap.parse_args()
    failed = False
    with sync_playwright() as p, p.chromium.launch() as browser:
        for path in args.paths:
            for width in args.width or [1440]:
                for theme in args.theme or ["light", "dark"]:
                    res = check(browser, args.base.rstrip("/") + path, width, theme)
                    if isinstance(res, str):
                        print(f"{path} at {width}px {theme}: {res}", file=sys.stderr)
                    failed = failed or res != 0
    return 1 if failed else 0


if __name__ == "__main__":
    sys.exit(main())
