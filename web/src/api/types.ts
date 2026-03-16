export interface ComponentRecord {
	id: number;
	snapshot_id: number;
	component: string;
	git_sha: string;
	image_url: string;
	git_url: string;
}

export interface TestCase {
	id: number;
	test_suite_id: number;
	name: string;
	status: string;
	duration_ms: number;
	message?: string;
	trace?: string;
	file_path?: string;
	suite?: string;
	retries: number;
	flaky: boolean;
}

export interface TestSuite {
	id: number;
	snapshot_id: number;
	name: string;
	status: string;
	pipeline_run: string;
	tool_name: string;
	tool_version: string;
	tests: number;
	passed: number;
	failed: number;
	skipped: number;
	pending: number;
	other: number;
	flaky: number;
	start_time: number;
	stop_time: number;
	duration_ms: number;
	created_at: string;
	test_cases?: TestCase[];
}

export interface Vulnerability {
	id: number;
	report_id: number;
	name: string;
	severity: string;
	package_name: string;
	package_version: string;
	fixed_in_version: string;
	description: string;
	link: string;
}

export interface VulnerabilityReport {
	id: number;
	snapshot_id: number;
	component: string;
	arch: string;
	total: number;
	critical: number;
	high: number;
	medium: number;
	low: number;
	unknown: number;
	fixable: number;
	created_at: string;
	vulnerabilities?: Vulnerability[];
}

export interface SnapshotRecord {
	id: number;
	application: string;
	name: string;
	tests_passed: boolean;
	has_tests: boolean;
	created_at: string;
	components?: ComponentRecord[];
	test_suites?: TestSuite[];
	vulnerability_reports?: VulnerabilityReport[];
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
	qa_contact: string;
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

export interface ReleaseOverview {
	release: ReleaseVersion;
	issue_summary?: IssueSummary;
	readiness: ReadinessResponse;
	snapshot?: SnapshotRecord;
}

export interface DashboardConfig {
	jira_base_url: string;
	jira_project: string;
}
