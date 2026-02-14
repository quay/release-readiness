import type {
  ApplicationSummary,
  IssueSummary,
  JiraIssue,
  ReleaseVersion,
  SnapshotRecord,
} from "./types";

const BASE = "/api/v1";

async function fetchJSON<T>(url: string): Promise<T> {
  const res = await fetch(url);
  if (!res.ok) {
    throw new Error(`${res.status} ${res.statusText}`);
  }
  return res.json() as Promise<T>;
}

export function listApplications(): Promise<ApplicationSummary[]> {
  return fetchJSON(`${BASE}/applications`);
}

export function listSnapshots(
  application?: string,
  limit = 50,
  offset = 0,
): Promise<SnapshotRecord[]> {
  const params = new URLSearchParams();
  if (application) params.set("application", application);
  params.set("limit", String(limit));
  params.set("offset", String(offset));
  return fetchJSON(`${BASE}/snapshots?${params}`);
}

export function getSnapshot(name: string): Promise<SnapshotRecord> {
  return fetchJSON(`${BASE}/snapshots/${encodeURIComponent(name)}`);
}

export function listIssues(
  app: string,
  filters?: { label?: string; status?: string; type?: string },
): Promise<JiraIssue[]> {
  const params = new URLSearchParams();
  if (filters?.label) params.set("label", filters.label);
  if (filters?.status) params.set("status", filters.status);
  if (filters?.type) params.set("type", filters.type);
  const qs = params.toString();
  return fetchJSON(
    `${BASE}/releases/${encodeURIComponent(app)}/issues${qs ? `?${qs}` : ""}`,
  );
}

export function getIssueSummary(app: string): Promise<IssueSummary> {
  return fetchJSON(
    `${BASE}/releases/${encodeURIComponent(app)}/issues/summary`,
  );
}

export function getReleaseVersion(app: string): Promise<ReleaseVersion> {
  return fetchJSON(`${BASE}/releases/${encodeURIComponent(app)}/version`);
}
