import assert from "node:assert/strict";
import test from "node:test";
import type { ReleaseOverview } from "../api/types.ts";
import { groupStreams, releaseStage, stageReleases } from "./portfolio.ts";

function release(
	name: string,
	published = false,
	next = false,
): ReleaseOverview {
	return {
		release: { name, released: published },
		published,
		shipped: published,
		next_in_stream: next,
		readiness: { signal: "", message: "" },
		shipment_mr: null,
	};
}
test("cards group x.y and retain exact next and historical z-streams", () => {
	const data = [
		release("quay-v3.18.10", false, true),
		release("quay-v3.18.9", true),
		release("quay-v3.18.8", true),
		release("omr-v3.18.1", false, true),
	];
	const groups = groupStreams(data);
	assert.equal(groups.length, 2);
	const quay = groups.find((g) => g.product === "quay")!;
	assert.equal(quay.version, "3.18");
	assert.equal(quay.featured.release.name, "quay-v3.18.10");
	assert.equal(quay.latest?.release.name, "quay-v3.18.9");
	assert.equal(quay.releases.length, 3);
	assert.equal(stageReleases(data, false).length, 3);
	assert.equal(stageReleases(data, true).length, 4);
});
test("JIRA released alone does not mean customer available", () => {
	const jira = release("quay-v3.18.3");
	jira.release.released = true;
	jira.shipped = true;
	assert.equal(releaseStage(jira), "validation");
	assert.equal(stageReleases([jira], false).length, 1);
	jira.published = true;
	assert.equal(releaseStage(jira), "available");
});
test("streams without another planned patch remain discoverable", () => {
	const data = [release("quay-v3.9.28", true), release("quay-v3.9.27", true)];
	assert.equal(groupStreams(data)[0].featured.release.name, "quay-v3.9.28");
	assert.deepEqual(
		stageReleases(data, false).map((r) => r.release.name),
		["quay-v3.9.28"],
	);
});
