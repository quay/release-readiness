import type { BrowserContextOptions, Page, Request } from "@playwright/test";
import { expect } from "@playwright/test";

export const launchOptions = process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE
	? { executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE }
	: {};

export function reviewContext(
	width: number,
	theme: "light" | "dark",
): BrowserContextOptions {
	return {
		viewport: { width, height: 1000 },
		locale: "en-US",
		timezoneId: "America/New_York",
		colorScheme: theme,
		reducedMotion: "reduce",
	};
}

export function collectBrowserErrors(page: Page): string[] {
	const errors: string[] = [];
	page.on("console", (message) => {
		if (message.type() === "error") errors.push(message.text());
	});
	page.on("pageerror", (error) => errors.push(error.message));
	return errors;
}

export async function settleLayout(page: Page) {
	await page.evaluate(async () => {
		await document.fonts.ready;
		await new Promise<void>((resolve) =>
			requestAnimationFrame(() => requestAnimationFrame(() => resolve())),
		);
	});
}

/** Wait for the app's initial requests and rendered content, not arbitrary sleeps. */
export async function openApp(page: Page, url: string) {
	const pending = new Set<Request>();
	const onRequest = (request: Request) => {
		if (new URL(request.url()).pathname.startsWith("/api/"))
			pending.add(request);
	};
	const onDone = (request: Request) => pending.delete(request);
	page.on("request", onRequest);
	page.on("requestfinished", onDone);
	page.on("requestfailed", onDone);
	try {
		const response = await page.goto(url, { waitUntil: "domcontentloaded" });
		if (!response || response.status() >= 400)
			throw new Error(
				`Page failed to load: HTTP ${response?.status() ?? "unknown"}`,
			);
		await expect(page.locator("main").first()).toBeVisible();
		await expect(
			page.locator("main").getByRole("heading").first(),
		).toBeVisible();
		await expect(page.locator(".pf-v6-c-spinner")).toHaveCount(0);
		await expect.poll(() => pending.size).toBe(0);
		await settleLayout(page);
	} finally {
		page.off("request", onRequest);
		page.off("requestfinished", onDone);
		page.off("requestfailed", onDone);
	}
}

export async function assertTheme(page: Page, theme: "light" | "dark") {
	await expect
		.poll(() =>
			page
				.locator("html")
				.evaluate((el) => el.classList.contains("pf-v6-theme-dark")),
		)
		.toBe(theme === "dark");
}
