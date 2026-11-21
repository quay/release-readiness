import {
	EmptyState,
	EmptyStateBody,
	Flex,
	FormSelect,
	FormSelectOption,
	Icon,
	Label,
	PageSection,
	Switch,
	TextInput,
	Title,
	ToggleGroup,
	ToggleGroupItem,
	Tooltip,
} from "@patternfly/react-core";
import {
	CheckCircleIcon,
	ColumnsIcon,
	ExclamationTriangleIcon,
	InfoCircleIcon,
	TimesCircleIcon,
} from "@patternfly/react-icons";
import { Table, Tbody, Td, Th, Thead, Tr } from "@patternfly/react-table";
import { Link, useSearchParams } from "react-router-dom";
import { listReleasesOverview } from "../api/client";
import type { ReleaseOverview } from "../api/types";
import LoadError, { Loading } from "../components/LoadError";
import { useNarrow } from "../components/OperatorImages";
import ValidationProgress from "../components/ValidationProgress";
import { useCachedFetch } from "../hooks/useCachedFetch";
import { useConfig } from "../hooks/useConfig";
import { day, days, due } from "../utils/format";
import { formatReleaseName, jiraIssueUrl } from "../utils/links";
import {
	portfolioLabel,
	releaseStage,
	stageReleases,
	streamIdentity,
} from "../utils/portfolio";
import { dueSource } from "../utils/releaseDetail";

const stages = [
	{
		key: "validation",
		title: "Validation",
		description:
			"Build preparation, testing, or release evidence still needs attention.",
	},
	{
		key: "shipment",
		title: "Shipment preparation",
		description:
			"Required validation passed. Prepare and approve the shipment.",
	},
	{
		key: "available",
		title: "Available to customers",
		description: "Publication confirmed by the Red Hat container catalog.",
	},
] as const;

const muted = { color: "var(--pf-t--global--text--color--subtle)" };
const dash = <span style={muted}>–</span>;
const nowrap = { whiteSpace: "nowrap" } as const;

