import {
	Alert,
	Button,
	FormSelect,
	FormSelectOption,
	Label,
	Tab,
	Tabs,
	TabTitleText,
	Title,
} from "@patternfly/react-core";
import {
	CodeBranchIcon,
	JiraIcon,
	OpenshiftIcon,
	TasksIcon,
} from "@patternfly/react-icons";
import type { ReactNode } from "react";
import { useState } from "react";
import { Link, useNavigate, useSearchParams } from "react-router-dom";
import { getValidation, listReleaseVersions } from "../api/client";
import type {
	DashboardConfig,
	ReleaseCandidate,
	ReleaseDetail,
	Validation,
	ValidationRow,
} from "../api/types";
import { useCachedFetch } from "../hooks/useCachedFetch";
import {
	ART_PIPELINES_HEALTH_URL,
	formatReleaseName,
	jiraSearchUrl,
	konfluxUrl,
	ticketsJql,
} from "../utils/links";
import { streamIdentity } from "../utils/portfolio";
import LoadError, { Loading } from "./LoadError";
import ValidationProgress from "./ValidationProgress";

export default function ReleaseWorkspace({
	release,
	config,
	candidate,
	facts,
	builds,
	tickets,
	freshness,
}: {
	release: ReleaseDetail;
	config?: DashboardConfig;
	candidate?: ReleaseCandidate;
	facts: ReactNode;
	builds: ReactNode;
	tickets: ReactNode;
	freshness: ReactNode;
}) {
	const [params, setParams] = useSearchParams();
	const navigate = useNavigate();
	const validation = useCachedFetch(`validation:${release.name}`, () =>
		getValidation(release.name),
	);
	const versions = useCachedFetch("releaseVersions", listReleaseVersions);
	const requested = params.get("view");
	const active =
		params.has("ci") || params.has("cell") || requested === "workbench"
			? "workbench"
			: "briefing";
	const select = (view: string) =>
		setParams((prev) => {
			const next = new URLSearchParams(prev);
			next.set("view", view);
			next.delete("ci");
			next.delete("job");
			next.delete("cell");
			return next;
		});
	const stream = streamIdentity(release.name);
	const konflux = konfluxUrl(
		config?.konflux_ui_url || "",
		config?.konflux_namespace || "",
		release.konflux_application || "",
	);
	const jira = config?.jira_base_url
		? jiraSearchUrl(
				ticketsJql(config.jira_project, release.name),
				config.jira_base_url,
			)
		: null;
	const blockers =
		candidate?.verdict.items.filter((i) => i.level === "blocker") || [];
	const published = release.published === true;
	return (
		<div className="rr-workspace">
			<div className="rr-version-nav">
				<div>
					<Title headingLevel="h1" size="3xl">
						{formatReleaseName(release.name)}
					</Title>
					{freshness}
				</div>
				<label className="rr-version-switch">
					Switch release
					<select
						aria-label="Switch release"
						value={release.name}
						onChange={(e) => {
							const next = new URLSearchParams(params);
							next.delete("ci");
							next.delete("job");
							next.delete("cell");
							navigate(
								`/releases/${encodeURIComponent(e.target.value)}?${next}`,
							);
						}}
					>
						{!versions.data?.some((v) => v.release.name === release.name) && (
							<option value={release.name}>
								{formatReleaseName(release.name)}
							</option>
						)}
						{versions.data
							?.slice()
							.sort((a, b) =>
								b.release.name.localeCompare(a.release.name, undefined, {
									numeric: true,
								}),
							)
							.map((v) => (
								<option value={v.release.name} key={v.release.name}>
									{formatReleaseName(v.release.name)}
								</option>
							))}
					</select>
				</label>
			</div>
			<nav aria-label="Release sources" className="rr-source-links">
				{stream.product === "quay" && (
					<a
						href={`https://sippy.dptools.openshift.org/sippy-ng/release/quay-${stream.version}`}
						target="_blank"
						rel="noreferrer"
					>
						<TasksIcon aria-hidden />
						Sippy ↗
					</a>
				)}
				{konflux && (
					<a href={konflux} target="_blank" rel="noreferrer">
						<OpenshiftIcon aria-hidden />
						Konflux ↗
					</a>
				)}
				<a href={ART_PIPELINES_HEALTH_URL} target="_blank" rel="noreferrer">
					<CodeBranchIcon aria-hidden />
					ART pipelines ↗
				</a>
				{jira && (
					<a href={jira} target="_blank" rel="noreferrer">
						<JiraIcon aria-hidden />
						JIRA ↗
					</a>
				)}
			</nav>
			<Tabs
				activeKey={active}
				onSelect={(_, key) => select(String(key))}
				aria-label="Release sections"
				mountOnEnter
				unmountOnExit
			>
				<Tab
					eventKey="briefing"
					title={<TabTitleText>Release briefing</TabTitleText>}
				>
					<section className="rr-record">
						<h2>Release coordination</h2>
						{facts}
					</section>
					{blockers.length > 0 && !published && (
						<section className="rr-record">
							<h2>What needs to happen next</h2>
							<ul>
								{blockers.map((b) => (
									<li key={`${b.code}-${b.operator}-${b.text}`}>
										<strong>{b.owner || "Release team"}</strong> · {b.text}
									</li>
								))}
							</ul>
						</section>
					)}
					{published ? (
						// A shipped release leads with its tickets; the FBC
						// record folds into a one-line count.
						<>
							{tickets}
							<FBCRecord
								candidate={candidate}
								shippedAt={release.release_date}
							/>
						</>
					) : (
						<>
							{builds}
							{tickets}
						</>
					)}
				</Tab>
				<Tab
					eventKey="workbench"
					title={<TabTitleText>Validation workbench</TabTitleText>}
				>
					<ValidationWorkbench
						data={validation.data}
						error={validation.error}
						retry={validation.refetch}
					/>
				</Tab>
			</Tabs>
		</div>
	);
}

