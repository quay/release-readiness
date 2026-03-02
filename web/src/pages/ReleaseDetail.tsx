import {
	Breadcrumb,
	BreadcrumbItem,
	Card,
	CardBody,
	CardTitle,
	EmptyState,
	EmptyStateBody,
	ExpandableSection,
	Flex,
	FlexItem,
	Label,
	PageSection,
	ProgressStep,
	ProgressStepper,
	Spinner,
	Title,
} from "@patternfly/react-core";
import {
	CheckCircleIcon,
	ExclamationCircleIcon,
} from "@patternfly/react-icons";
import {
	Table,
	Tbody,
	Td,
	Th,
	Thead,
	type ThProps,
	Tr,
} from "@patternfly/react-table";
import { useMemo, useState } from "react";
import { Link, useParams } from "react-router-dom";
import {
	getRelease,
	getReleaseIssueSummary,
	getReleaseReadiness,
	getReleaseSnapshot,
	listReleaseIssues,
} from "../api/client";
import type {
	IssueSummary,
	JiraIssue,
	ReadinessResponse,
	ReleaseVersion,
	SnapshotRecord,
} from "../api/types";
import ExpandableCard from "../components/ExpandableCard";
import GitShaLink from "../components/GitShaLink";
import PriorityLabel from "../components/PriorityLabel";
import StatusLabel from "../components/StatusLabel";
import { useCachedFetch } from "../hooks/useCachedFetch";
import { useConfig } from "../hooks/useConfig";
import { formatReleaseName, jiraIssueUrl, quayImageUrl } from "../utils/links";

