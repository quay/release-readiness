import { useState } from "react";
import { useNavigate, useSearchParams } from "react-router-dom";
import {
  PageSection,
  Card,
  CardTitle,
  CardBody,
  ExpandableSection,
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
  Progress,
  ProgressMeasureLocation,
  SearchInput,
  Title,
  Toolbar,
  ToolbarContent,
  ToolbarItem,
  ToolbarGroup,
  ToggleGroup,
  ToggleGroupItem,
} from "@patternfly/react-core";
import {
  CheckCircleIcon,
  ExclamationCircleIcon,
  ExclamationTriangleIcon,
  ListIcon,
  ThIcon,
} from "@patternfly/react-icons";
import type {
  ReleaseVersion,
  IssueSummary,
  ReadinessResponse,
  SnapshotRecord,
} from "../api/types";
import {
  listReleases,
  getReleaseIssueSummary,
  getReleaseReadiness,
  getReleaseSnapshot,
} from "../api/client";
import { useCachedFetch } from "../hooks/useCachedFetch";
import { useConfig } from "../hooks/useConfig";
import { formatReleaseName, jiraIssueUrl } from "../utils/links";

type SignalFilter = "all" | "red" | "yellow" | "green";
type ViewMode = "compact" | "expanded";

export default function ReleasesOverview() {
  const [searchParams, setSearchParams] = useSearchParams();
  const query = searchParams.get("q") ?? "";
  const signalFilter = (searchParams.get("signal") ?? "all") as SignalFilter;
  const viewMode = (searchParams.get("view") ?? "compact") as ViewMode;

  const config = useConfig();

  const { data: releases, loading } = useCachedFetch(
    "releases",
    listReleases,
  );

  const [releasedExpanded, setReleasedExpanded] = useState(false);

  const setParam = (key: string, value: string) => {
    setSearchParams((prev) => {
      const next = new URLSearchParams(prev);
      if (value && value !== "all" && value !== "compact") {
        next.set(key, value);
      } else {
        next.delete(key);
      }
      return next;
    });
  };

  if (loading && !releases) {
    return (
      <PageSection>
        <div style={{ textAlign: "center" }}>
          <Spinner />
        </div>
      </PageSection>
    );
  }

  const releaseList = releases ?? [];
  const activeReleases = releaseList.filter((r) => !r.released);
  const releasedReleases = releaseList.filter((r) => r.released);

  if (releaseList.length === 0) {
    return (
      <PageSection>
        <EmptyState>
          <Title headingLevel="h2" size="lg">
            No releases found
          </Title>
          <EmptyStateBody>
            No active releases discovered from JIRA. Ensure the JIRA sync is
            configured and release tickets with component &quot;-area/release&quot; exist.
          </EmptyStateBody>
        </EmptyState>
      </PageSection>
    );
  }

  const galleryMinWidth = viewMode === "compact" ? "300px" : "400px";

  return (
    <PageSection>
      <Toolbar>
        <ToolbarContent>
          <ToolbarItem>
            <SearchInput
              placeholder="Filter releases..."
              value={query}
              onChange={(_e, val) => setParam("q", val)}
              onClear={() => setParam("q", "")}
            />
          </ToolbarItem>
          <ToolbarItem>
            <ToggleGroup aria-label="Signal filter">
              {(["all", "red", "yellow", "green"] as const).map((s) => (
                <ToggleGroupItem
                  key={s}
                  text={s.charAt(0).toUpperCase() + s.slice(1)}
                  isSelected={signalFilter === s}
                  onChange={() => setParam("signal", s)}
                />
              ))}
            </ToggleGroup>
          </ToolbarItem>
          <ToolbarGroup align={{ default: "alignEnd" }}>
            <ToolbarItem>
              <ToggleGroup aria-label="View mode">
                <ToggleGroupItem
                  icon={<ListIcon />}
                  aria-label="Compact view"
                  isSelected={viewMode === "compact"}
                  onChange={() => setParam("view", "compact")}
                />
                <ToggleGroupItem
                  icon={<ThIcon />}
                  aria-label="Expanded view"
                  isSelected={viewMode === "expanded"}
                  onChange={() => setParam("view", "expanded")}
                />
              </ToggleGroup>
            </ToolbarItem>
          </ToolbarGroup>
        </ToolbarContent>
      </Toolbar>

      <Gallery hasGutter minWidths={{ default: galleryMinWidth }}>
        {activeReleases.map((rel) => (
          <ReleaseCardWrapper
            key={rel.name}
            release={rel}
            query={query}
            signalFilter={signalFilter}
            viewMode={viewMode}
            jiraBaseUrl={config?.jira_base_url}
          />
        ))}
      </Gallery>

      {releasedReleases.length > 0 && (
        <ExpandableSection
          toggleText={`Released (${releasedReleases.length})`}
          isExpanded={releasedExpanded}
          onToggle={(_e, val) => setReleasedExpanded(val)}
          style={{ marginTop: "1.5rem" }}
        >
          <Gallery hasGutter minWidths={{ default: galleryMinWidth }}>
            {releasedReleases.map((rel) => (
              <ReleaseCardWrapper
                key={rel.name}
                release={rel}
                query={query}
                signalFilter={signalFilter}
                viewMode={viewMode}
                jiraBaseUrl={config?.jira_base_url}
              />
            ))}
          </Gallery>
        </ExpandableSection>
      )}
    </PageSection>
  );
}

