import { expect, test } from "@playwright/test";
import { mockPortfolio, releases, validation } from "./fixtures/portfolio.ts";
import { captureFullPage } from "./helpers/screenshots.ts";

test.beforeEach(async ({ page }) => mockPortfolio(page));

test("release table groups z-streams into stages and preserves navigation", async ({
	page,
}, testInfo) => {
	const errors: string[] = [];
	page.on("pageerror", (e) => errors.push(e.message));
	await page.goto("/");
	await expect(
		page.getByRole("heading", { name: "Release portfolio", exact: true }),
	).toBeVisible();
	// In-flight releases only; released ones wait for history.
	await expect(
		page.locator("table[aria-label='Releases'] tbody tr"),
	).toHaveCount(2);
	await expect(
		page.getByRole("link", { name: "Quay v3.18.4", exact: true }),
	).toBeVisible();
	await expect(
		page.getByRole("link", { name: "Quay v3.18.3", exact: true }),
	).toHaveCount(0);
	await expect(page.getByText("Mock evidence")).toHaveCount(0);
	await page.getByRole("button", { name: "Stages", exact: true }).click();
	await expect(page.locator(".rr-stage-grid article")).toHaveCount(3);
	await page.getByText("Show history", { exact: true }).click();
	await expect(page.locator(".rr-stage-grid article")).toHaveCount(4);
	await page
		.getByRole("heading", { name: "Quay v3.18.3", exact: true })
		.getByRole("link")
		.click();
	await expect(
		page.getByRole("heading", { name: "Shipped FBC record", exact: true }),
	).toBeVisible();
	// A shipped release leads with its tickets on the briefing.
	await expect(
		page.getByRole("link", { name: "PROJQUAY-999", exact: true }),
	).toBeVisible();
	await page
		.getByRole("tab", { name: "Validation workbench", exact: true })
		.click();
	await expect(
		page.getByRole("heading", { name: "Required validation matrix" }),
	).toBeVisible();
	await page
		.getByRole("combobox", { name: "Architecture", exact: true })
		.selectOption("arm64");
	await expect(page.getByText("GCP / GCS")).toBeVisible();
	await expect(
		page.getByRole("link", { name: "Install · ARM64 ↗", exact: true }),
	).toBeVisible();
	await expect(
		page.getByText(
			"The latest observed run tested a different FBC; no verified match was found.",
		),
	).not.toBeVisible();
	await page.locator(".rr-validation-table summary").click();
	await expect(
		page.getByText(
			"The latest observed run tested a different FBC; no verified match was found.",
		),
	).toBeVisible();
	await expect(
		page.getByRole("link", { name: /Open observed run/ }),
	).toHaveAttribute("href", "https://prow.example/run/other");
	await expect(page.getByText("AWS / S3")).toHaveCount(0);
	await page
		.getByRole("tab", { name: "Release briefing", exact: true })
		.click();
	await expect(
		page.getByRole("link", { name: "PROJQUAY-999", exact: true }),
	).toBeVisible();
	await page.reload();
	await expect(
		page.getByRole("tab", { name: "Release briefing", exact: true }),
	).toHaveAttribute("aria-selected", "true");
	await page.getByRole("link", { name: "All releases", exact: true }).click();
	await expect(
		page.getByRole("button", { name: "Stages", exact: true }),
	).toHaveAttribute("aria-pressed", "true");
	await expect(
		page.getByRole("switch", { name: "Show history", exact: true }),
	).toBeChecked();
	await captureFullPage(page, testInfo.outputPath("stages.png"));
	expect(errors).toEqual([]);
});

test("accessible controls, mobile layout and reduced motion", async ({
	page,
}, testInfo) => {
	await page.emulateMedia({ reducedMotion: "reduce" });
	await page.goto("/");
	await page.getByRole("button", { name: "Stages", exact: true }).click();
	expect(await page.evaluate(() => document.getAnimations().length)).toBe(0);
	await page.getByRole("button", { name: "About this dashboard" }).click();
	await expect(
		page.getByText("Release readiness 2.0.0-dev", { exact: true }),
	).toBeVisible();
	await expect(
		page.getByRole("link", { name: "Send feedback or report an issue" }),
	).toHaveAttribute(
		"href",
		"https://github.com/quay/release-readiness/issues/new",
	);
	await page.keyboard.press("Escape");
	await page.getByRole("button", { name: "Switch to dark theme" }).click();
	await page.reload();
	await expect(
		page.getByRole("button", { name: "Switch to light theme" }),
	).toBeVisible();
	await page.getByRole("button", { name: "Table", exact: true }).click();
	for (const width of [1440, 1024, 736, 390, 320]) {
		await page.setViewportSize({ width, height: 1000 });
		expect(
			await page.evaluate(
				() => document.documentElement.scrollWidth <= innerWidth,
			),
			`No horizontal overflow at ${width}px`,
		).toBe(true);
	}
	await captureFullPage(page, testInfo.outputPath("table-mobile-dark.png"));
});

