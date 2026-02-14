import { useEffect, useState } from "react";
import { useParams, Link } from "react-router-dom";
import {
  PageSection,
  Title,
  Spinner,
  EmptyState,
  EmptyStateBody,
  Card,
  CardTitle,
  CardBody,
  DescriptionList,
  DescriptionListGroup,
  DescriptionListTerm,
  DescriptionListDescription,
  Label,
  Flex,
  FlexItem,
  Breadcrumb,
  BreadcrumbItem,
} from "@patternfly/react-core";
import {
  Table,
  Thead,
  Tbody,
  Tr,
  Th,
  Td,
} from "@patternfly/react-table";
import {
  CheckCircleIcon,
  ExclamationCircleIcon,
} from "@patternfly/react-icons";
import type {
  ApplicationSummary,
  IssueSummary,
  JiraIssue,
  ReleaseVersion,
} from "../api/types";
import {
  listApplications,
  getSnapshot,
  listIssues,
  getIssueSummary,
  getReleaseVersion,
} from "../api/client";
import type { SnapshotRecord } from "../api/types";
import StatusLabel from "../components/StatusLabel";
import TestResultDonut from "../components/TestResultDonut";

export default function ReleaseDetail() {
  const { app } = useParams<{ app: string }>();
  const [appSummary, setAppSummary] = useState<ApplicationSummary | null>(null);
  const [snapshot, setSnapshot] = useState<SnapshotRecord | null>(null);
  const [issues, setIssues] = useState<JiraIssue[]>([]);
  const [issueSummary, setIssueSummary] = useState<IssueSummary | null>(null);
  const [releaseVersion, setReleaseVersion] = useState<ReleaseVersion | null>(
    null,
  );
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    if (!app) return;

    const fetchData = async () => {
      try {
        // Get the latest snapshot for this application
        const apps = await listApplications();
        const summary = (apps ?? []).find((a) => a.application === app);
        setAppSummary(summary ?? null);

        if (summary?.latest_snapshot) {
          const snap = await getSnapshot(summary.latest_snapshot.name);
          setSnapshot(snap);
        }

        // Fetch JIRA data (may 404 if not configured)
        try {
          const [issueList, summary2, version] = await Promise.all([
            listIssues(app),
            getIssueSummary(app),
            getReleaseVersion(app),
          ]);
          setIssues(issueList ?? []);
          setIssueSummary(summary2);
          setReleaseVersion(version);
        } catch {
          // JIRA not configured
        }
      } catch (err) {
        console.error(err);
      } finally {
        setLoading(false);
      }
    };
    fetchData();
  }, [app]);

  if (loading) {
    return (
      <PageSection>
        <Spinner />
      </PageSection>
    );
  }

  if (!appSummary) {
    return (
      <PageSection>
        <EmptyState>
          <Title headingLevel="h2" size="lg">
            Application not found
          </Title>
          <EmptyStateBody>
            No data found for application &quot;{app}&quot;.
          </EmptyStateBody>
        </EmptyState>
      </PageSection>
    );
  }

  const testTotals = (snapshot?.test_results ?? []).reduce(
    (acc, tr) => ({
      total: acc.total + tr.total,
      passed: acc.passed + tr.passed,
      failed: acc.failed + tr.failed,
      skipped: acc.skipped + tr.skipped,
    }),
    { total: 0, passed: 0, failed: 0, skipped: 0 },
  );

  return (
    <>
      <PageSection>
        <Breadcrumb>
          <BreadcrumbItem>
            <Link to="/">Releases</Link>
          </BreadcrumbItem>
          <BreadcrumbItem isActive>{app}</BreadcrumbItem>
        </Breadcrumb>
      </PageSection>

      <PageSection>
        <Flex
          justifyContent={{ default: "justifyContentSpaceBetween" }}
          alignItems={{ default: "alignItemsCenter" }}
          style={{ marginBottom: "1rem" }}
        >
          <FlexItem>
            <Title headingLevel="h1">{app}</Title>
          </FlexItem>
          <FlexItem>
            <Link to={`/releases/${encodeURIComponent(app!)}/snapshots`}>
              View all snapshots ({appSummary.snapshot_count})
            </Link>
          </FlexItem>
        </Flex>

        {releaseVersion && <ReleaseSignal version={releaseVersion} issueSummary={issueSummary} snapshot={snapshot} />}

        {snapshot && (
          <>
            <Card isCompact style={{ marginBottom: "1rem" }}>
              <CardTitle>Latest Snapshot</CardTitle>
              <CardBody>
                <Flex>
                  <FlexItem>
                    <TestResultDonut {...testTotals} />
                  </FlexItem>
                  <FlexItem grow={{ default: "grow" }}>
                    <DescriptionList isCompact isHorizontal>
                      <DescriptionListGroup>
                        <DescriptionListTerm>Snapshot</DescriptionListTerm>
                        <DescriptionListDescription>
                          {snapshot.name}
                        </DescriptionListDescription>
                      </DescriptionListGroup>
                      <DescriptionListGroup>
                        <DescriptionListTerm>
                          Trigger Component
                        </DescriptionListTerm>
                        <DescriptionListDescription>
                          {snapshot.trigger_component}
                        </DescriptionListDescription>
                      </DescriptionListGroup>
                      <DescriptionListGroup>
                        <DescriptionListTerm>Git SHA</DescriptionListTerm>
                        <DescriptionListDescription>
                          <code>
                            {snapshot.trigger_git_sha?.substring(0, 12)}
                          </code>
                        </DescriptionListDescription>
                      </DescriptionListGroup>
                      <DescriptionListGroup>
                        <DescriptionListTerm>Pipeline Run</DescriptionListTerm>
                        <DescriptionListDescription>
                          {snapshot.trigger_pipeline_run}
                        </DescriptionListDescription>
                      </DescriptionListGroup>
                      <DescriptionListGroup>
                        <DescriptionListTerm>Tests</DescriptionListTerm>
                        <DescriptionListDescription>
                          {snapshot.tests_passed ? (
                            <Label color="green" icon={<CheckCircleIcon />}>
                              Passed
                            </Label>
                          ) : (
                            <Label color="red" icon={<ExclamationCircleIcon />}>
                              Failed
                            </Label>
                          )}
                        </DescriptionListDescription>
                      </DescriptionListGroup>
                      <DescriptionListGroup>
                        <DescriptionListTerm>Released</DescriptionListTerm>
                        <DescriptionListDescription>
                          {snapshot.released ? (
                            <Label color="green">Yes</Label>
                          ) : (
                            <Label color="grey">No</Label>
                          )}
                        </DescriptionListDescription>
                      </DescriptionListGroup>
                      {snapshot.release_blocked_reason && (
                        <DescriptionListGroup>
                          <DescriptionListTerm>Blocked</DescriptionListTerm>
                          <DescriptionListDescription>
                            {snapshot.release_blocked_reason}
                          </DescriptionListDescription>
                        </DescriptionListGroup>
                      )}
                      <DescriptionListGroup>
                        <DescriptionListTerm>Created</DescriptionListTerm>
                        <DescriptionListDescription>
                          {new Date(snapshot.created_at).toLocaleString()}
                        </DescriptionListDescription>
                      </DescriptionListGroup>
                    </DescriptionList>
                  </FlexItem>
                </Flex>
              </CardBody>
            </Card>

            {/* Components Table */}
            {snapshot.components && snapshot.components.length > 0 && (
              <Card isCompact style={{ marginBottom: "1rem" }}>
                <CardTitle>Components</CardTitle>
                <CardBody>
                  <Table variant="compact">
                    <Thead>
                      <Tr>
                        <Th>Component</Th>
                        <Th>Git SHA</Th>
                        <Th>Image</Th>
                      </Tr>
                    </Thead>
                    <Tbody>
                      {snapshot.components.map((c) => (
                        <Tr key={c.id}>
                          <Td>{c.component}</Td>
                          <Td>
                            <code>{c.git_sha?.substring(0, 12)}</code>
                          </Td>
                          <Td>
                            <code style={{ fontSize: "0.85em" }}>
                              {c.image_url}
                            </code>
                          </Td>
                        </Tr>
                      ))}
                    </Tbody>
                  </Table>
                </CardBody>
              </Card>
            )}

            {/* Test Results Table */}
            {snapshot.test_results && snapshot.test_results.length > 0 && (
              <Card isCompact style={{ marginBottom: "1rem" }}>
                <CardTitle>Integration Test Results</CardTitle>
                <CardBody>
                  <Table variant="compact">
                    <Thead>
                      <Tr>
                        <Th>Scenario</Th>
                        <Th>Status</Th>
                        <Th modifier="fitContent">Passed</Th>
                        <Th modifier="fitContent">Failed</Th>
                        <Th modifier="fitContent">Skipped</Th>
                        <Th modifier="fitContent">Total</Th>
                        <Th modifier="fitContent">Duration</Th>
                      </Tr>
                    </Thead>
                    <Tbody>
                      {snapshot.test_results.map((tr) => (
                        <Tr key={tr.id}>
                          <Td>{tr.scenario}</Td>
                          <Td>
                            <StatusLabel status={tr.status} />
                          </Td>
                          <Td>{tr.passed}</Td>
                          <Td>{tr.failed}</Td>
                          <Td>{tr.skipped}</Td>
                          <Td>{tr.total}</Td>
                          <Td>{formatDuration(tr.duration_sec)}</Td>
                        </Tr>
                      ))}
                    </Tbody>
                  </Table>
                </CardBody>
              </Card>
            )}
          </>
        )}

        {/* Approval Progress */}
        <ApprovalProgress snapshot={snapshot} issueSummary={issueSummary} />

        {/* Bug Verification Table */}
        {issues.length > 0 && (
          <Card isCompact style={{ marginBottom: "1rem" }}>
            <CardTitle>Bug Verification</CardTitle>
            <CardBody>
              <IssuesTable issues={issues} />
            </CardBody>
          </Card>
        )}
      </PageSection>
    </>
  );
}