export default function ReleasePortfolio() {
	const { data, error, refetch } = useCachedFetch(
		"portfolio",
		listReleasesOverview,
	);
	const config = useConfig();
	const [params, setParams] = useSearchParams();
	const layout = params.get("layout") === "stages" ? "stages" : "table";
	const product = params.get("product") || "all",
		query = params.get("q") || "";
	const history = params.get("history") === "1";
	const narrow = useNarrow();
	function change(key: string, value: string) {
		setParams((prev) => {
			const next = new URLSearchParams(prev);
			if (value) next.set(key, value);
			else next.delete(key);
			return next;
		});
	}
	const path = (ov: ReleaseOverview) =>
		`/releases/${encodeURIComponent(ov.release.name)}?${new URLSearchParams({ layout, ...(product !== "all" ? { product } : {}), ...(history ? { history: "1" } : {}), ...(query ? { q: query } : {}) })}`;
	const ticket = (ov: ReleaseOverview) =>
		ov.release.release_ticket_key ? (
			<a
				href={jiraIssueUrl(
					ov.release.release_ticket_key,
					config?.jira_base_url || "https://redhat.atlassian.net",
				)}
				target="_blank"
				rel="noreferrer"
			>
				{ov.release.release_ticket_key} ↗
			</a>
		) : (
			<span className="rr-muted">No release ticket</span>
		);
	function facts(ov: ReleaseOverview) {
		return (
			<>
				{!ov.published && (
					<ValidationProgress data={ov.portfolio?.validation} />
				)}
				{ov.published &&
					ov.portfolio?.validation.plan_known &&
					ov.portfolio.validation.total === 0 && (
						<p className="rr-muted">No CI configured</p>
					)}
				<div className="rr-card-facts">
					<span>{ticket(ov)}</span>
					<span>{ov.release.release_ticket_assignee || "Unassigned"}</span>
				</div>
				<ShipDate ov={ov} />
			</>
		);
	}
	if (!data || error)
		return (
			<PageSection>
				{error ? (
					<LoadError
						title="Could not load the release portfolio"
						onRetry={refetch}
					>
						Release data is unavailable. Retry the request.
					</LoadError>
				) : (
					<Loading title="Loading release portfolio…" />
				)}
			</PageSection>
		);
	const products = [
		...new Set(data.map((ov) => streamIdentity(ov.release.name).product)),
	].sort();
	const filtered = [...data]
		.sort((a, b) =>
			b.release.name.localeCompare(a.release.name, undefined, {
				numeric: true,
			}),
		)
		.filter(
			(ov) =>
				(product === "all" ||
					streamIdentity(ov.release.name).product === product) &&
				`${ov.release.name} ${ov.release.release_ticket_key || ""}`
					.toLowerCase()
					.includes(query.toLowerCase()),
		);
	// The stages view shows upcoming releases plus each stream's latest
	// published z-stream, so a stream like 3.9 whose every release shipped
	// still shows its newest one; history adds earlier z-streams. The table
	// shows only in-flight releases: released ones wait for history.
	const staged = stageReleases(filtered, history);
	const rows = history
		? staged
		: filtered.filter((ov) => !ov.shipped && !ov.release.released);
	return (
		<PageSection className="rr-page">
			<div className="rr-portfolio-heading">
				<div>
					<Title headingLevel="h1" size="3xl">
						Release portfolio
					</Title>
					<p className="rr-muted">
						{layout === "table"
							? "One row per release; open it for builds, validation and issues."
							: "Follow individual releases from validation to customer availability."}
					</p>
				</div>
				<ToggleGroup aria-label="Release portfolio view">
					<ToggleGroupItem
						text="Table"
						isSelected={layout === "table"}
						onChange={() => change("layout", "")}
					/>
					<ToggleGroupItem
						text="Stages"
						icon={<ColumnsIcon />}
						isSelected={layout === "stages"}
						onChange={() => change("layout", "stages")}
					/>
				</ToggleGroup>
			</div>
			<div className="rr-portfolio-tools">
				<FormSelect
					aria-label="Product"
					value={product}
					onChange={(_, v) => change("product", v === "all" ? "" : v)}
				>
					<FormSelectOption key="all" value="all" label="All products" />
					{products.map((p) => (
						<FormSelectOption
							key={p}
							value={p}
							label={p === "quay" ? "Quay" : p.toUpperCase()}
						/>
					))}
				</FormSelect>
				<TextInput
					aria-label="Find a release or ticket"
					placeholder="Find a release or ticket"
					value={query}
					onChange={(_, v) => change("q", v)}
				/>
				<Switch
					id="release-history"
					label="Show history"
					isChecked={history}
					onChange={(_, v) => change("history", v ? "1" : "")}
				/>
			</div>
			<p className="rr-muted rr-scope-note">
				{history
					? "Showing all recorded z-streams, including earlier releases."
					: layout === "table"
						? "Showing upcoming releases only. Enable history to include released ones."
						: "Showing upcoming releases and the latest published z-stream in each stream. Enable history for earlier releases."}
			</p>
			{!(layout === "table" ? rows.length : staged.length) && (
				<EmptyState
					titleText={data.length ? "No matching releases" : "No releases yet"}
					headingLevel="h2"
				>
					<EmptyStateBody>
						{data.length
							? "Try another product, version or ticket."
							: "Releases appear when JIRA sync discovers them. Source status is available in the header."}
					</EmptyStateBody>
				</EmptyState>
			)}
			{layout === "table" ? (
				<Table aria-label="Releases" variant="compact" gridBreakPoint="">
					<Thead>
						{narrow ? (
							<Tr>
								<Th>Release</Th>
							</Tr>
						) : (
							<Tr>
								<Th width={20}>Release</Th>
								<Th width={15}>Status</Th>
								<Th width={30}>Validation</Th>
								<Th width={15}>Due date</Th>
								<Th width={10}>Ticket</Th>
								<Th width={10}>Assignee</Th>
							</Tr>
						)}
					</Thead>
					<Tbody>
						{rows.map((ov) => (
							<ReleaseRow
								key={ov.release.name}
								ov={ov}
								narrow={narrow}
								href={path(ov)}
							/>
						))}
					</Tbody>
				</Table>
			) : (
				<div className="rr-stage-grid">
					{stages.map((s, i) => (
						<section
							key={s.key}
							className={`rr-stage-lane ${s.key}`}
							aria-labelledby={`stage-${s.key}`}
						>
							<header className="rr-stage-heading">
								<div className="rr-card-heading">
									<span>Stage {i + 1}</span>
									<span>
										{staged.filter((ov) => releaseStage(ov) === s.key).length}{" "}
										releases
									</span>
								</div>
								<h2 id={`stage-${s.key}`}>{s.title}</h2>
								<p>{s.description}</p>
							</header>
							<div className="rr-stage-items">
								{staged
									.filter((ov) => releaseStage(ov) === s.key)
									.map((ov) => (
										<article className="rr-release-card" key={ov.release.name}>
											<div className="rr-card-heading">
												<h3>
													<Link to={path(ov)}>
														{formatReleaseName(ov.release.name)}
													</Link>
												</h3>
												<StatusLabel ov={ov} />
											</div>
											{facts(ov)}
										</article>
									))}
								{!staged.some((ov) => releaseStage(ov) === s.key) && (
									<div className="rr-stage-empty">
										No releases in this stage.
									</div>
								)}
							</div>
						</section>
					))}
				</div>
			)}
			<p className="pf-v6-screen-reader" role="status">
				{layout === "table"
					? `${rows.length} releases`
					: `${staged.length} z-stream releases grouped by stage`}
			</p>
		</PageSection>
	);
}

