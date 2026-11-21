import type { Page } from "@playwright/test";

const names = ["quay-v3.18.4", "quay-v3.18.3", "quay-v3.18.2", "omr-v2.0.13"];
export const validation = {
	plan_known: true,
	revision: "reviewed-plan",
	source_fresh: true,
	ready: false,
	total: 2,
	passed: 1,
	failed: 0,
	running: 0,
	missing: 1,
	rows: [
		{
			job: "job-a",
			display_name: "Install · FIPS",
			ocp: "4.18",
			cloud: "AWS",
			storage: "S3",
			architecture: "amd64",
			mode: "Operator",
			source_url: "https://github.com/openshift/release",
			state: "passed",
			run_url: "https://prow.example/run/1",
		},
		{
			job: "job-b",
			display_name: "Install · ARM64",
			ocp: "4.19",
			cloud: "GCP",
			storage: "GCS",
			architecture: "arm64",
			mode: "Operator",
			source_url: "https://github.com/openshift/release",
			state: "missing",
			evidence:
				"The latest observed run tested a different FBC; no verified match was found.",
			run_url: "https://prow.example/run/other",
		},
	],
};
export const releases = names.map((name, index) => ({
	release: {
		name,
		released: index === 1 || index === 2,
		release_ticket_key: `PROJQUAY-${100 + index}`,
		konflux_application: name.startsWith("quay") ? "quay-3-18" : "omr-2-0",
	},
	published: index === 1 || index === 2,
	shipped: index === 1 || index === 2,
	next_in_stream: index === 0 || index === 3,
	shipment_mr: null,
	readiness: { signal: "", message: "" },
	portfolio: {
		stage: index === 1 || index === 2 ? "available" : "validation",
		verdict: { state: "not_ready", items: [] },
		validation,
	},
}));

export async function mockPortfolio(page: Page) {
	await page.route("**/api/v1/**", async (route) => {
		const path = new URL(route.request().url()).pathname;
		if (path.endsWith("/config"))
			return route.fulfill({
				json: {
					app_version: "2.0.0-dev",
					jira_base_url: "https://jira.example",
					jira_project: "PROJQUAY",
					jira_enabled: true,
					konflux_ui_url: "https://konflux.example",
					konflux_namespace: "art-quay-tenant",
				},
			});
		if (path.endsWith("/sync-status"))
			return route.fulfill({ json: { sources: [], problems: [] } });
		if (path.endsWith("/overview")) return route.fulfill({ json: releases });
		if (path.endsWith("/validation"))
			return route.fulfill({ json: validation });
		if (path.endsWith("/issues"))
			return route.fulfill({
				json: [
					{
						key: "PROJQUAY-999",
						summary: "Improve registry availability",
						status: "Release Pending",
						issue_type: "Bug",
						link: "https://jira.example/browse/PROJQUAY-999",
					},
				],
			});
		if (path.endsWith("/ci")) return route.fulfill({ json: [] });
		if (path.endsWith("/candidate"))
			return route.fulfill({
				json: {
					shipped: path.includes("3.18.3"),
					candidate: null,
					reason: "No staged candidate",
					// The in-flight release has a selected FBC; shipped ones do not.
					fbc_candidate: path.includes("3.18.4")
						? {
								source: "stage_snapshots",
								state: "complete",
								pullspecs: ["quay.io/ocp-art-tenant/art-fbc@sha256:selected"],
								operators: [
									{
										operator: "quay-operator",
										head_bundle_nvr: "quay-operator-bundle-3.18.4",
										fbc_ri_stage: "none",
										image_check: { bundle_digest: "sha256:bundle" },
										images: [],
										conflicts: [],
										missing_ocp: [],
										members: [
											{
												nvr: "quay-operator-fbc-3.18.4-selected",
												digest: "sha256:selected",
												pullspec:
													"quay.io/ocp-art-tenant/art-fbc@sha256:selected",
												build_url: "",
												head_bundle_nvr: "quay-operator-bundle-3.18.4",
												head_bundle_digest: "sha256:bundle",
												ocp: "4.18",
												release: null,
												ci: null,
											},
										],
									},
								],
							}
						: null,
					fbc_stream: null,
					verdict: { state: "not_ready", items: [] },
					reading: { read: 0, failed: 0, pending: 0, history_days: 0 },
				},
			});
		const release = releases.find((r) => path.endsWith(r.release.name));
		if (release)
			return route.fulfill({
				json: {
					...release.release,
					published: release.published,
					shipment_mr: null,
				},
			});
		return route.fulfill({ status: 404, json: { error: "not found" } });
	});
}