function ReleaseSignal({
  version,
  issueSummary,
  snapshot,
}: {
  version: ReleaseVersion;
  issueSummary: IssueSummary | null;
  snapshot: SnapshotRecord | null;
}) {
  let color: "green" | "yellow" | "red" = "green";
  let text = "On Track";

  const now = new Date();
  const releaseDate = version.release_date
    ? new Date(version.release_date)
    : null;
  const daysUntilRelease = releaseDate
    ? Math.ceil(
        (releaseDate.getTime() - now.getTime()) / (1000 * 60 * 60 * 24),
      )
    : null;

  const testsFailing = snapshot && !snapshot.tests_passed;
  const openIssues = issueSummary && issueSummary.open > 0;

  if (
    (daysUntilRelease !== null && daysUntilRelease < 0) ||
    (testsFailing && openIssues)
  ) {
    color = "red";
    text = "Blocked";
  } else if (testsFailing || openIssues || (daysUntilRelease !== null && daysUntilRelease <= 3)) {
    color = "yellow";
    text = "At Risk";
  }

  return (
    <Card isCompact style={{ marginBottom: "1rem" }}>
      <CardBody>
        <Flex alignItems={{ default: "alignItemsCenter" }} gap={{ default: "gapMd" }}>
          <FlexItem>
            <Label color={color} isCompact>
              {text}
            </Label>
          </FlexItem>
          <FlexItem>
            <strong>Target: </strong>
            {releaseDate ? releaseDate.toLocaleDateString() : "TBD"}
            {daysUntilRelease !== null && ` (${daysUntilRelease} days)`}
          </FlexItem>
          {version.released && (
            <FlexItem>
              <Label color="green">Released</Label>
            </FlexItem>
          )}
        </Flex>
      </CardBody>
    </Card>
  );
}

