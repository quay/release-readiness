import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import {
  PageSection,
  Title,
  Card,
  CardTitle,
  CardBody,
  Gallery,
  Spinner,
  EmptyState,
  EmptyStateBody,
  Label,
  Flex,
  FlexItem,
  DescriptionList,
  DescriptionListGroup,
  DescriptionListTerm,
  DescriptionListDescription,
} from "@patternfly/react-core";
import {
  CheckCircleIcon,
  ExclamationCircleIcon,
} from "@patternfly/react-icons";
import type { ApplicationSummary, IssueSummary } from "../api/types";
import { listApplications, getIssueSummary } from "../api/client";
import TestResultDonut from "../components/TestResultDonut";

export default function ReleasesOverview() {
  const [apps, setApps] = useState<ApplicationSummary[]>([]);
  const [issueSummaries, setIssueSummaries] = useState<
    Record<string, IssueSummary>
  >({});
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    listApplications()
      .then((data) => {
        setApps(data ?? []);
        // Fetch issue summaries in parallel (will 404 if JIRA not configured)
        for (const app of data ?? []) {
          getIssueSummary(app.application)
            .then((summary) =>
              setIssueSummaries((prev) => ({
                ...prev,
                [app.application]: summary,
              })),
            )
            .catch(() => {
              /* JIRA not configured */
            });
        }
      })
      .catch(console.error)
      .finally(() => setLoading(false));
  }, []);

  if (loading) {
    return (
      <PageSection>
        <Spinner />
      </PageSection>
    );
  }

  if (apps.length === 0) {
    return (
      <PageSection>
        <EmptyState>
          <Title headingLevel="h2" size="lg">
            No releases found
          </Title>
          <EmptyStateBody>
            No applications have been synced from S3 yet. Configure S3
            credentials and wait for the sync loop to run.
          </EmptyStateBody>
        </EmptyState>
      </PageSection>
    );
  }

  return (
    <PageSection>
      <Title headingLevel="h1" style={{ marginBottom: "1rem" }}>
        Release Readiness
      </Title>
      <Gallery hasGutter minWidths={{ default: "400px" }}>
        {apps.map((app) => (
          <ReleaseCard
            key={app.application}
            app={app}
            issueSummary={issueSummaries[app.application]}
          />
        ))}
      </Gallery>
    </PageSection>
  );
}

function ReleaseCard({
  app,
  issueSummary,
}: {
  app: ApplicationSummary;
  issueSummary?: IssueSummary;
}) {
  const snap = app.latest_snapshot;

  // Aggregate test results from latest snapshot
  const testTotals = (snap?.test_results ?? []).reduce(
    (acc, tr) => ({
      total: acc.total + tr.total,
      passed: acc.passed + tr.passed,
      failed: acc.failed + tr.failed,
      skipped: acc.skipped + tr.skipped,
    }),
    { total: 0, passed: 0, failed: 0, skipped: 0 },
  );

  return (
    <Card isCompact>
      <CardTitle>
        <Link
          to={`/releases/${encodeURIComponent(app.application)}`}
          style={{ textDecoration: "none", color: "inherit" }}
        >
          {app.application}
        </Link>
      </CardTitle>
      <CardBody>
        {snap ? (
          <Flex>
            <FlexItem>
              <TestResultDonut {...testTotals} />
            </FlexItem>
            <FlexItem grow={{ default: "grow" }}>
              <DescriptionList isCompact>
                <DescriptionListGroup>
                  <DescriptionListTerm>Tests</DescriptionListTerm>
                  <DescriptionListDescription>
                    {snap.tests_passed ? (
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
                  <DescriptionListTerm>Latest Snapshot</DescriptionListTerm>
                  <DescriptionListDescription>
                    <Link to={`/releases/${encodeURIComponent(app.application)}`}>
                      {snap.name}
                    </Link>
                  </DescriptionListDescription>
                </DescriptionListGroup>
                <DescriptionListGroup>
                  <DescriptionListTerm>Trigger</DescriptionListTerm>
                  <DescriptionListDescription>
                    {snap.trigger_component}{" "}
                    <code>{snap.trigger_git_sha?.substring(0, 12)}</code>
                  </DescriptionListDescription>
                </DescriptionListGroup>
                <DescriptionListGroup>
                  <DescriptionListTerm>Snapshots</DescriptionListTerm>
                  <DescriptionListDescription>
                    <Link
                      to={`/releases/${encodeURIComponent(app.application)}/snapshots`}
                    >
                      {app.snapshot_count} total
                    </Link>
                  </DescriptionListDescription>
                </DescriptionListGroup>
                {issueSummary && (
                  <DescriptionListGroup>
                    <DescriptionListTerm>Issues</DescriptionListTerm>
                    <DescriptionListDescription>
                      {issueSummary.verified}/{issueSummary.total} verified
                      {issueSummary.cves > 0 && ` | ${issueSummary.cves} CVEs`}
                    </DescriptionListDescription>
                  </DescriptionListGroup>
                )}
              </DescriptionList>
            </FlexItem>
          </Flex>
        ) : (
          <span>No snapshots yet</span>
        )}
      </CardBody>
    </Card>
  );
}
