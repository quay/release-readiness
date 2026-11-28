import { expect, test } from "@playwright/test";
import { mockPortfolio } from "./fixtures/portfolio.ts";
import { auditAccessibility } from "./helpers/accessibility.ts";
import {
	assertTheme,
	collectBrowserErrors,
	openApp,
} from "./helpers/browser.ts";

for (const theme of ["light", "dark"] as const) {
	for (const width of [1440, 420]) {
		test.describe(`${theme} at ${width}px`, () => {
			test.use({
				colorScheme: theme,
				viewport: { width, height: 1000 },
				reducedMotion: "reduce",
			});
			for (const [name, path] of [
				["portfolio cards", "/"],
				["portfolio stages", "/?layout=stages"],
				["release briefing", "/releases/quay-v3.18.4?view=briefing"],
				["validation workbench", "/releases/quay-v3.18.4?view=workbench"],
			]) {
				test(name, async ({ page }, testInfo) => {
					await mockPortfolio(page);
					const errors = collectBrowserErrors(page);
					await openApp(page, path);
					await assertTheme(page, theme);
					const report = await auditAccessibility(page, errors);
					await testInfo.attach("accessibility-report", {
						body: JSON.stringify(report, null, 2),
						contentType: "application/json",
					});
					expect(report.measurement.text.length).toBeGreaterThan(0);
					expect(report.failures, JSON.stringify(report, null, 2)).toEqual([]);
				});
			}
		});
	}
}

test("audit detects measured failures and reports unmeasured backgrounds", async ({
	page,
}) => {
	await page.setContent(`<body style="background:white;color:black">
		<p style="color:#aaa">Low contrast text</p>
		<p style="background-image:linear-gradient(white,black)">Unmeasured gradient</p>
		<button style="width:10px;height:10px;padding:0;border:0">a</button><button style="width:10px;height:10px;padding:0;border:0">b</button>
		<div style="width:4000px">Overflow</div></body>`);
	const report = await auditAccessibility(page, ["Synthetic browser error"]);
	expect(report.failures.some((failure) => failure.startsWith("text:"))).toBe(
		true,
	);
	expect(report.failures.some((failure) => failure.startsWith("target:"))).toBe(
		true,
	);
	expect(
		report.failures.some((failure) => failure.startsWith("overflow:")),
	).toBe(true);
	expect(report.failures).toContain("browser: Synthetic browser error");
	expect(
		report.measurement.unmeasured.some((item) =>
			item.why.includes("background-image"),
		),
	).toBe(true);
});