function ReleaseCardWrapper({
  release,
  query,
  signalFilter,
  viewMode,
  jiraBaseUrl,
}: {
  release: ReleaseVersion;
  query: string;
  signalFilter: SignalFilter;
  viewMode: ViewMode;
  jiraBaseUrl?: string;
}) {
  const { data: issueSummary } = useCachedFetch(
    `issueSummary:${release.name}`,
    () => getReleaseIssueSummary(release.name),
  );
  const { data: readinessSignal } = useCachedFetch(
    `readiness:${release.name}`,
    () => getReleaseReadiness(release.name),
  );
  const { data: snapshot } = useCachedFetch(
    release.s3_application ? `snapshot:${release.name}` : null,
    () => getReleaseSnapshot(release.name),
  );

  // Apply filters
  const displayName = formatReleaseName(release.name);
  if (query && !displayName.toLowerCase().includes(query.toLowerCase()) && !release.name.toLowerCase().includes(query.toLowerCase())) {
    return null;
  }
  if (signalFilter !== "all" && readinessSignal?.signal !== signalFilter) {
    return null;
  }

  return (
    <ReleaseCard
      release={release}
      issueSummary={issueSummary}
      readinessSignal={readinessSignal}
      snapshot={snapshot}
      viewMode={viewMode}
      jiraBaseUrl={jiraBaseUrl}
    />
  );
}

