import { expect, test, type Page } from "@playwright/test";

const version = "quay-v3.18.1";
const mrURL = "https://gitlab.example/merge_requests/948";

const config = {
	jira_base_url: "https://jira.example",
	jira_project: "PROJQUAY",
	jira_enabled: true,
	konflux_ui_url: "",
	konflux_namespace: "",
};

const candidate = {
	shipped: true,
	candidate: null,
	reason: "",
	fbc_candidate: null,
	fbc_stream: null,
	verdict: { state: "shipped", items: [] },
	reading: { read: 0, failed: 0, pending: 0, history_days: 0 },
};

async function mockDashboard(
	page: Page,
	mr: { state: "opened" | "merged"; stage: "prod_released" | "shipped" },
) {
	await page.route("**/api/v1/config", (route) => route.fulfill({ json: config }));
	await page.route("**/api/v1/sync-status", (route) =>
		route.fulfill({ json: { problems: [], sources: [] } }),
	);
	await page.route("**/api/v1/releases/overview", (route) =>
		route.fulfill({
			json: [
				{
					release: { name: version, released: true },
					readiness: { signal: "green", message: "Released" },
					next_in_stream: true,
					shipment_mr: {
						iid: 948,
						web_url: mrURL,
						state: mr.state,
						draft: false,
						stage: mr.stage,
						pipeline_failed: false,
						needs_approval: false,
						source: "pointer",
					},
				},
			],
		}),
	);
	await page.route(`**/api/v1/releases/${version}/candidate`, (route) =>
		route.fulfill({ json: candidate }),
	);
}

test("shows an open shipment MR as unshipped even after the version releases", async ({
	page,
}, testInfo) => {
	await mockDashboard(page, { state: "opened", stage: "prod_released" });

	await page.goto("/");

	const shipment = page.locator(`a[href="${mrURL}"]`);
	await expect(shipment).toHaveText("MR open");
	await expect(page.getByText("Production release succeeded")).toHaveCount(0);
	await expect(shipment).not.toHaveText("MR shipped");
	await expect(page.getByText("Shipment MR: shipped", { exact: true })).toHaveCount(
		0,
	);
	await page.screenshot({
		path: testInfo.outputPath("open-shipment-mr.png"),
		fullPage: true,
	});
});

test("shows a shipment MR as shipped after it merges", async ({ page }, testInfo) => {
	await mockDashboard(page, { state: "merged", stage: "shipped" });

	await page.goto("/");

	await expect(page.locator(`a[href="${mrURL}"]`)).toHaveText("MR shipped");
	await page.screenshot({
		path: testInfo.outputPath("merged-shipment-mr.png"),
		fullPage: true,
	});
});