test("no configured jobs differs from unavailable inventory", async ({
	page,
}) => {
	const empty = { ...validation, total: 0, passed: 0, missing: 0, rows: [] };
	await page.route("**/api/v1/releases/overview?*", (route) =>
		route.fulfill({
			json: releases.map((r) => ({
				...r,
				portfolio: { ...r.portfolio, validation: empty },
			})),
		}),
	);
	await page.route("**/api/v1/releases/*/validation", (route) =>
		route.fulfill({ json: empty }),
	);
	await page.goto("/");
	await expect(
		page.locator("table[aria-label='Releases']").getByText("No CI configured"),
	).toHaveCount(2);
	await page.getByRole("link", { name: "Quay v3.18.4", exact: true }).click();
	await page
		.getByRole("tab", { name: "Validation workbench", exact: true })
		.click();
	await expect(
		page
			.getByRole("tabpanel")
			.getByRole("heading", { name: /No CI configured/ }),
	).toBeVisible();
	await expect(
		page.getByText("CI readiness cannot be established.", { exact: false }),
	).toBeVisible();
	await page.route("**/api/v1/releases/*/validation", (route) =>
		route.fulfill({ json: { ...empty, plan_known: false } }),
	);
	await page.reload();
	await expect(
		page
			.getByRole("tabpanel")
			.getByRole("heading", { name: /CI configuration unavailable/ }),
	).toBeVisible();
});

test("long source-status popover stays within the viewport", async ({
	page,
}, testInfo) => {
	await page.route("**/api/v1/sync-status", (route) =>
		route.fulfill({
			json: {
				sources: [],
				problems: ["jira", "konflux", "prow", "fbc-render", "image-scans"].map(
					(source) => ({
						source,
						message: "Source failure: ".repeat(25),
						since: new Date().toISOString(),
						last_success: null,
					}),
				),
			},
		}),
	);
	for (const [width, height] of [
		[1366, 768],
		[720, 640],
		[390, 560],
	]) {
		await page.setViewportSize({ width, height });
		await page.goto("/");
		await page.getByRole("button", { name: /4 sources stale/ }).click();
		const popup = page.locator(".rr-source-popover:visible");
		await expect(popup).toBeVisible();
		await page.screenshot({
			path: testInfo.outputPath(`source-status-${width}.png`),
		});
		await expect(async () => {
			const box = await popup.boundingBox();
			expect(box).not.toBeNull();
			expect(box!.y).toBeGreaterThanOrEqual(0);
			expect(box!.x).toBeGreaterThanOrEqual(0);
			expect(box!.y + box!.height).toBeLessThanOrEqual(height);
			expect(box!.x + box!.width).toBeLessThanOrEqual(width);
		}).toPass({ timeout: 3000 });
		const content = popup.locator(".pf-v6-c-popover__content");
		expect(
			await content.evaluate((el) => el.scrollHeight > el.clientHeight),
		).toBe(true);
		await page.screenshot({
			path: testInfo.outputPath(`source-status-${width}.png`),
		});
		await page.keyboard.press("Escape");
		await expect(popup).not.toBeVisible();
	}
});

test("confirmed active CI is Running with compact evidence details", async ({
	page,
}) => {
	await page.route("**/api/v1/releases/*/validation", (route) =>
		route.fulfill({
			json: {
				...validation,
				passed: 0,
				running: 1,
				missing: 0,
				total: 1,
				rows: [
					{
						...validation.rows[0],
						state: "running",
						evidence: "The selected-FBC run is still in progress.",
					},
				],
			},
		}),
	);
	await page.goto("/");
	await page.getByRole("link", { name: "Quay v3.18.4", exact: true }).click();
	await page
		.getByRole("tab", { name: "Validation workbench", exact: true })
		.click();
	const table = page.locator(".rr-validation-table");
	await expect(table.getByText("Running", { exact: true })).toBeVisible();
	await expect(
		table.getByText("Awaiting evidence", { exact: true }),
	).toHaveCount(0);
	await expect(
		table.getByText("The selected-FBC run is still in progress."),
	).not.toBeVisible();
	await table.locator("summary").click();
	await expect(
		table.getByText("The selected-FBC run is still in progress."),
	).toBeVisible();
});

