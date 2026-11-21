import type { Validation } from "../api/types";

export default function ValidationProgress({ data }: { data?: Validation }) {
	if (!data?.plan_known)
		return (
			<div className="rr-validation-progress">
				{/* An absence, not a state: a dash, with the reason in a
				 * tooltip. The masthead sync popover carries the source
				 * detail behind it. */}
				<p className="rr-muted" title="CI configuration unavailable">
					&mdash;
				</p>
			</div>
		);
	if (data.total === 0)
		return (
			<div className="rr-validation-progress">
				<p className="rr-muted">No CI configured</p>
			</div>
		);
	const values = [
		["passed", data.passed],
		["failed", data.failed],
		["running", data.running],
		["missing", data.missing],
	] as const;
	return (
		<div className="rr-validation-progress">
			<p>
				<strong>
					{data.passed} of {data.total}
				</strong>{" "}
				required jobs passed for the selected FBC
			</p>
			<div
				className="rr-progress-track"
				role="img"
				aria-label={values.map(([label, n]) => `${n} ${label}`).join(", ")}
			>
				{values.map(([label, n]) => (
					<span
						key={label}
						className={label}
						style={{ width: `${data.total ? (n / data.total) * 100 : 0}%` }}
					/>
				))}
			</div>
			<p className="rr-muted">
				{data.failed} failed · {data.running} running · {data.missing} without a
				matching selected-FBC run
			</p>
			{!data.source_fresh && (
				<p className="rr-muted">Required sources are not up to date.</p>
			)}
		</div>
	);
}
