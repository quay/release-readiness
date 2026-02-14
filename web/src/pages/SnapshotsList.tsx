import { useEffect, useState, useCallback } from "react";
import { useParams, Link } from "react-router-dom";
import {
  PageSection,
  Title,
  Spinner,
  EmptyState,
  EmptyStateBody,
  Breadcrumb,
  BreadcrumbItem,
  Pagination,
} from "@patternfly/react-core";
import {
  Table,
  Thead,
  Tbody,
  Tr,
  Th,
  Td,
} from "@patternfly/react-table";
import type { SnapshotRecord } from "../api/types";
import { listSnapshots, getRelease } from "../api/client";
import { useCachedFetch } from "../hooks/useCachedFetch";
import { formatReleaseName, githubCommitUrl } from "../utils/links";
import StatusLabel from "../components/StatusLabel";

const PAGE_SIZE = 50;

export default function SnapshotsList() {
  const { version } = useParams<{ version: string }>();
  const [snapshots, setSnapshots] = useState<SnapshotRecord[]>([]);
  const [loading, setLoading] = useState(true);
  const [page, setPage] = useState(1);
  const [hasMore, setHasMore] = useState(false);

  const { data: release } = useCachedFetch(
    version ? `release:${version}` : null,
    () => getRelease(version!),
  );

  const displayName = version ? formatReleaseName(version) : "";

  const fetchPage = useCallback(
    (p: number) => {
      if (!release?.s3_application) return;
      setLoading(true);
      listSnapshots(release.s3_application, PAGE_SIZE + 1, (p - 1) * PAGE_SIZE)
        .then((data) => {
          const rows = data ?? [];
          if (rows.length > PAGE_SIZE) {
            setHasMore(true);
            setSnapshots(rows.slice(0, PAGE_SIZE));
          } else {
            setHasMore(false);
            setSnapshots(rows);
          }
        })
        .catch(console.error)
        .finally(() => setLoading(false));
    },
    [release?.s3_application],
  );

  useEffect(() => {
    if (release?.s3_application) {
      fetchPage(1);
    }
  }, [release?.s3_application, fetchPage]);

  const onSetPage = (_: unknown, p: number) => {
    setPage(p);
    fetchPage(p);
  };

  return (
    <>
      <PageSection>
        <Breadcrumb>
          <BreadcrumbItem>
            <Link to="/">Releases</Link>
          </BreadcrumbItem>
          {version && (
            <BreadcrumbItem>
              <Link to={`/releases/${encodeURIComponent(version)}`}>
                {displayName}
              </Link>
            </BreadcrumbItem>
          )}
          <BreadcrumbItem isActive>Snapshots</BreadcrumbItem>
        </Breadcrumb>
      </PageSection>

      <PageSection>
        <Title headingLevel="h1" style={{ marginBottom: "1rem" }}>
          Snapshots{version ? ` - ${displayName}` : ""}
        </Title>

        {loading ? (
          <div style={{ textAlign: "center" }}><Spinner /></div>
        ) : snapshots.length === 0 ? (
          <EmptyState>
            <Title headingLevel="h2" size="lg">
              No snapshots
            </Title>
            <EmptyStateBody>
              No snapshots found for this release.
            </EmptyStateBody>
          </EmptyState>
        ) : (
          <>
            <Table variant="compact">
              <Thead>
                <Tr>
                  <Th>Snapshot</Th>
                  <Th>Application</Th>
                  <Th>Trigger</Th>
                  <Th>Tests</Th>
                  <Th>Released</Th>
                  <Th>Created</Th>
                </Tr>
              </Thead>
              <Tbody>
                {snapshots.map((s) => {
                  const commitUrl = githubCommitUrl(s.trigger_component, s.trigger_git_sha);
                  return (
                    <Tr key={s.id}>
                      <Td>
                        {s.trigger_pipeline_run ? (
                          <a href={s.trigger_pipeline_run} target="_blank" rel="noopener noreferrer">
                            {s.name}
                          </a>
                        ) : (
                          s.name
                        )}
                      </Td>
                      <Td>{s.application}</Td>
                      <Td>
                        {s.trigger_component}{" @ "}
                        {commitUrl ? (
                          <a href={commitUrl} target="_blank" rel="noopener noreferrer">
                            <code>{s.trigger_git_sha?.substring(0, 12)}</code>
                          </a>
                        ) : (
                          <code>{s.trigger_git_sha?.substring(0, 12)}</code>
                        )}
                      </Td>
                      <Td>
                        <StatusLabel
                          status={s.tests_passed ? "passed" : "failed"}
                        />
                      </Td>
                      <Td>
                        <StatusLabel
                          status={s.released ? "passed" : "pending"}
                        />
                      </Td>
                      <Td>{new Date(s.created_at).toLocaleString()}</Td>
                    </Tr>
                  );
                })}
              </Tbody>
            </Table>
            <Pagination
              itemCount={
                hasMore ? page * PAGE_SIZE + 1 : (page - 1) * PAGE_SIZE + snapshots.length
              }
              perPage={PAGE_SIZE}
              page={page}
              onSetPage={onSetPage}
              isCompact
              style={{ marginTop: "1rem" }}
            />
          </>
        )}
      </PageSection>
    </>
  );
}