function ApprovalProgress({
  snapshot,
  issueSummary,
}: {
  snapshot: SnapshotRecord | null;
  issueSummary: IssueSummary | null;
}) {
  const buildsReady =
    snapshot !== null &&
    snapshot.components !== undefined &&
    snapshot.components.length > 0;
  const allTestsPassed = snapshot?.tests_passed ?? false;
  const bugsVerified =
    issueSummary !== null &&
    issueSummary.total > 0 &&
    issueSummary.open === 0;
  const qeSignOff = allTestsPassed && (bugsVerified || issueSummary === null);

  const items = [
    { label: "Builds ready", done: buildsReady },
    { label: "Integration tests passed", done: allTestsPassed },
    ...(issueSummary
      ? [{ label: "Bug verification complete", done: bugsVerified }]
      : []),
    { label: "QE sign off", done: qeSignOff },
  ];

  return (
    <Card isCompact style={{ marginBottom: "1rem" }}>
      <CardTitle>Approval Progress</CardTitle>
      <CardBody>
        <Flex gap={{ default: "gapLg" }}>
          {items.map((item) => (
            <FlexItem key={item.label}>
              {item.done ? (
                <Label color="green" icon={<CheckCircleIcon />}>
                  {item.label}
                </Label>
              ) : (
                <Label color="grey">{item.label}</Label>
              )}
            </FlexItem>
          ))}
        </Flex>
      </CardBody>
    </Card>
  );
}

function IssuesTable({ issues }: { issues: JiraIssue[] }) {
  return (
    <Table variant="compact">
      <Thead>
        <Tr>
          <Th>Key</Th>
          <Th>Type</Th>
          <Th>Priority</Th>
          <Th>Summary</Th>
          <Th>Status</Th>
          <Th>Assignee</Th>
        </Tr>
      </Thead>
      <Tbody>
        {issues.map((issue) => (
          <Tr key={issue.key}>
            <Td>
              <a href={issue.link} target="_blank" rel="noopener noreferrer">
                {issue.key}
              </a>
            </Td>
            <Td>{issue.issue_type}</Td>
            <Td>
              <PriorityLabel priority={issue.priority} />
            </Td>
            <Td>{issue.summary}</Td>
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

function PriorityLabel({ priority }: { priority: string }) {
  const p = priority.toLowerCase();
  if (p === "critical" || p === "blocker") {
    return <Label color="red">{priority}</Label>;
  }
  if (p === "major") {
    return <Label color="yellow">{priority}</Label>;
  }
  return <Label color="grey">{priority}</Label>;
}

function formatDuration(seconds: number): string {
  if (seconds < 60) return `${seconds.toFixed(1)}s`;
  const m = Math.floor(seconds / 60);
  const s = Math.round(seconds % 60);
  return `${m}m ${s}s`;
}