test("observed CI success stays separate from selected-FBC readiness", async ({
	page,
}) => {
	await page.route("**/api/v1/releases/*/validation", (route) =>
		route.fulfill({
			json: {
				...validation,
				passed: 0,
				running: 0,
				missing: 2,
				total: 2,
				rows: [
					{
						...validation.rows[0],
						display_name: "Install · ARM64",
						state: "missing",
						ci_result: "passed",
						fbc_match: "newer",
						evidence:
							"This run tested a newer ART FBC than the selected candidate.",
						tested_fbc: { nvr: "newer-build", digest: "sha256:newer" },
						selected_fbc: { nvr: "selected-build", digest: "sha256:selected" },
					},
					{
						...validation.rows[1],
						display_name: "Install · RDS",
						state: "missing",
						run_url: undefined,
						evidence: "No run has been ingested for this configured job.",
					},
				],
			},
		}),
	);
	await page.goto("/releases/quay-v3.18.4?view=workbench");
	const table = page.locator(".rr-validation-table");
	const arm = table.locator("tbody tr").filter({ hasText: "Install · ARM64" });
	await expect(
		arm.getByText("Passed · newer FBC", { exact: true }),
	).toBeVisible();
	// The sentence sits in a tooltip; the label stays compact.
	await expect(
		arm.locator('span[title="No matching run for selected FBC"]'),
	).toHaveCount(1);
	await expect(
		table
			.locator("tbody tr")
			.filter({ hasText: "Install · RDS" })
			.getByText("No run", { exact: true }),
	).toBeVisible();
	await expect(page.getByText("0 of 2", { exact: true })).toBeVisible();
	await arm.locator("summary").click();
	await expect(arm.getByText("sha256:newer", { exact: true })).toBeVisible();
	await expect(arm.getByText("sha256:selected", { exact: true })).toBeVisible();
	await page
		.getByRole("combobox", { name: "CI result", exact: true })
		.selectOption("passed");
	await expect(table.locator("tbody tr")).toHaveCount(1);
	await page
		.getByRole("combobox", { name: "CI result", exact: true })
		.selectOption("unmatched");
	await expect(table.locator("tbody tr")).toHaveCount(2);
});

test("source freshness uses only the shared hook polling interval", async ({
	page,
}) => {
	await page.clock.install();
	let requests = 0;
	await page.route("**/api/v1/sync-status", (route) => {
		requests++;
		return route.fulfill({ json: { sources: [], problems: [] } });
	});
	await page.goto("/");
	await expect(
		page.getByRole("heading", { name: "Release portfolio" }),
	).toBeVisible();
	await expect.poll(() => requests).toBeGreaterThan(0);
	requests = 0;
	await page.clock.runFor(30_000);
	expect(requests).toBe(0);
	await page.clock.runFor(30_000);
	await expect.poll(() => requests).toBeGreaterThan(0);
	// Desktop and compact masthead consumers each poll once per minute.
	expect(requests).toBeLessThanOrEqual(2);
});

test("released releases wait for history in the table, not in stages", async ({
	page,
}) => {
	const published = structuredClone(releases[1]);
	published.release.name = "quay-v3.17.9";
	await page.route("**/api/v1/releases/overview*", (route) =>
		route.fulfill({ json: [...releases, published] }),
	);
	await page.goto("/");
	// The table shows in-flight releases only, so a stream whose every
	// release shipped is hidden until history is enabled.
	await expect(
		page.getByRole("link", { name: "Quay v3.17.9", exact: true }),
	).toHaveCount(0);
	await expect(
		page.getByRole("link", { name: "Quay v3.18.2", exact: true }),
	).toHaveCount(0);
	// Stages still show each stream's latest published z-stream.
	await page.getByRole("button", { name: "Stages", exact: true }).click();
	await expect(
		page
			.locator(".rr-stage-lane.available")
			.getByRole("heading", { name: "Quay v3.17.9", exact: true }),
	).toBeVisible();
	await page.getByRole("button", { name: "Table", exact: true }).click();
	await page.getByText("Show history", { exact: true }).click();
	await expect(
		page.getByRole("link", { name: "Quay v3.17.9", exact: true }),
	).toBeVisible();
	await expect(
		page.getByRole("link", { name: "Quay v3.18.2", exact: true }),
	).toBeVisible();
	await page.getByRole("link", { name: "Quay v3.18.4", exact: true }).click();
	await page.getByRole("link", { name: "All releases", exact: true }).click();
	await expect(
		page.getByRole("switch", { name: "Show history", exact: true }),
	).toBeChecked();
	await page.getByText("Show history", { exact: true }).click();
	await expect(
		page.getByRole("switch", { name: "Show history", exact: true }),
	).not.toBeChecked();
	await expect(
		page.getByRole("link", { name: "Quay v3.18.2", exact: true }),
	).toHaveCount(0);
	await expect(
		page.getByRole("link", { name: "Quay v3.17.9", exact: true }),
	).toHaveCount(0);
});

