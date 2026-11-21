import type { Page } from "@playwright/test";

type Color = number[];
interface Contrast {
	label: string;
	ratio: number;
	need: number;
	fg: string;
	bg: string;
	kind?: string;
}
interface Unmeasured {
	kind: string;
	label: string;
	why: string;
}
interface Target {
	e: HTMLElement;
	r: DOMRect;
}

/** Port of the existing computed-style audit; unmeasured nodes are not passes. */
export function measureAccessibility() {
	const ctx = document
		.createElement("canvas")
		.getContext("2d", { willReadFrequently: true })!;
	// Computed colours are rgb()/rgba() in Chromium unless the CSS used another colour space;
	// the canvas converts those.
	const rgba = (s: string): Color => {
		const m = s.match(
			/^rgba?\(([\d.]+),? ([\d.]+),? ([\d.]+)(?:,? \/? ?([\d.]+))?\)$/,
		);
		if (m) return [+m[1], +m[2], +m[3], m[4] === undefined ? 1 : +m[4]];
		ctx.clearRect(0, 0, 1, 1);
		ctx.fillStyle = "#000";
		ctx.fillStyle = s;
		ctx.fillRect(0, 0, 1, 1);
		const d = ctx.getImageData(0, 0, 1, 1).data;
		return [d[0], d[1], d[2], d[3] / 255];
	};
	const over = (top: Color, bottom: Color): Color =>
		top.slice(0, 3).map((c, i) => top[3] * c + (1 - top[3]) * bottom[i]);
	const hex = (c: Color) =>
		"#" +
		c
			.slice(0, 3)
			.map((v) => Math.round(v).toString(16).padStart(2, "0"))
			.join("");
	const lum = (c: Color) => {
		const [r, g, b] = c.map((value) => {
			const v = value / 255;
			return v <= 0.04045 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4;
		});
		return 0.2126 * r + 0.7152 * g + 0.0722 * b;
	};
	const ratio = (a: Color, b: Color) => {
		const [x, y] = [lum(a), lum(b)].sort((p, q) => q - p);
		return (x + 0.05) / (y + 0.05);
	};
	// PatternFly's button ripple at rest: a centred circle whose transparent core (P% of a
	// background S% of the element) covers the whole element when P * S >= 100%.
	const restingRipple = (cs: CSSStyleDeclaration) => {
		const m = cs.backgroundImage.match(
			/^radial-gradient\(circle, rgba\(0, 0, 0, 0\) ([\d.]+)%, [^)]+\)[^)]*\)$/,
		);
		const size = cs.backgroundSize.match(/^([\d.]+)% \1%$/);
		return (
			m &&
			size &&
			cs.backgroundPosition === "50% 50%" &&
			Number(m[1]) * Number(size[1]) >= 10000
		);
	};
	// Layers painted over e's own background, top first: an absolute pseudo-element filling its
	// containing block (PF Label's ::before) is one; any other pseudo background is not resolved.
	const pseudoLayers = (
		e: Element,
	): { why: string; out?: never } | { out: Color[]; why?: never } => {
		const out = [];
		for (const p of ["::after", "::before"]) {
			const ps = getComputedStyle(e, p);
			if (
				ps.content === "none" ||
				(ps.backgroundImage === "none" && rgba(ps.backgroundColor)[3] === 0)
			)
				continue;
			if (
				ps.backgroundImage !== "none" ||
				ps.position !== "absolute" ||
				ps.inset !== "0px" ||
				ps.opacity !== "1"
			)
				return { why: `background on ${label(e)}${p}` };
			out.push(rgba(ps.backgroundColor));
		}
		return { out };
	};
	const background = (
		el: Element,
	): { why: string; bg?: never } | { bg: Color; why?: never } => {
		const layers = [];
		for (let e: Element | null = el; e; e = e.parentElement) {
			const cs = getComputedStyle(e);
			if (cs.opacity !== "1")
				return { why: `opacity ${cs.opacity} on ${label(e)}` };
			if (cs.backgroundImage !== "none" && !restingRipple(cs))
				return { why: `background-image on ${label(e)}` };
			const pseudo = pseudoLayers(e);
			if (pseudo.out === undefined) return pseudo;
			for (const c of [...pseudo.out, rgba(cs.backgroundColor)]) {
				if (c[3] > 0) layers.push(c);
				if (c[3] >= 1)
					return {
						bg: layers.reverse().reduce((acc, l) => over(l, acc), [0, 0, 0]),
					};
			}
		}
		return { why: "no opaque background" };
	};
	const label = (e: Element) =>
		e.tagName.toLowerCase() +
		[...e.classList]
			.slice(0, 2)
			.map((c) => `.${c}`)
			.join("");
	// e's accessible-ish name: its own or its nearest ancestor's aria-label or text.
	const snippet = (e: Element) => {
		for (let n: Element | null = e; n; n = n.parentElement) {
			const t = (n.ariaLabel || n.textContent || "").trim();
			if (t) return JSON.stringify(t.replace(/\s+/g, " ").slice(0, 40));
		}
		return '""';
	};
	const shown = (e: Element) =>
		e.checkVisibility({ opacityProperty: true, visibilityProperty: true }) &&
		e.getBoundingClientRect().width > 1 &&
		e.getBoundingClientRect().height > 1;

	const text: Contrast[] = [],
		icons: Contrast[] = [],
		unmeasured: Unmeasured[] = [];
	const parents = new Set();
	const walker = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT);
	for (let n = walker.nextNode(); n; n = walker.nextNode()) {
		const el = n.parentElement;
		if (
			!n.textContent?.trim() ||
			!el ||
			parents.has(el) ||
			el.closest("script, style, noscript, svg")
		)
			continue;
		parents.add(el);
		if (!shown(el) || el.closest(':disabled, [aria-disabled="true"]')) continue;
		const cs = getComputedStyle(el);
		const b = background(el);
		if (b.bg === undefined) {
			unmeasured.push({
				kind: "text",
				label: `${label(el)} ${snippet(el)}`,
				why: b.why,
			});
			continue;
		}
		const fg = over(rgba(cs.color), b.bg);
		const px = parseFloat(cs.fontSize);
		const need = px >= 24 || (px >= 18.66 && +cs.fontWeight >= 700) ? 3 : 4.5;
		text.push({
			label: `${label(el)} ${snippet(el)}`,
			ratio: ratio(fg, b.bg),
			need,
			fg: hex(fg),
			bg: hex(b.bg),
		});
	}
	const status =
		".pf-v6-c-icon__content, .pf-v6-c-label__icon, .pf-v6-c-alert__icon, .pf-v6-c-helper-text__item-icon";
	for (const svg of document.querySelectorAll(
		`:is(${status}) svg:not(svg svg)`,
	)) {
		if (!shown(svg)) continue;
		const holder = svg.closest(status)!;
		const kind =
			[...holder.classList].find((c) => c.startsWith("pf-m-"))?.slice(5) ||
			label(holder).split(".")[1];
		const where = `${label(holder)} near ${snippet(holder)}`;
		const b = background(svg);
		if (b.bg === undefined) {
			unmeasured.push({ kind: "icon", label: where, why: b.why });
			continue;
		}
		const cs = getComputedStyle(svg);
		const fg = over(rgba(cs.fill.startsWith("rgb") ? cs.fill : cs.color), b.bg);
		icons.push({
			kind,
			label: where,
			ratio: ratio(fg, b.bg),
			need: 3,
			fg: hex(fg),
			bg: hex(b.bg),
		});
	}

	const targets = [
		...document.querySelectorAll<HTMLElement>(
			'a[href], button, input:not([type="hidden"]), select, textarea, ' +
				'summary, [role="button"], [role="link"], [role="checkbox"], [role="radio"], [role="switch"], [role="tab"], ' +
				'[role="menuitem"], [tabindex]:not([tabindex="-1"])',
		),
	]
		.filter((e) => shown(e) && !e.matches(':disabled, [aria-disabled="true"]'))
		.map((e) => ({ e, r: e.getBoundingClientRect() }));
	const inline = (e: HTMLElement) => {
		if (getComputedStyle(e).display !== "inline" || !e.parentElement)
			return false;
		let rest = e.parentElement.textContent || "";
		for (const t of e.parentElement.querySelectorAll("a, button"))
			rest = rest.replace(t.textContent || "", "");
		return rest.trim() !== "";
	};
	const small = targets.filter(
		(t) => (t.r.width < 24 || t.r.height < 24) && !inline(t.e),
	);
	const centre = (r: DOMRect) => [r.left + r.width / 2, r.top + r.height / 2];
	const hits = (t: Target, u: Target) => {
		if (t.e.contains(u.e) || u.e.contains(t.e)) return false;
		const [cx, cy] = centre(t.r),
			[ux, uy] = centre(u.r);
		const dx = Math.max(u.r.left - cx, 0, cx - u.r.right),
			dy = Math.max(u.r.top - cy, 0, cy - u.r.bottom);
		return (
			dx * dx + dy * dy < 144 ||
			(small.includes(u) && Math.hypot(cx - ux, cy - uy) < 24)
		);
	};
	const tiny = [];
	for (const t of small) {
		const u = targets.find((u) => u !== t && hits(t, u));
		if (u)
			tiny.push({
				label: `${label(t.e)} ${snippet(t.e)}`,
				w: t.r.width,
				h: t.r.height,
				near: `${label(u.e)} ${snippet(u.e)}`,
			});
	}

	const main = document.querySelector("main.pf-v6-c-page__main");
	const de = document.documentElement;
	return {
		dark: de.classList.contains("pf-v6-theme-dark"),
		text,
		icons,
		unmeasured,
		tiny,
		overflow: {
			doc: de.scrollWidth - de.clientWidth,
			main: main ? main.scrollWidth - main.clientWidth : 0,
		},
	};
}

export type AccessibilityMeasurement = ReturnType<typeof measureAccessibility>;

export async function auditAccessibility(page: Page, errors: string[] = []) {
	const measurement = await page.evaluate(measureAccessibility);
	const failures: string[] = [];
	for (const [kind, rows] of [
		["text", measurement.text],
		["icon", measurement.icons],
	] as const) {
		for (const row of rows) {
			if (row.ratio < row.need)
				failures.push(
					`${kind}: ${row.ratio.toFixed(2)} < ${row.need}, ${row.fg} on ${row.bg}: ${row.label}`,
				);
		}
	}
	for (const target of measurement.tiny)
		failures.push(
			`target: ${target.w.toFixed(0)}x${target.h.toFixed(0)} ${target.label}, near ${target.near}`,
		);
	for (const [area, pixels] of Object.entries(measurement.overflow))
		if (pixels > 0) failures.push(`overflow: ${area} +${pixels}px`);
	failures.push(...errors.map((error) => `browser: ${error}`));
	return { measurement, failures };
}