function ReleaseCard({
  release,
  issueSummary,
  readinessSignal,
  snapshot,
  viewMode,
  jiraBaseUrl,
}: {
  release: ReleaseVersion;
  issueSummary?: IssueSummary;
  readinessSignal?: ReadinessResponse;
  snapshot?: SnapshotRecord;
  viewMode: ViewMode;
  jiraBaseUrl?: string;
}) {
  const dueDate = release.due_date ? new Date(release.due_date) : null;
  const releaseDate = release.release_date
    ? new Date(release.release_date)
    : null;
  const targetDate = dueDate ?? releaseDate;

  const signalColor = readinessSignal?.signal ?? "grey";
  const signalIcon =
    signalColor === "green" ? (
      <CheckCircleIcon />
    ) : signalColor === "red" ? (
      <ExclamationCircleIcon />
    ) : signalColor === "yellow" ? (
      <ExclamationTriangleIcon />
    ) : undefined;

  const verifiedPercent =
    issueSummary && issueSummary.total > 0
      ? Math.round((issueSummary.verified / issueSummary.total) * 100)
      : 0;

  const navigate = useNavigate();
  const displayName = formatReleaseName(release.name);

  const ticketLink =
    release.release_ticket_key && jiraBaseUrl
      ? jiraIssueUrl(release.release_ticket_key, jiraBaseUrl)
      : null;

  return (
    <Card
      isCompact
      isClickable
      style={{ cursor: "pointer" }}
      onClick={(e) => {
        if ((e.target as HTMLElement).closest("a, button")) return;
        navigate(`/releases/${encodeURIComponent(release.name)}`);
      }}
    >
      <CardTitle>
        <Flex
          justifyContent={{ default: "justifyContentSpaceBetween" }}
          alignItems={{ default: "alignItemsCenter" }}
        >
          <FlexItem>
            {displayName}
          </FlexItem>
          <FlexItem>
            {readinessSignal && (
              <Label
                color={
                  signalColor === "green"
                    ? "green"
                    : signalColor === "red"
                      ? "red"
                      : signalColor === "yellow"
                        ? "yellow"
                        : "grey"
                }
                icon={signalIcon}
                isCompact
              >
                {readinessSignal.message}
              </Label>
            )}
          </FlexItem>
        </Flex>
      </CardTitle>
      <CardBody>
        {viewMode === "compact" ? (
          <DescriptionList isCompact isHorizontal>
            {targetDate && (
              <DescriptionListGroup>
                <DescriptionListTerm>Target</DescriptionListTerm>
                <DescriptionListDescription>
                  {targetDate.toLocaleDateString()}
                </DescriptionListDescription>
              </DescriptionListGroup>
            )}
            {release.release_ticket_key && (
              <DescriptionListGroup>
                <DescriptionListTerm>Ticket</DescriptionListTerm>
                <DescriptionListDescription>
                  {ticketLink ? (
                    <a href={ticketLink} target="_blank" rel="noopener noreferrer">
                      {release.release_ticket_key}
                    </a>
                  ) : (
                    release.release_ticket_key
                  )}
                </DescriptionListDescription>
              </DescriptionListGroup>
            )}
            {snapshot && (
              <DescriptionListGroup>
                <DescriptionListTerm>Tests</DescriptionListTerm>
                <DescriptionListDescription>
                  {snapshot.tests_passed ? (
                    <Label color="green" icon={<CheckCircleIcon />} isCompact>
                      Passed
                    </Label>
                  ) : (
                    <Label color="red" icon={<ExclamationCircleIcon />} isCompact>
                      Failed
                    </Label>
                  )}
                </DescriptionListDescription>
              </DescriptionListGroup>
            )}
          </DescriptionList>
        ) : (
          <Flex direction={{ default: "column" }}>
            <FlexItem>
              <Flex justifyContent={{ default: "justifyContentSpaceBetween" }}>
                {targetDate && (
                  <FlexItem>
                    <span style={{ fontWeight: 600, fontSize: "0.85rem" }}>Target Date</span>
                    <div>{targetDate.toLocaleDateString()}</div>
                  </FlexItem>
                )}
                {release.release_ticket_key && (
                  <FlexItem>
                    <span style={{ fontWeight: 600, fontSize: "0.85rem" }}>Ticket</span>
                    <div>
                      {ticketLink ? (
                        <a href={ticketLink} target="_blank" rel="noopener noreferrer">
                          {release.release_ticket_key}
                        </a>
                      ) : (
                        release.release_ticket_key
                      )}
                    </div>
                  </FlexItem>
                )}
                {snapshot && (
                  <FlexItem>
                    <span style={{ fontWeight: 600, fontSize: "0.85rem" }}>Tests</span>
                    <div>
                      {snapshot.tests_passed ? (
                        <Label color="green" icon={<CheckCircleIcon />} isCompact>
                          Passed
                        </Label>
                      ) : (
                        <Label color="red" icon={<ExclamationCircleIcon />} isCompact>
                          Failed
                        </Label>
                      )}
                    </div>
                  </FlexItem>
                )}
                {issueSummary && issueSummary.cves > 0 && (
                  <FlexItem>
                    <span style={{ fontWeight: 600, fontSize: "0.85rem" }}>CVEs</span>
                    <div>{issueSummary.cves}</div>
                  </FlexItem>
                )}
              </Flex>
            </FlexItem>
            {issueSummary && issueSummary.total > 0 && (
              <FlexItem>
                <Progress
                  value={verifiedPercent}
                  title={`${issueSummary.verified}/${issueSummary.total} verified`}
                  measureLocation={ProgressMeasureLocation.outside}
                  size="sm"
                />
              </FlexItem>
            )}
          </Flex>
        )}
      </CardBody>
    </Card>
  );
}