function ValidationWorkbench({
	data,
	error,
	retry,
}: {
	data?: Validation;
	error?: Error;
	retry: () => void;
}) {
	const [params, setParams] = useSearchParams();
	const historyLink = (job?: string) => {
		const next = new URLSearchParams(params);
		next.set("view", "workbench");
		next.set("ci", job ? "job" : "all");
		next.delete("cell");
		if (job) next.set("job", job);
		else next.delete("job");
		return `?${next}`;
	};
	const historyAction = (
		<Button
			onClick={() => setParams(new URLSearchParams(historyLink().slice(1)))}
			variant="secondary"
		>
			All CI history
		</Button>
	);
	const [status, setStatus] = useState("all"),
		[arch, setArch] = useState("all");
	if (error)
		return (
			<LoadError title="Could not load validation" onRetry={retry}>
				Required-job evidence is unavailable.
			</LoadError>
		);
	if (!data) return <Loading title="Loading validation…" />;
	if (!data.plan_known)
		return (
			<Alert isInline variant="info" title="CI configuration unavailable">
				The job inventory has not been loaded. Readiness cannot be determined
				yet. {historyAction}
			</Alert>
		);
	if (data.total === 0)
		return (
			<Alert isInline variant="info" title="No CI configured">
				No periodic jobs are configured for this version in the latest imported
				job inventory. CI readiness cannot be established. {historyAction}
			</Alert>
		);
	const rows = (data.rows || []).filter(
		(r) =>
			(status === "all" ||
				(status === "unmatched"
					? r.state === "missing"
					: (r.ci_result || r.state) === status)) &&
			(arch === "all" || r.architecture === arch),
	);
	return (
		<section className="rr-record">
			<div className="rr-card-heading">
				<h2>Required validation matrix</h2>
				{historyAction}
			</div>
			<ValidationProgress data={data} />
			<p className="rr-muted">
				Progress counts runs against the selected FBC. Results also show
				observed CI runs for other FBCs; those do not count toward shipment
				readiness.
			</p>
			<div className="rr-portfolio-tools">
				<FormSelect
					aria-label="CI result"
					value={status}
					onChange={(_, value) => setStatus(value)}
				>
					<FormSelectOption value="all" label="All statuses" />
					{["passed", "failed", "running", "missing", "unmatched"].map((s) => (
						<FormSelectOption
							key={s}
							value={s}
							label={
								s === "missing"
									? "No CI result"
									: s === "unmatched"
										? "No selected-FBC run"
										: s
							}
						/>
					))}
				</FormSelect>
				<FormSelect
					aria-label="Architecture"
					value={arch}
					onChange={(_, value) => setArch(value)}
				>
					<FormSelectOption value="all" label="All architectures" />
					{[
						...new Set(data.rows?.map((r) => r.architecture).filter(Boolean)),
					].map((a) => (
						<FormSelectOption value={a} key={a} label={a} />
					))}
				</FormSelect>
				<Button
					variant="link"
					onClick={() => {
						setStatus("all");
						setArch("all");
					}}
				>
					Reset filters
				</Button>
			</div>
			<div className="rr-validation-table">
				<table>
					<caption className="pf-v6-screen-reader">
						Required jobs and latest verified candidate evidence
					</caption>
					<thead>
						<tr>
							<th scope="col">Configuration</th>
							<th scope="col">Platform</th>
							<th scope="col">Result</th>
							<th scope="col">Evidence</th>
						</tr>
					</thead>
					<tbody>
						{rows.map((row) => (
							<tr key={row.job}>
								<td>
									<a href={row.source_url} target="_blank" rel="noreferrer">
										{row.display_name || row.mode || row.job} ↗
									</a>
									<p className="rr-muted">
										{[row.cloud, row.storage].filter(Boolean).join(" / ")}
									</p>
								</td>
								<td>
									{row.ocp
										? `OCP ${row.ocp}`
										: row.mode === "Standalone"
											? "Standalone"
											: "Not specified"}
									<p className="rr-muted">{row.architecture}</p>
								</td>
								<td>
									<ValidationResult row={row} />
								</td>
								<td>
									{row.run_url ? (
										<a href={row.run_url} target="_blank" rel="noreferrer">
											{row.state === "missing"
												? "Open observed run ↗"
												: "Open Prow run ↗"}
										</a>
									) : (
										"No verified candidate run"
									)}
									<p>
										<Link
											to={historyLink(row.job)}
											className="rr-history-link"
											aria-label={`View history for ${[row.display_name || row.job, row.cloud, row.storage, row.ocp && `OCP ${row.ocp}`, row.architecture].filter(Boolean).join(" · ")}`}
										>
											View history
										</Link>
									</p>
									<details>
										<summary>Details</summary>
										{row.evidence && <p className="rr-muted">{row.evidence}</p>}
										{row.tested_fbc && (
											<p className="rr-muted">
												Tested FBC: {row.tested_fbc.nvr}
												<br />
												<code>{row.tested_fbc.digest}</code>
											</p>
										)}
										{row.selected_fbc && (
											<p className="rr-muted">
												Selected FBC: {row.selected_fbc.nvr}
												<br />
												<code>{row.selected_fbc.digest}</code>
											</p>
										)}
										<code>{row.job}</code>
									</details>
								</td>
							</tr>
						))}
					</tbody>
				</table>
				{!rows.length && <p>No jobs match these filters.</p>}
			</div>
			<p className="rr-muted">
				Equivalent bundle runs and jobs without exact FBC identity do not
				satisfy this gate. Their available runs can still be inspected in CI
				history.
			</p>
		</section>
	);
}