export default function ReleaseDetail() {
	const { version } = useParams<{ version: string }>();
	const config = useConfig();

	const { data: release, loading: loadingRelease } = useCachedFetch(
		version ? `release:${version}` : null,
		() => getRelease(version!),
	);
	const { data: snapshot } = useCachedFetch(
		version ? `snapshot:${version}` : null,
		() => getReleaseSnapshot(version!),
	);
	const { data: issues } = useCachedFetch(
		version ? `issues:${version}` : null,
		() => listReleaseIssues(version!),
	);
	const { data: issueSummary } = useCachedFetch(
		version ? `issueSummary:${version}` : null,
		() => getReleaseIssueSummary(version!),
	);
	const { data: readinessSignal } = useCachedFetch(
		version ? `readiness:${version}` : null,
		() => getReleaseReadiness(version!),
	);

	const [componentsExpanded, setComponentsExpanded] = useState(false);
	const [testResultsExpanded, setTestResultsExpanded] = useState(false);

	if (loadingRelease && !release) {
		return (
			<PageSection>
				<div style={{ textAlign: "center" }}>
					<Spinner />
				</div>
			</PageSection>
		);
	}

	if (!release) {
		return (
			<PageSection>
				<EmptyState>
					<Title headingLevel="h2" size="lg">
						Release not found
					</Title>
					<EmptyStateBody>
						No data found for release &quot;{version}&quot;.
					</EmptyStateBody>
				</EmptyState>
			</PageSection>
		);
	}

	const displayName = formatReleaseName(release.name);

	return (
		<>
			<PageSection>
				<Breadcrumb>
					<BreadcrumbItem>
						<Link to="/">Releases</Link>
					</BreadcrumbItem>
					<BreadcrumbItem isActive>{displayName}</BreadcrumbItem>
				</Breadcrumb>
			</PageSection>

			<PageSection>
				<Flex
					justifyContent={{ default: "justifyContentSpaceBetween" }}
					alignItems={{ default: "alignItemsCenter" }}
					style={{ marginBottom: "1rem" }}
				>
					<FlexItem>
						<Title headingLevel="h1">{displayName}</Title>
					</FlexItem>
					{release.s3_application && (
						<FlexItem>
							<Link to={`/releases/${encodeURIComponent(version!)}/snapshots`}>
								View all snapshots
							</Link>
						</FlexItem>
					)}
				</Flex>

				<ReleaseSignal
					release={release}
					readiness={readinessSignal ?? null}
					jiraBaseUrl={config?.jira_base_url}
					snapshot={snapshot ?? null}
					issueSummary={issueSummary ?? null}
				/>

				{snapshot && (
					<Card isCompact style={{ marginBottom: "1rem" }}>
						<CardTitle>Latest Snapshot</CardTitle>
						<CardBody>
							<Flex
								justifyContent={{ default: "justifyContentSpaceEvenly" }}
								flexWrap={{ default: "nowrap" }}
							>
								<FlexItem style={{ textAlign: "center" }}>
									<div className="rr-label">Snapshot</div>
									<div>{snapshot.name}</div>
								</FlexItem>
								<FlexItem style={{ textAlign: "center" }}>
									<div className="rr-label">Tests</div>
									<div>
										{!snapshot.has_tests ? (
											<Label color="grey">N/A</Label>
										) : snapshot.tests_passed ? (
											<Label color="green" icon={<CheckCircleIcon />}>
												Passed
											</Label>
										) : (
											<Label color="red" icon={<ExclamationCircleIcon />}>
												Failed
											</Label>
										)}
									</div>
								</FlexItem>
								<FlexItem style={{ textAlign: "center" }}>
									<div className="rr-label">Created</div>
									<div>{new Date(snapshot.created_at).toLocaleString()}</div>
								</FlexItem>
							</Flex>

							{/* Components Table */}
							{snapshot.components && snapshot.components.length > 0 && (
								<ExpandableSection
									toggleText={`Components (${snapshot.components.length})`}
									isExpanded={componentsExpanded}
									onToggle={(_e, val) => setComponentsExpanded(val)}
									style={{ marginTop: "1rem" }}
								>
									<Table variant="compact">
										<Thead>
											<Tr>
												<Th>Component</Th>
												<Th>Git SHA</Th>
												<Th>Image</Th>
											</Tr>
										</Thead>
										<Tbody>
											{snapshot.components.map((c) => {
												const imgUrl = quayImageUrl(c.image_url);
												const imgDisplay = c.image_url.includes("/")
													? (c.image_url.split("/").pop()?.split("@")[0] ??
														c.image_url)
													: c.image_url;
												return (
													<Tr key={c.id}>
														<Td>{c.component}</Td>
														<Td>
															<GitShaLink
																component={c.component}
																sha={c.git_sha}
																gitUrl={c.git_url}
															/>
														</Td>
														<Td>
															{imgUrl ? (
																<a
																	href={imgUrl}
																	target="_blank"
																	rel="noopener noreferrer"
																>
																	<code style={{ fontSize: "0.85em" }}>
																		{imgDisplay}
																	</code>
																</a>
															) : (
																<code style={{ fontSize: "0.85em" }}>
																	{c.image_url}
																</code>
															)}
														</Td>
													</Tr>
												);
											})}
										</Tbody>
									</Table>
								</ExpandableSection>
							)}

							{/* Test Suites Table */}
							{snapshot.test_suites && snapshot.test_suites.length > 0 && (
								<ExpandableSection
									toggleText={`Test Suites (${snapshot.test_suites.length})`}
									isExpanded={testResultsExpanded}
									onToggle={(_e, val) => setTestResultsExpanded(val)}
									style={{ marginTop: "1rem" }}
								>
									<Table variant="compact">
										<Thead>
											<Tr>
												<Th>Suite</Th>
												<Th>Status</Th>
												<Th>Tool</Th>
												<Th modifier="fitContent">Passed</Th>
												<Th modifier="fitContent">Failed</Th>
												<Th modifier="fitContent">Skipped</Th>
												<Th modifier="fitContent">Total</Th>
											</Tr>
										</Thead>
										<Tbody>
											{snapshot.test_suites.map((ts) => (
												<Tr key={ts.id}>
													<Td>{ts.name}</Td>
													<Td>
														<StatusLabel status={ts.status} />
													</Td>
													<Td>
														{ts.tool_name}
														{ts.tool_version ? ` ${ts.tool_version}` : ""}
													</Td>
													<Td>{ts.tests === 0 ? "\u2014" : ts.passed}</Td>
													<Td>{ts.tests === 0 ? "\u2014" : ts.failed}</Td>
													<Td>{ts.tests === 0 ? "\u2014" : ts.skipped}</Td>
													<Td>{ts.tests === 0 ? "\u2014" : ts.tests}</Td>
												</Tr>
											))}
										</Tbody>
									</Table>
								</ExpandableSection>
							)}
						</CardBody>
					</Card>
				)}

				{/* Bug Verification Table */}
				{(issues ?? []).length > 0 && (
					<ExpandableCard title={`Bug Verification (${(issues ?? []).length})`}>
						<IssuesTable issues={issues ?? []} />
					</ExpandableCard>
				)}
			</PageSection>
		</>
	);
}

