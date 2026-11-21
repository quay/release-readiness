import { join } from "node:path";
import { chromium } from "@playwright/test";
import {
	assertTheme,
	collectBrowserErrors,
	launchOptions,
	openApp,
	reviewContext,
} from "../helpers/browser.ts";
import { captureFullPage, cardDiagnostics } from "../helpers/screenshots.ts";
import { mockRoutes, pageURL, reviewOptions } from "./options.ts";

try {
	const options = reviewOptions(true);
	const browser = await chromium.launch(launchOptions);
	try {
		for (const path of options.paths) {
			for (const width of options.widths) {
				const theme = options.themes[0];
				const context = await browser.newContext(reviewContext(width, theme));
				try {
					await mockRoutes(context, options.routes);
					const page = await context.newPage();
					const errors = collectBrowserErrors(page);
					await openApp(page, pageURL(options.base, path));
					await assertTheme(page, theme);
					const diagnostics = await cardDiagnostics(page);
					// Width and theme avoid overwriting a previous review's capture.
					const name =
						path.replace(/[^a-zA-Z0-9._-]/g, "_").replace(/^_+|_+$/g, "") ||
						"overview";
					const output = join(options.output!, `${name}-${theme}-${width}.png`);
					await captureFullPage(page, output);
					console.log(
						JSON.stringify(
							{ output, width, theme, errors, ...diagnostics },
							null,
							2,
						),
					);
					if (
						errors.length ||
						Object.values(diagnostics.overflow).some((n) => n > 0) ||
						diagnostics.cards.some((c) => c.overflow.length)
					)
						process.exitCode = 1;
				} catch (error) {
					console.error(
						`${path} at ${width}px: ${error instanceof Error ? error.message : error}`,
					);
					process.exitCode = 1;
				} finally {
					await context.close();
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