test("job history scopes FBCs, pages through stored runs and closes back to workbench", async ({
	page,
}, testInfo) => {
	const requests: string[] = [];
	await page.route("**/api/v1/releases/*/ci-history?*", (route) => {
		const url = new URL(route.request().url());
		requests.push(url.search);
		const all = url.searchParams.get("scope") === "all";
		const next = !!url.searchParams.get("cursor");
		return route.fulfill({
			json: {
				selected_fbcs: [
					{
						digest: "sha256:selected",
						nvr: "quay-operator-fbc-3.18.4-selected",
					},
				],
				job: url.searchParams.get("job") ? validation.rows[0] : undefined,
				next_cursor: next ? undefined : "page-two",
				runs: [
					{
						job: "job-a",
						build_id: next ? "2" : "1",
						display_name: "Install · FIPS · AWS / S3",
						state: next ? "failure" : "success",
						started_at: "2026-11-19T03:00:00Z",
						prow_url: `https://prow.example/${next ? "2" : "1"}`,
						fbc_match: "exact",
						fbc: {
							digest: "sha256:selected",
							nvr: "quay-operator-fbc-3.18.4-selected",
						},
					},
					...(all
						? [
								{
									job: "job-a",
									build_id: "3",
									state: "success",
									started_at: "2026-11-18T03:00:00Z",
									prow_url: "https://prow.example/3",
									fbc_match: "different",
									fbc: {
										digest: "sha256:other",
										nvr: "quay-operator-fbc-3.18.4-other",
									},
								},
								{
									job: "job-a",
									build_id: "4",
									state: "pending",
									started_at: "2026-11-17T03:00:00Z",
									prow_url: "https://prow.example/4",
									fbc_match: "unverified",
								},
							]
						: []),
				],
			},
		});
	});
	await page.goto("/releases/quay-v3.18.4?view=workbench");
	expect(requests).toHaveLength(0);
	const open = page.getByRole("link", {
		name: "View history for Install · FIPS · AWS · S3 · OCP 4.18 · amd64",
	});
	await open.click();
	await expect(
		page.getByRole("heading", { name: "Job history", exact: true }),
	).toBeVisible();
	await expect(page.getByText("Selected FBC", { exact: true })).toBeVisible();
	await expect(page.getByText("Other FBC", { exact: true })).toHaveCount(0);
	expect(requests[0]).toContain("job=job-a");
	await page.getByRole("button", { name: "Next history page" }).click();
	await expect(
		page.getByRole("link", { name: /Open Prow run 2:/ }),
	).toBeVisible();
	await page.getByRole("button", { name: "Previous history page" }).click();
	await expect(
		page.getByRole("link", { name: /Open Prow run 1:/ }),
	).toBeVisible();
	await page
		.getByText("Include other and unverified FBCs", { exact: true })
		.click();
	await expect(
		page.getByRole("switch", { name: "Include other and unverified FBCs" }),
	).toBeChecked();
	await expect(page.getByText("Other FBC", { exact: true })).toBeVisible();
	await expect(page.getByText("FBC unverified", { exact: true })).toBeVisible();
	await captureFullPage(page, testInfo.outputPath("job-history.png"));
	await page.keyboard.press("Escape");
	await expect(
		page.getByRole("heading", { name: "Job history", exact: true }),
	).toHaveCount(0);
	await expect(
		page.getByRole("tab", { name: "Validation workbench", exact: true }),
	).toHaveAttribute("aria-selected", "true");
	await page
		.getByRole("button", { name: "All CI history", exact: true })
		.click();
	await expect(
		page.getByRole("heading", { name: "All CI history", exact: true }),
	).toBeVisible();
	// The remounted drawer's first request lands shortly after its heading.
	await expect.poll(() => requests.at(-1) ?? "").not.toContain("job=");
	await page.setViewportSize({ width: 420, height: 850 });
	await expect(
		page.getByRole("button", { name: "Close drawer panel" }),
	).toBeVisible();
	await page.getByRole("button", { name: "Next history page" }).click();
	await expect(
		page.getByRole("link", { name: /Open Prow run 2:/ }),
	).toBeVisible();
	const historyScope = page.getByRole("switch", {
		name: "Include other and unverified FBCs",
	});
	await historyScope.focus();
	await page.keyboard.press("Space");
	await expect(historyScope).toBeChecked();
	await expect(page.getByText("Other FBC", { exact: true })).toBeVisible();
	await captureFullPage(page, testInfo.outputPath("history-mobile.png"));
	await page.keyboard.press("Escape");
	await expect(
		page.getByRole("heading", { name: "Required validation matrix" }),
	).toBeVisible();
});