function ReleaseSignal({
	release,
	readiness,
	jiraBaseUrl,
	snapshot,
	issueSummary,
}: {
	release: ReleaseVersion;
	readiness: ReadinessResponse | null;
	jiraBaseUrl?: string;
	snapshot: SnapshotRecord | null;
	issueSummary: IssueSummary | null;
}) {
	const dueDate = release.due_date ? new Date(release.due_date) : null;
	const releaseDate = release.release_date
		? new Date(release.release_date)
		: null;
	const targetDate = dueDate ?? releaseDate;

	const daysUntil = targetDate
		? Math.ceil((targetDate.getTime() - Date.now()) / (1000 * 60 * 60 * 24))
		: null;

	const signalColor =
		readiness?.signal === "green"
			? "green"
			: readiness?.signal === "red"
				? "red"
				: readiness?.signal === "yellow"
					? "yellow"
					: "grey";

	const ticketLink = release.release_ticket_key
		? jiraIssueUrl(
				release.release_ticket_key,
				jiraBaseUrl || "https://issues.redhat.com",
			)
		: null;

	const buildsReady =
		snapshot !== null &&
		snapshot.components !== undefined &&
		snapshot.components.length > 0;
	const hasTests = snapshot?.has_tests ?? false;
	const allTestsPassed = hasTests && (snapshot?.tests_passed ?? false);
	const bugsVerified =
		issueSummary !== null && issueSummary.total > 0 && issueSummary.open === 0;
	const qeSignOff = allTestsPassed && (bugsVerified || issueSummary === null);

	const progressItems = [
		{ label: "Builds ready", done: buildsReady, warning: snapshot === null },
		{ label: "Tests passed", done: allTestsPassed, warning: !hasTests },
		...(issueSummary ? [{ label: "Bugs verified", done: bugsVerified }] : []),
		{ label: "QE sign off", done: qeSignOff },
	];

	const firstIncomplete = progressItems.findIndex((i) => !i.done);

	return (
		<Card isCompact style={{ marginBottom: "1rem" }}>
			<CardTitle>Release Status</CardTitle>
			<CardBody>
				<Flex justifyContent={{ default: "justifyContentSpaceEvenly" }}>
					{readiness && (
						<FlexItem style={{ textAlign: "center" }}>
							<div className="rr-label">Signal</div>
							<Label color={signalColor} isCompact>
								{readiness.message}
							</Label>
						</FlexItem>
					)}
					<FlexItem style={{ textAlign: "center" }}>
						<div className="rr-label">Target</div>
						<div>
							{targetDate ? targetDate.toLocaleDateString() : "TBD"}
							{daysUntil !== null && ` (${daysUntil} days)`}
						</div>
					</FlexItem>
					{release.release_ticket_key && (
						<FlexItem style={{ textAlign: "center" }}>
							<div className="rr-label">Ticket</div>
							<div>
								{ticketLink ? (
									<a
										href={ticketLink}
										target="_blank"
										rel="noopener noreferrer"
									>
										{release.release_ticket_key}
									</a>
								) : (
									release.release_ticket_key
								)}
							</div>
						</FlexItem>
					)}
					{release.release_ticket_assignee && (
						<FlexItem style={{ textAlign: "center" }}>
							<div className="rr-label">Assignee</div>
							<div>{release.release_ticket_assignee}</div>
						</FlexItem>
					)}
					{release.released && (
						<FlexItem style={{ textAlign: "center" }}>
							<div className="rr-label">Status</div>
							<Label color="green">Released</Label>
						</FlexItem>
					)}
				</Flex>
				<ProgressStepper isCenterAligned style={{ marginTop: "1.5rem" }}>
					{progressItems.map((item, idx) => (
						<ProgressStep
							key={item.label}
							variant={
								item.done
									? "success"
									: item.warning
										? "warning"
										: "pending"
							}
							isCurrent={idx === firstIncomplete}
							id={`step-${idx}`}
							titleId={`step-${idx}-title`}
							aria-label={item.label}
						>
							{item.label}
						</ProgressStep>
					))}
				</ProgressStepper>
			</CardBody>
		</Card>
	);
}