function ValidationResult({ row }: { row: ValidationRow }) {
	const result = row.ci_result || row.state;
	const unmatched = row.state === "missing";
	if (result === "missing") {
		return (
			<span title="No matching run for selected FBC">
				<Label color="grey">No run</Label>
			</span>
		);
	}
	const qualifier = unmatched
		? row.fbc_match === "newer"
			? "newer FBC"
			: row.fbc_match === "different"
				? "different FBC"
				: "FBC unverified"
		: "";
	const label = (
		<Label
			color={
				result === "failed"
					? "red"
					: result === "passed" && !unmatched
						? "green"
						: "blue"
			}
		>
			{result.charAt(0).toUpperCase() + result.slice(1)}
			{qualifier && ` · ${qualifier}`}
		</Label>
	);
	return unmatched ? (
		<span title="No matching run for selected FBC">{label}</span>
	) : (
		label
	);
}

function FBCRecord({
	candidate,
	shippedAt,
}: {
	candidate?: ReleaseCandidate;
	shippedAt?: string;
}) {
	const fbc = candidate?.fbc_candidate;
	const members = (fbc?.operators || []).flatMap((op) =>
		op.members.map((m) => ({ ...m, operator: op.operator })),
	);
	const shipped = shippedAt
		? new Date(shippedAt).toLocaleDateString("en-US", {
				month: "short",
				day: "numeric",
				year: "numeric",
				timeZone: "UTC",
			})
		: null;
	return (
		<section className="rr-record">
			<h2>Shipped FBC record</h2>
			{!fbc ? (
				<p className="rr-muted">
					Historical FBC identities are not recorded for this release.
				</p>
			) : (
				<p>
					<strong>
						{members.length} shipped FBC{" "}
						{members.length === 1 ? "build" : "builds"}
					</strong>
					{shipped && ` · shipped ${shipped}`} ·{" "}
					<span className="rr-muted">
						{fbc.source === "stage_snapshots"
							? "recorded in the release's staged snapshots"
							: "stream fallback: not verified as the catalogs published to customers"}
					</span>{" "}
					<details style={{ display: "inline-block" }}>
						<summary>Build identities</summary>
						<ul style={{ marginTop: "0.5rem" }}>
							{members.map((m) => (
								<li key={`${m.operator}-${m.ocp}-${m.digest}`}>
									{m.build_url ? (
										<a href={m.build_url} target="_blank" rel="noreferrer">
											{m.nvr || "ART build"} ↗
										</a>
									) : (
										m.nvr || "Build identity unresolved"
									)}{" "}
									({m.operator}, OCP {m.ocp}) <code>{m.digest}</code>
								</li>
							))}
						</ul>
					</details>
				</p>
			)}
		</section>
	);
}
