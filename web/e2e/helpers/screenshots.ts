import { mkdir } from "node:fs/promises";
import { dirname } from "node:path";
import type { Page } from "@playwright/test";
import { settleLayout } from "./browser.ts";

/** PatternFly scrolls main independently; fullPage alone misses its overflow. */
export async function captureFullPage(page: Page, output: string) {
	const original = page.viewportSize();
	if (!original) throw new Error("Screenshot capture requires a viewport");
	await mkdir(dirname(output), { recursive: true });
	try {
		for (let attempt = 0; attempt < 5; attempt++) {
			await settleLayout(page);
			const grow = await page
				.locator("main.pf-v6-c-page__main")
				.evaluateAll((elements) => {
					const main = elements[0];
					return main ? main.scrollHeight - main.clientHeight : 0;
				});
			if (grow <= 0) {
				await page.screenshot({
					path: output,
					fullPage: true,
					animations: "disabled",
				});
				return;
			}
			const height = page.viewportSize()!.height + grow;
			if (height > 30_000)
				throw new Error("Page exceeds the 30,000px screenshot limit");
			await page.setViewportSize({ width: original.width, height });
		}
		throw new Error(
			"PatternFly scroll area did not stabilize for a full-page screenshot",
		);
	} finally {
		await page.setViewportSize(original);
	}
}

export async function cardDiagnostics(page: Page) {
	return page.evaluate(() => ({
		overflow: {
			doc:
				document.documentElement.scrollWidth -
				document.documentElement.clientWidth,
			main: (() => {
				const main = document.querySelector("main.pf-v6-c-page__main");
				return main ? main.scrollWidth - main.clientWidth : 0;
			})(),
		},
		cards: [
			...document.querySelectorAll<HTMLElement>("main .pf-v6-c-card"),
		].map((card) => ({
			// biome-ignore lint/performance/useDomNodeTextContent: Review output needs rendered text, excluding hidden content.
			text: card.innerText,
			overflow: [...card.querySelectorAll("*")]
				.filter(
					(el) =>
						el.scrollWidth > el.clientWidth + 1 &&
						getComputedStyle(el).overflowX !== "visible",
				)
				.map((el) => el.getAttribute("class") || el.tagName),
		})),
	}));
}
