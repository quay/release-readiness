import {
	Breadcrumb,
	BreadcrumbItem,
	Button,
	Card,
	CardBody,
	CardTitle,
	EmptyState,
	EmptyStateBody,
	Flex,
	FlexItem,
	Label,
	PageSection,
	ProgressStep,
	ProgressStepper,
	Spinner,
	Tab,
	Tabs,
	TabTitleText,
	Title,
	Tooltip,
} from "@patternfly/react-core";
import {
	CheckCircleIcon,
	DownloadIcon,
	ExclamationCircleIcon,
} from "@patternfly/react-icons";
import {
	ExpandableRowContent,
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
	downloadSuiteArtifacts,
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
	VulnerabilityReport,
} from "../api/types";
import GitShaLink from "../components/GitShaLink";
import PriorityLabel from "../components/PriorityLabel";
import StatusLabel from "../components/StatusLabel";
import TestCasesTable from "../components/TestCasesTable";
import VulnerabilitiesTable from "../components/VulnerabilitiesTable";
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

	const [activeSnapshotTab, setActiveSnapshotTab] = useState<string | number>(
		"components",
	);
	const [expandedSuites, setExpandedSuites] = useState<Set<number>>(new Set());
	const [expandedComponents, setExpandedComponents] = useState<Set<string>>(
		new Set(),
	);
	const [activeArchTab, setActiveArchTab] = useState<Record<string, string>>(
		{},
	);

	const groupedVulnReports = useMemo(() => {
		const reports = snapshot?.vulnerability_reports;
		if (!reports || reports.length === 0) return [];

		const map = new Map<string, VulnerabilityReport[]>();
		for (const rpt of reports) {
			const existing = map.get(rpt.component);
			if (existing) {
				existing.push(rpt);
			} else {
				map.set(rpt.component, [rpt]);
			}
		}

		return [...map.entries()]
			.map(([component, compReports]) => ({
				component,
				reports: compReports.sort((a, b) => a.arch.localeCompare(b.arch)),
				total: compReports.reduce((s, r) => s + r.total, 0),
				critical: compReports.reduce((s, r) => s + r.critical, 0),
				high: compReports.reduce((s, r) => s + r.high, 0),
				medium: compReports.reduce((s, r) => s + r.medium, 0),
				low: compReports.reduce((s, r) => s + r.low, 0),
				fixable: compReports.reduce((s, r) => s + r.fixable, 0),
			}))
			.sort((a, b) => a.component.localeCompare(b.component));
	}, [snapshot?.vulnerability_reports]);

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

							<Tabs
								activeKey={activeSnapshotTab}
								onSelect={(_e, key) => setActiveSnapshotTab(key)}
								isFilled
								style={{ marginTop: "1rem" }}
							>
								{snapshot.components && snapshot.components.length > 0 && (
									<Tab
										eventKey="components"
										title={
											<TabTitleText>
												Components ({snapshot.components.length})
											</TabTitleText>
										}
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
									</Tab>
								)}

								{snapshot.test_suites && snapshot.test_suites.length > 0 && (
									<Tab
										eventKey="testSuites"
										title={
											<TabTitleText>
												Test Suites ({snapshot.test_suites.length})
											</TabTitleText>
										}
									>
										<Table variant="compact">
											<Thead>
												<Tr>
													<Th screenReaderText="Toggle" />
													<Th>Suite</Th>
													<Th>Status</Th>
													<Th>Tool</Th>
													<Th modifier="fitContent">Passed</Th>
													<Th modifier="fitContent">Failed</Th>
													<Th modifier="fitContent">Skipped</Th>
													<Th modifier="fitContent">Total</Th>
													<Th screenReaderText="Actions" />
												</Tr>
											</Thead>
											{snapshot.test_suites.map((ts) => {
												const isSuiteExpanded = expandedSuites.has(ts.id);
												return (
													<Tbody key={ts.id} isExpanded={isSuiteExpanded}>
														<Tr>
															<Td
																expand={{
																	rowIndex: ts.id,
																	isExpanded: isSuiteExpanded,
																	onToggle: () =>
																		setExpandedSuites((prev) => {
																			const next = new Set(prev);
																			if (next.has(ts.id)) {
																				next.delete(ts.id);
																			} else {
																				next.add(ts.id);
																			}
																			return next;
																		}),
																}}
															/>
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
															<Td modifier="fitContent">
																<Tooltip content="Download artifacts">
																	<Button
																		variant="plain"
																		aria-label="Download artifacts"
																		style={{ padding: 0 }}
																		onClick={() =>
																			downloadSuiteArtifacts(
																				snapshot.id,
																				ts.id,
																			)
																		}
																	>
																		<DownloadIcon />
																	</Button>
																</Tooltip>
															</Td>
														</Tr>
														{isSuiteExpanded && (
															<Tr isExpanded>
																<Td colSpan={9}>
																	<ExpandableRowContent>
																		{ts.test_cases &&
																		ts.test_cases.length > 0 ? (
																			<TestCasesTable
																				testCases={ts.test_cases}
																			/>
																		) : (
																			<em>No test cases recorded.</em>
																		)}
																	</ExpandableRowContent>
																</Td>
															</Tr>
														)}
													</Tbody>
												);
											})}
										</Table>
									</Tab>
								)}
								{groupedVulnReports.length > 0 && (
									<Tab
										eventKey="securityScans"
										title={
											<TabTitleText>
												Security Scans ({groupedVulnReports.length})
											</TabTitleText>
										}
									>
										<Table variant="compact">
											<Thead>
												<Tr>
													<Th screenReaderText="Toggle" />
													<Th>Component</Th>
													<Th modifier="fitContent">Architectures</Th>
													<Th modifier="fitContent">Critical</Th>
													<Th modifier="fitContent">High</Th>
													<Th modifier="fitContent">Medium</Th>
													<Th modifier="fitContent">Low</Th>
													<Th modifier="fitContent">Total</Th>
													<Th modifier="fitContent">Fixable</Th>
												</Tr>
											</Thead>
											{groupedVulnReports.map((group, groupIdx) => {
												const isExpanded = expandedComponents.has(
													group.component,
												);
												const selectedArch =
													activeArchTab[group.component] ??
													group.reports[0]?.arch;
												const selectedReport = group.reports.find(
													(r) => r.arch === selectedArch,
												);
												return (
													<Tbody key={group.component} isExpanded={isExpanded}>
														<Tr>
															<Td
																expand={{
																	rowIndex: groupIdx,
																	isExpanded,
																	onToggle: () =>
																		setExpandedComponents((prev) => {
																			const next = new Set(prev);
																			if (next.has(group.component)) {
																				next.delete(group.component);
																			} else {
																				next.add(group.component);
																			}
																			return next;
																		}),
																}}
															/>
															<Td>{group.component}</Td>
															<Td>{group.reports.length}</Td>
															<Td>
																<SeverityCount
																	count={group.critical}
																	severity="Critical"
																/>
															</Td>
															<Td>
																<SeverityCount
																	count={group.high}
																	severity="High"
																/>
															</Td>
															<Td>
																<SeverityCount
																	count={group.medium}
																	severity="Medium"
																/>
															</Td>
															<Td>
																<SeverityCount
																	count={group.low}
																	severity="Low"
																/>
															</Td>
															<Td>{group.total}</Td>
															<Td>{group.fixable}</Td>
														</Tr>
														{isExpanded && selectedReport && (
															<Tr isExpanded>
																<Td colSpan={9}>
																	<ExpandableRowContent>
																		<Tabs
																			isFilled
																			activeKey={selectedArch}
																			onSelect={(_e, key) =>
																				setActiveArchTab((prev) => ({
																					...prev,
																					[group.component]: String(key),
																				}))
																			}
																		>
																			{group.reports.map((rpt) => (
																				<Tab
																					key={rpt.arch}
																					eventKey={rpt.arch}
																					title={
																						<TabTitleText>
																							{rpt.arch} ({rpt.total})
																						</TabTitleText>
																					}
																				>
																					<div style={{ padding: "1rem 0" }}>
																						<Flex
																							spaceItems={{
																								default: "spaceItemsLg",
																							}}
																							style={{ marginBottom: "1rem" }}
																						>
																							<FlexItem>
																								Critical:{" "}
																								<SeverityCount
																									count={rpt.critical}
																									severity="Critical"
																								/>
																							</FlexItem>
																							<FlexItem>
																								High:{" "}
																								<SeverityCount
																									count={rpt.high}
																									severity="High"
																								/>
																							</FlexItem>
																							<FlexItem>
																								Medium:{" "}
																								<SeverityCount
																									count={rpt.medium}
																									severity="Medium"
																								/>
																							</FlexItem>
																							<FlexItem>
																								Low:{" "}
																								<SeverityCount
																									count={rpt.low}
																									severity="Low"
																								/>
																							</FlexItem>
																							<FlexItem>
																								Total: {rpt.total}
																							</FlexItem>
																							<FlexItem>
																								Fixable: {rpt.fixable}
																							</FlexItem>
																						</Flex>
																						{rpt.vulnerabilities &&
																						rpt.vulnerabilities.length > 0 ? (
																							<VulnerabilitiesTable
																								vulnerabilities={
																									rpt.vulnerabilities
																								}
																							/>
																						) : (
																							<em>
																								No vulnerabilities recorded.
																							</em>
																						)}
																					</div>
																				</Tab>
																			))}
																		</Tabs>
																	</ExpandableRowContent>
																</Td>
															</Tr>
														)}
													</Tbody>
												);
											})}
										</Table>
									</Tab>
								)}
							</Tabs>
						</CardBody>
					</Card>
				)}

				{/* Bug Verification Table */}
				{(issues ?? []).length > 0 && (
					<Card isCompact style={{ marginBottom: "1rem" }}>
						<CardTitle>{`Bug Verification (${(issues ?? []).length})`}</CardTitle>
						<CardBody>
							<IssuesTable issues={issues ?? []} />
						</CardBody>
					</Card>
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
		{
			label: "Tests passed",
			done: allTestsPassed,
			warning: !hasTests,
			danger: hasTests && !allTestsPassed,
		},
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
									: item.danger
										? "danger"
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

const severityLabelColor: Record<string, "red" | "orange" | "yellow" | "grey"> =
	{
		Critical: "red",
		High: "red",
		Medium: "orange",
		Low: "yellow",
	};

function SeverityCount({
	count,
	severity,
}: {
	count: number;
	severity: string;
}) {
	if (count === 0) return <>{"\u2014"}</>;
	return (
		<Label color={severityLabelColor[severity] ?? "grey"} isCompact>
			{count}
		</Label>
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