const priorityWeight: Record<string, number> = {
	blocker: 0,
	critical: 1,
	major: 2,
	normal: 3,
	minor: 4,
	undefined: 5,
};

function IssuesTable({ issues }: { issues: JiraIssue[] }) {
	const [activeSortIndex, setActiveSortIndex] = useState<number | undefined>(
		undefined,
	);
	const [activeSortDirection, setActiveSortDirection] = useState<
		"asc" | "desc" | undefined
	>(undefined);

	const sortedIssues = useMemo(() => {
		if (activeSortIndex === undefined || activeSortDirection === undefined) {
			return issues;
		}
		return [...issues].sort((a, b) => {
			let cmp = 0;
			switch (activeSortIndex) {
				case 1: // Type
					cmp = a.issue_type.localeCompare(b.issue_type);
					break;
				case 3: // Priority
					cmp =
						(priorityWeight[a.priority.toLowerCase()] ?? 5) -
						(priorityWeight[b.priority.toLowerCase()] ?? 5);
					break;
				case 4: // Status
					cmp = a.status.localeCompare(b.status);
					break;
				case 5: // Assignee
					cmp = a.assignee.localeCompare(b.assignee);
					break;
			}
			return activeSortDirection === "asc" ? cmp : -cmp;
		});
	}, [issues, activeSortIndex, activeSortDirection]);

	const getSortParams = (columnIndex: number): ThProps["sort"] => ({
		sortBy: {
			index: activeSortIndex,
			direction: activeSortDirection,
		},
		onSort: (_event, index, direction) => {
			setActiveSortIndex(index);
			setActiveSortDirection(direction);
		},
		columnIndex,
	});

	return (
		<Table variant="compact" style={{ tableLayout: "auto" }}>
			<Thead>
				<Tr>
					<Th style={{ whiteSpace: "nowrap" }}>Key</Th>
					<Th sort={getSortParams(1)} style={{ whiteSpace: "nowrap" }}>
						Type
					</Th>
					<Th>Summary</Th>
					<Th
						sort={getSortParams(3)}
						style={{ whiteSpace: "nowrap", minWidth: "120px" }}
					>
						Priority
					</Th>
					<Th
						sort={getSortParams(4)}
						style={{ whiteSpace: "nowrap", minWidth: "110px" }}
					>
						Status
					</Th>
					<Th
						sort={getSortParams(5)}
						style={{ whiteSpace: "nowrap", minWidth: "140px" }}
					>
						Assignee
					</Th>
				</Tr>
			</Thead>
			<Tbody>
				{sortedIssues.map((issue) => (
					<Tr key={issue.key}>
						<Td>
							<a href={issue.link} target="_blank" rel="noopener noreferrer">
								{issue.key}
							</a>
						</Td>
						<Td>{issue.issue_type}</Td>
						<Td style={{ whiteSpace: "normal", wordBreak: "break-word" }}>
							{issue.summary}
						</Td>
						<Td>
							<PriorityLabel priority={issue.priority} />
						</Td>
						<Td>
							<StatusLabel status={issue.status} />
						</Td>
						<Td>{issue.assignee}</Td>
					</Tr>
				))}
			</Tbody>
		</Table>
	);
}
