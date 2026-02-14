export interface ComponentRecord {
  id: number;
  snapshot_id: number;
  component: string;
  git_sha: string;
  image_url: string;
  git_url: string;
}

export interface SnapshotTestResult {
  id: number;
  snapshot_id: number;
  scenario: string;
  status: string;
  pipeline_run: string;
  total: number;
  passed: number;
  failed: number;
  skipped: number;
  duration_sec: number;
  created_at: string;
}

export interface SnapshotRecord {
  id: number;
  application: string;
  name: string;
  trigger_component: string;
  trigger_git_sha: string;
  trigger_pipeline_run: string;
  tests_passed: boolean;
  released: boolean;
  release_blocked_reason?: string;
  created_at: string;
  components?: ComponentRecord[];
  test_results?: SnapshotTestResult[];
}

export interface ApplicationSummary {
  application: string;
  latest_snapshot?: SnapshotRecord;
  snapshot_count: number;
}

export interface JiraIssue {
  key: string;
  summary: string;
  status: string;
  priority: string;
  labels: string;
  fix_version: string;
  assignee: string;
  issue_type: string;
  resolution: string;
  link: string;
  updated_at: string;
}

export interface IssueSummary {
  total: number;
  verified: number;
  open: number;
  cves: number;
  bugs: number;
}

export interface ReleaseVersion {
  name: string;
  description: string;
  release_date?: string;
  released: boolean;
  archived: boolean;
  release_ticket_key?: string;
  release_ticket_assignee?: string;
  s3_application?: string;
  due_date?: string;
}

export interface ReadinessResponse {
  signal: "green" | "yellow" | "red";
  message: string;
}

export interface DashboardConfig {
  jira_base_url: string;
}
