import { chromium } from "@playwright/test";
import { auditAccessibility } from "../helpers/accessibility.ts";
import {
	assertTheme,
	collectBrowserErrors,
	launchOptions,
	openApp,
	reviewContext,
} from "../helpers/browser.ts";
import { mockRoutes, pageURL, reviewOptions } from "./options.ts";

try {
	const options = reviewOptions(false);
	const browser = await chromium.launch(launchOptions);
	try {
		for (const path of options.paths) {
			for (const width of options.widths) {
				for (const theme of options.themes) {
					const context = await browser.newContext(reviewContext(width, theme));
					try {
						await mockRoutes(context, options.routes);
						const page = await context.newPage();
						const errors = collectBrowserErrors(page);
						const url = pageURL(options.base, path);
						await openApp(page, url);
						await assertTheme(page, theme);
						const report = await auditAccessibility(page, errors);
						console.log(
							JSON.stringify({ url, width, theme, ...report }, null, 2),
						);
						if (report.failures.length) process.exitCode = 1;
					} catch (error) {
						console.error(
							`${path} at ${width}px ${theme}: ${error instanceof Error ? error.message : error}`,
						);
						process.exitCode = 1;
					} finally {
						await context.close();
					}
				}
			}
		}
	} finally {
		await browser.close();
	}
} catch (error) {
	console.error(error instanceof Error ? error.message : error);
	process.exitCode = 1;
}
