import type { ReleaseOverview } from "../api/types";

export type PortfolioStage = "validation" | "shipment" | "available";
export function streamIdentity(name: string) {
	const match = name.match(/^([a-z]+)-v(\d+)\.(\d+)(?:\.|$)/i);
	return match
		? {
				key: `${match[1]}-${match[2]}.${match[3]}`,
				product: match[1].toLowerCase(),
				version: `${match[2]}.${match[3]}`,
			}
		: { key: name, product: "other", version: name };
}
export function releaseStage(ov: ReleaseOverview): PortfolioStage {
	if (ov.published) return "available";
	return ov.portfolio?.validation.ready ? "shipment" : "validation";
}
export function groupStreams(releases: ReleaseOverview[]) {
	const groups = new Map<
		string,
		{
			key: string;
			product: string;
			version: string;
			releases: ReleaseOverview[];
		}
	>();
	for (const ov of releases) {
		const identity = streamIdentity(ov.release.name);
		const group = groups.get(identity.key) ?? { ...identity, releases: [] };
		group.releases.push(ov);
		groups.set(identity.key, group);
	}
	return [...groups.values()]
		.map((group) => {
			group.releases.sort((a, b) =>
				b.release.name.localeCompare(a.release.name, undefined, {
					numeric: true,
				}),
			);
			const next = group.releases.find((r) => r.next_in_stream);
			const latest = group.releases.find((r) => r.published);
			return {
				...group,
				next,
				latest,
				featured: next ?? latest ?? group.releases[0],
			};
		})
		.sort(
			(a, b) =>
				a.product.localeCompare(b.product) ||
				b.version.localeCompare(a.version, undefined, { numeric: true }),
		);
}
export function stageReleases(releases: ReleaseOverview[], history: boolean) {
	if (history) return groupStreams(releases).flatMap((g) => g.releases);
	return groupStreams(releases).flatMap((g) =>
		g.releases.filter(
			(r) =>
				r === g.featured ||
				r === g.latest ||
				(!r.shipped && !r.release.released && !r.published),
		),
	);
}
export function portfolioLabel(ov: ReleaseOverview) {
	if (ov.published) return "Available to customers";
	if (ov.shipped || ov.release.released) return "Publication unconfirmed";
	if (ov.portfolio?.error) return "Evidence unavailable";
	if (ov.portfolio?.validation.ready) return "Ready for shipment";
	if (
		ov.portfolio?.verdict.items.some((i) => i.level === "blocker") ||
		ov.portfolio?.validation.failed
	)
		return "Needs attention";
	return "Validation in progress";
}
