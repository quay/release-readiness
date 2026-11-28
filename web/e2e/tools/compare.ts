import { mkdir, readFile } from "node:fs/promises";
import { dirname, resolve } from "node:path";
import { chromium } from "@playwright/test";
import { launchOptions } from "../helpers/browser.ts";

type Mark = {
	n: string | number;
	x: number;
	y: number;
	w: number;
	h: number;
	side?: "before" | "after";
};
type Panel = { img: string; title: string; marks?: Mark[] };
type Spec = {
	out: string;
	sheet?: Panel[];
	before?: string;
	after?: string;
	title_before?: string;
	title_after?: string;
	marks?: Mark[];
};
const GAP = 24,
	TITLE = 40,
	BADGE = 28;

try {
	const paths = process.argv.slice(2);
	if (!paths.length || paths.includes("--help")) {
		console.log(
			"compare SPEC.json [SPEC.json ...] — paths inside each spec resolve relative to that spec",
		);
		process.exit(paths.length ? 0 : 1);
	}
	const browser = await chromium.launch(launchOptions);
	try {
		for (const path of paths) {
			const spec = JSON.parse(await readFile(path, "utf8")) as Spec;
			if (!spec || typeof spec.out !== "string" || !spec.out)
				throw new Error("Spec needs an output path");
			if (spec.marks?.some((m) => m.side !== "before" && m.side !== "after"))
				throw new Error("Mark needs before/after side");
			const panels =
				spec.sheet ??
				(["before", "after"] as const).map((side) => ({
					img: spec[side]!,
					title: spec[`title_${side}`]!,
					marks: (spec.marks ?? []).filter((m) => m.side === side),
				}));
			if (!Array.isArray(panels) || !panels.length)
				throw new Error("Sheet must not be empty");
			const loaded = await Promise.all(
				panels.map(async (panel) => {
					if (typeof panel.img !== "string" || typeof panel.title !== "string")
						throw new Error("Panel needs image and title");
					const png = await readFile(resolve(dirname(path), panel.img));
					if (
						png.length < 24 ||
						!png
							.subarray(0, 8)
							.equals(Buffer.from([137, 80, 78, 71, 13, 10, 26, 10])) ||
						png.toString("ascii", 12, 16) !== "IHDR"
					)
						throw new Error(`Not a PNG: ${panel.img}`);
					const width = png.readUInt32BE(16),
						height = png.readUInt32BE(20);
					for (const mark of panel.marks ?? []) {
						if (
							![mark.x, mark.y, mark.w, mark.h].every(Number.isFinite) ||
							mark.x < 0 ||
							mark.y < 0 ||
							mark.w <= 0 ||
							mark.h <= 0 ||
							mark.x + mark.w > width ||
							mark.y + mark.h > height ||
							!["number", "string"].includes(typeof mark.n)
						)
							throw new Error(`Invalid mark outside ${panel.img}`);
					}
					return {
						...panel,
						width,
						height,
						src: `data:image/png;base64,${png.toString("base64")}`,
					};
				}),
			);
			const width =
				loaded.reduce((n, p) => n + p.width, 0) + GAP * (loaded.length - 1);
			const height = TITLE + Math.max(...loaded.map((p) => p.height));
			if (width < 1 || height <= TITLE || width > 30_000 || height > 30_000)
				throw new Error("Composite dimensions exceed limits");
			const context = await browser.newContext({ viewport: { width, height } });
			try {
				const page = await context.newPage();
				await page.setContent("<!doctype html><html><body></body></html>");
				await page.addStyleTag({
					content: `body {margin:0;background:white;display:flex;gap:${GAP}px;align-items:flex-start} .title {height:${TITLE}px;line-height:${TITLE}px;padding:0 12px;box-sizing:border-box;background:#151515;color:white;font:bold 18px sans-serif;white-space:nowrap;overflow:hidden} .shot {position:relative} img {display:block} .box {position:absolute;border:3px solid #ff00ff;box-sizing:border-box} .badge {position:absolute;width:${BADGE}px;height:${BADGE}px;border-radius:50%;background:#ff00ff;color:white;font:bold 16px/${BADGE}px sans-serif;text-align:center;box-shadow:0 0 0 2px #000}`,
				});
				await page.evaluate(
					async ({ panels, badge }) => {
						for (const panel of panels) {
							const wrapper = document.createElement("div");
							wrapper.style.cssText = `flex:none;width:${panel.width}px`;
							const title = document.createElement("div");
							title.className = "title";
							title.textContent = panel.title;
							const shot = document.createElement("div");
							shot.className = "shot";
							const image = document.createElement("img");
							image.src = panel.src;
							shot.append(image);
							for (const mark of panel.marks ?? []) {
								const box = document.createElement("div");
								box.className = "box";
								box.style.cssText = `left:${mark.x}px;top:${mark.y}px;width:${mark.w}px;height:${mark.h}px`;
								const number = document.createElement("div");
								number.className = "badge";
								number.textContent = String(mark.n);
								const x =
									mark.y >= badge || mark.x < badge ? mark.x : mark.x - badge;
								const y = mark.y >= badge ? mark.y - badge : mark.y;
								number.style.cssText = `left:${x}px;top:${y}px`;
								shot.append(box, number);
							}
							wrapper.append(title, shot);
							document.body.append(wrapper);
							await image.decode();
						}
					},
					{ panels: loaded, badge: BADGE },
				);
				const output = resolve(dirname(path), spec.out);
				await mkdir(dirname(output), { recursive: true });
				await page.screenshot({ path: output });
				console.log(`${output} ${width}x${height}`);
			} finally {
				await context.close();
			}
		}
	} finally {
		await browser.close();
	}
} catch (error) {
	console.error(error instanceof Error ? error.message : error);
	process.exitCode = 1;
}
