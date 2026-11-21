import {
	Alert,
	Button,
	DrawerActions,
	DrawerCloseButton,
	DrawerHead,
	DrawerPanelBody,
	DrawerPanelContent,
	Label,
	Pagination,
	Spinner,
	Switch,
	Title,
} from "@patternfly/react-core";
import { Table, Tbody, Td, Th, Thead, Tr } from "@patternfly/react-table";
import { useEffect, useState } from "react";
import { getCIHistory } from "../api/client";
import type { CIHistoryRun } from "../api/types";
import { useCachedFetch } from "../hooks/useCachedFetch";
import { jobShortName, runResult } from "../utils/releaseDetail";
import { Status } from "./Status";

export default function JobHistoryDrawer({
	version,
	job,
	hasSelectedFBCs,
	onClose,
}: {
	version: string;
	job: string;
	/** Whether the release has selected FBCs to scope runs to. */
	hasSelectedFBCs: boolean;
	onClose: () => void;
}) {
	// A release with no selected FBCs starts on all runs: there is nothing
	// to scope to, and flipping after the first fetch flickers.
	const [includeOther, setIncludeOther] = useState(!hasSelectedFBCs);
	const [cursors, setCursors] = useState<string[]>([""]);
	const [page, setPage] = useState(1);
	const cursor = cursors[page - 1] || "";
	const history = useCachedFetch(
		`job-history:${version}:${job}:${includeOther}:${cursor}`,
		() => getCIHistory(version, job, includeOther, cursor),
	);

	useEffect(() => {
		const close = (event: KeyboardEvent) => {
			// Escape aimed at an open popover or select must not close the
			// drawer behind it.
			if (event.key === "Escape" && !event.defaultPrevented) {
				event.preventDefault();
				onClose();
			}
		};
		document.addEventListener("keydown", close);
		return () => document.removeEventListener("keydown", close);
	}, [onClose]);
	const groups = new Map<string, CIHistoryRun[]>();
	for (const run of history.data?.runs || []) {
		const key = `${run.fbc_match}:${run.fbc?.digest || "unknown"}`;
		groups.set(key, [...(groups.get(key) || []), run]);
	}
	const sorted = [...groups.entries()].sort(
		(a, b) =>
			Number(b[1][0].fbc_match === "exact") -
			Number(a[1][0].fbc_match === "exact"),
	);
	const jobInfo = history.data?.job;
	return (
		<DrawerPanelContent
			className="rr-job-history"
			widths={{ default: "width_100", lg: "width_33" }}
			focusTrap={{ enabled: true }}
		>
			<DrawerHead>
				<Title headingLevel="h2" size="xl">
					{job ? "Job history" : "All CI history"}
				</Title>
				<p className="rr-muted">
					{jobInfo
						? [
								jobInfo.display_name,
								[jobInfo.cloud, jobInfo.storage].filter(Boolean).join(" / "),
								jobInfo.ocp && `OCP ${jobInfo.ocp}`,
								jobInfo.architecture,
							]
								.filter(Boolean)
								.join(" · ")
						: job
							? jobShortName(job)
							: "Stored runs for this release’s selected FBCs"}
				</p>
				<DrawerActions>
					<DrawerCloseButton onClick={onClose} />
				</DrawerActions>
			</DrawerHead>
			<DrawerPanelBody>
				{job && (
					<details className="rr-history-job">
						<summary>Full job name</summary>
						<code>{job}</code>
					</details>
				)}
				<Switch
					id="history-other-fbcs"
					label="Include other and unverified FBCs"
					isChecked={includeOther}
					onChange={(_, checked) => {
						setIncludeOther(checked);
						setPage(1);
						setCursors([""]);
					}}
				/>
				<p className="rr-muted rr-history-note">
					{includeOther
						? "Includes other FBCs and z-streams from this product stream. These runs do not change release readiness."
						: "Only runs with verified identity matching a selected FBC. Run totals are historical observations, not readiness counts."}
				</p>
				{history.error && (
					<Alert
						isInline
						variant="warning"
						title="Could not refresh CI history"
					>
						<Button variant="link" onClick={history.refetch}>
							Retry history
						</Button>
					</Alert>
				)}
				{!history.data && !history.error && (
					<Spinner aria-label="Loading CI history" />
				)}
				{history.data && (
					<>
						{!includeOther && !history.data.selected_fbcs.length && (
							<Alert isInline variant="info" title="No selected FBC available">
								Include other and unverified FBCs to inspect stored runs.
							</Alert>
						)}
						{!history.data.runs.length && (
							<p role="status">
								{includeOther
									? "No stored runs found for this scope."
									: "No stored runs match the selected FBC. Include other and unverified FBCs to investigate."}
							</p>
						)}
						{sorted.map(([key, runs]) => {
							const first = runs[0];
							return (
								<section
									className="rr-history-group"
									key={key}
									aria-label={
										first.fbc?.nvr || first.fbc?.digest || "FBC unverified"
									}
								>
									<header>
										<Label
											color={first.fbc_match === "exact" ? "blue" : "grey"}
										>
											{first.fbc_match === "exact"
												? "Selected FBC"
												: first.fbc_match === "different"
													? "Other FBC"
													: "FBC unverified"}
										</Label>
										{first.fbc?.nvr && (
											<p className="rr-history-nvr">{first.fbc.nvr}</p>
										)}
										{first.fbc && (
											<details>
												<summary>FBC digest</summary>
												<code>{first.fbc.digest}</code>
											</details>
										)}
										<p className="rr-muted">
											{runs.length} {runs.length === 1 ? "run" : "runs"} on this
											page ·{" "}
											{
												runs.filter((r) => runResult(r.state).status === "done")
													.length
											}{" "}
											passed ·{" "}
											{
												runs.filter(
													(r) => runResult(r.state).status === "failed",
												).length
											}{" "}
											failed
										</p>
									</header>
									<Table
										variant="compact"
										aria-label="Historical CI runs"
										gridBreakPoint="grid-md"
									>
										<Thead>
											<Tr>
												{!job && <Th>Configuration</Th>}
												<Th>Result</Th>
												<Th>Started (UTC)</Th>
											</Tr>
										</Thead>
										<Tbody>
											{runs.map((run) => {
												const result = runResult(run.state);
												const started = run.started_at
													? new Date(run.started_at).toLocaleString("en-GB", {
															timeZone: "UTC",
															day: "numeric",
															month: "short",
															year: "numeric",
															hour: "2-digit",
															minute: "2-digit",
														})
													: "Start time unavailable";
												return (
													<Tr key={`${run.job}:${run.build_id}`}>
														{!job && (
															<Td dataLabel="Configuration">
																<span>
																	{run.display_name || jobShortName(run.job)}
																</span>
																<details>
																	<summary>Job name</summary>
																	<code>{run.job}</code>
																</details>
															</Td>
														)}
														<Td dataLabel="Result">
															{result.status ? (
																<Status status={result.status}>
																	{result.text}
																</Status>
															) : (
																result.text
															)}
														</Td>
														<Td dataLabel="Started (UTC)">
															{run.prow_url ? (
																<a
																	href={run.prow_url}
																	target="_blank"
																	rel="noreferrer"
																	aria-label={`Open Prow run ${run.build_id}: ${started}`}
																>
																	{started} ↗
																</a>
															) : (
																started
															)}
														</Td>
													</Tr>
												);
											})}
										</Tbody>
									</Table>
								</section>
							);
						})}
						<Pagination
							variant="bottom"
							isCompact
							isStatic
							page={page}
							perPage={50}
							itemCount={
								history.data.next_cursor
									? undefined
									: (page - 1) * 50 + history.data.runs.length
							}
							toggleTemplate={`${history.data.runs.length} runs on page ${page}`}
							titles={{
								paginationAriaLabel: "CI history pages",
								toNextPageAriaLabel: "Next history page",
								toPreviousPageAriaLabel: "Previous history page",
							}}
							onSetPage={(_, next) => {
								if (next === page + 1 && history.data?.next_cursor) {
									setCursors([
										...cursors.slice(0, page),
										history.data.next_cursor,
									]);
									setPage(next);
								} else if (next >= 1 && next < page) setPage(next);
							}}
						/>
					</>
				)}
			</DrawerPanelBody>
		</DrawerPanelContent>
	);
}
