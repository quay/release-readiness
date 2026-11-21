import assert from "node:assert/strict";
import { test } from "node:test";
import type {
	DashboardConfig,
	ReleaseCandidate,
	VerdictItem,
} from "../api/types.ts";
import { blockers } from "./blockers.ts";

const item = (i: Partial<VerdictItem>): VerdictItem => ({
	level: "blocker",
	operator: "",
	owner: "",
	code: "",
	text: "",
	...i,
});

const candidate = (items: VerdictItem[]) =>
	({
		verdict: { state: "not_ready", items },
		fbc_candidate: null,
	}) as unknown as ReleaseCandidate;

const of = (items: VerdictItem[]) =>
	blockers(candidate(items), "quay-v3.17.6", "quay-3-17", undefined);

test("an unknown code falls back to the item's text, with no action", () => {
	const [b] = of([item({ code: "new_code", text: "Something new" })]);
	assert.equal(b.what, "Something new");
	assert.equal(b.action, "");
	assert.equal(b.operator, "");
});

const withOperator = (o: object, items: VerdictItem[]) =>
	blockers(
		{
			verdict: { state: "not_ready", items },
			fbc_candidate: { operators: [{ conflicts: [], ...o }] },
		} as unknown as ReleaseCandidate,
		"quay-v3.17.6",
		"quay-3-17",
		{ konflux_ui_url: "https://k", konflux_namespace: "ns" } as DashboardConfig,
	);

test("stage failures retain task-specific explanations and OCP release links", () => {
	const op = "quay-operator";
	const gs = withOperator(
		{
			operator: op,
			members: ["4.17", "4.21"].map((ocp) => ({
				ocp,
				release: { name: `rel-${ocp}` },
			})),
		},
		[
			["4.17", "verify-conforma"],
			["4.21", "apply-mapping"],
		].map(([ocp, task]) =>
			item({
				code: "stage_release_failed",
				operator: op,
				text: `${op} OCP ${ocp}: stage release failed (${task})`,
			}),
		),
	);
	assert.deepEqual(
		gs.map((g) => [g.why, g.ocps, g.links.map((l) => [l.text, l.href])]),
		[
			[
				"Enterprise Contract policy failed",
				["4.17"],
				[
					[
						"Release OCP 4.17",
						"https://k/ns/ns/applications/quay-3-17/releases/rel-4.17",
					],
				],
			],
			[
				"Mapping config failed",
				["4.21"],
				[
					[
						"Release OCP 4.21",
						"https://k/ns/ns/applications/quay-3-17/releases/rel-4.21",
					],
				],
			],
		],
	);
});

test("mixed builds names the OCPs to rebuild and the head bundle", () => {
	const op = "quay-operator";
	const [b] = withOperator(
		{
			operator: op,
			members: [],
			head_bundle_nvr: "quay-operator-bundle-v3.17.6-202610071230.p0.el9",
		},
		[
			item({
				code: "mixed_builds",
				operator: op,
				text: `${op} OCP 4.17, 4.18: on another bundle`,
			}),
		],
	);
	assert.equal(b.action, "Rebuild OCP 4.17, 4.18 on the Oct 7 12:30 bundle");
});