/**
 * One release's facts in a row: the link, its status icon and label, the
 * required-validation progress, the target or shipped date with its flag,
 * the ticket and the assignee.
 */
function ReleaseRow({
	ov,
	narrow,
	href,
}: {
	ov: ReleaseOverview;
	narrow: boolean;
	href: string;
}) {
	const r = ov.release;
	const release = (
		<Flex
			columnGap={{ default: "columnGapSm" }}
			flexWrap={{ default: "nowrap" }}
			alignItems={{ default: "alignItemsCenter" }}
		>
			<span style={nowrap}>
				<Link to={href} style={{ fontWeight: 600 }}>
					{formatReleaseName(r.name)}
				</Link>
			</span>
		</Flex>
	);
	const ticketLink = r.release_ticket_key && (
		<a
			href={jiraIssueUrl(r.release_ticket_key, "https://redhat.atlassian.net")}
			target="_blank"
			rel="noreferrer"
		>
			{r.release_ticket_key} ↗
		</a>
	);
	const assignee = r.release_ticket_assignee || (
		<span style={muted}>Unassigned</span>
	);
	if (narrow)
		return (
			<Tr>
				<Td dataLabel="Release">
					<Flex
						justifyContent={{ default: "justifyContentSpaceBetween" }}
						alignItems={{ default: "alignItemsCenter" }}
						flexWrap={{ default: "wrap" }}
						columnGap={{ default: "columnGapMd" }}
					>
						{release}
						<ShipDate ov={ov} />
					</Flex>
					<Flex
						columnGap={{ default: "columnGapMd" }}
						alignItems={{ default: "alignItemsCenter" }}
						flexWrap={{ default: "wrap" }}
						style={{ marginTop: "0.25rem" }}
					>
						<StatusLabel ov={ov} />
						<span style={nowrap}>
							{ticketLink || <span style={muted}>No ticket</span>}
						</span>
						<span style={nowrap}>{assignee}</span>
					</Flex>
					{!ov.published && (
						<div style={{ marginTop: "0.25rem" }}>
							<ValidationProgress data={ov.portfolio?.validation} />
						</div>
					)}
				</Td>
			</Tr>
		);
	return (
		<Tr>
			<Td dataLabel="Release">{release}</Td>
			<Td dataLabel="Status">
				<StatusLabel ov={ov} />
			</Td>
			<Td dataLabel="Validation">
				{ov.published ? (
					dash
				) : (
					<ValidationProgress data={ov.portfolio?.validation} />
				)}
			</Td>
			<Td dataLabel="Due date">
				<ShipDate ov={ov} />
			</Td>
			<Td dataLabel="Ticket">{ticketLink || dash}</Td>
			<Td dataLabel="Assignee">{assignee}</Td>
		</Tr>
	);
}

/** The release's state as an icon and a status-coloured label. */
function StatusLabel({ ov }: { ov: ReleaseOverview }) {
	const label = portfolioLabel(ov);
	const failed = (ov.portfolio?.validation.failed ?? 0) > 0;
	const icon = ov.published ? (
		<Icon status="success">
			<CheckCircleIcon />
		</Icon>
	) : failed ? (
		<Icon status="danger">
			<TimesCircleIcon />
		</Icon>
	) : ov.portfolio?.validation.ready ? (
		<Icon status="success">
			<CheckCircleIcon />
		</Icon>
	) : ov.portfolio?.verdict ? (
		<Icon status="info">
			<InfoCircleIcon />
		</Icon>
	) : null;
	const color = ov.published
		? "green"
		: ov.portfolio?.validation.ready
			? "green"
			: failed
				? "red"
				: ov.portfolio?.verdict
					? "blue"
					: "grey";
	return (
		<span
			style={{
				display: "inline-flex",
				gap: "0.25rem",
				alignItems: "center",
			}}
		>
			{icon}
			<Label color={color}>{label}</Label>
		</span>
	);
}

const exact = (iso: string) =>
	`${new Date(iso).toLocaleString("en-US", {
		timeZone: "UTC",
		month: "short",
		day: "numeric",
		year: "numeric",
		hour: "2-digit",
		minute: "2-digit",
		hourCycle: "h23",
	})} UTC`;

/**
 * Unshipped, the target date with its due flag, its Jira source in its
 * tooltip; shipped, a check and the day the catalog first published it, the
 * exact time in its tooltip.
 */
function ShipDate({ ov }: { ov: ReleaseOverview }) {
	const r = ov.release;
	const shipped = ov.shipped || r.released;
	if (shipped)
		return (
			<Tooltip
				content={
					ov.shipped_at
						? `Published to the Red Hat catalog ${exact(ov.shipped_at)}`
						: "Shipped"
				}
			>
				<span
					// biome-ignore lint/a11y/noNoninteractiveTabindex: focus shows its tooltip.
					tabIndex={0}
					className="rr-ship-date"
					style={{
						...nowrap,
						display: "inline-flex",
						gap: "0.375rem",
						alignItems: "center",
					}}
				>
					<Icon status="success" isInline>
						<CheckCircleIcon />
					</Icon>
					{ov.shipped_at ? `Shipped ${day(ov.shipped_at)}` : "Shipped"}
				</span>
			</Tooltip>
		);
	const target = r.due_date ?? r.release_date;
	const d = target ? due(target, false) : undefined;
	const flag =
		d?.kind === "past"
			? "Past due"
			: d?.kind === "soon"
				? d.days === 0
					? "Due today"
					: `Due in ${days(d.days)}`
				: null;
	return (
		<span style={nowrap}>
			{d ? (
				<Tooltip content={dueSource(r)}>
					{/* biome-ignore lint/a11y/noNoninteractiveTabindex: focus shows its tooltip. */}
					<span tabIndex={0} className="rr-ship-date">
						{d.date}
					</span>
				</Tooltip>
			) : (
				<span style={muted}>TBD</span>
			)}
			{flag && (
				<span
					style={{
						marginLeft: "0.5rem",
						display: "inline-flex",
						gap: "0.25rem",
						alignItems: "center",
					}}
				>
					<Icon status="warning" isInline>
						<ExclamationTriangleIcon />
					</Icon>
					{flag}
				</span>
			)}
		</span>
	);
}
