// Package jira provides a client for querying JIRA Server and Cloud REST APIs.
package jira

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Config holds JIRA connection settings.
type Config struct {
	BaseURL            string // e.g. https://issues.redhat.com
	Token              string // Personal Access Token (Server) or API token (Cloud)
	Project            string // e.g. PROJQUAY
	TargetVersionField string // custom field name for Target Version (e.g. customfield_12319940)
}

// Client is a JIRA REST API client.
type Client struct {
	baseURL            string
	token              string
	project            string
	targetVersionField string
	httpClient         *http.Client
	minDelay           time.Duration // minimum delay between requests
}

// New creates a new JIRA client.
func New(cfg Config) *Client {
	return &Client{
		baseURL:            strings.TrimRight(cfg.BaseURL, "/"),
		token:              cfg.Token,
		project:            cfg.Project,
		targetVersionField: cfg.TargetVersionField,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		minDelay: 1 * time.Second,
	}
}

// Issue represents a JIRA issue from the REST API.
type Issue struct {
	Key    string      `json:"key"`
	Fields IssueFields `json:"fields"`
}

// IssueFields holds the fields we care about from a JIRA issue.
type IssueFields struct {
	Summary    string        `json:"summary"`
	Status     StatusField   `json:"status"`
	Priority   PriorityField `json:"priority"`
	Labels     []string      `json:"labels"`
	FixVersions []VersionField `json:"fixVersions"`
	Assignee   *UserField    `json:"assignee"`
	IssueType  TypeField     `json:"issuetype"`
	Resolution *ResField     `json:"resolution"`
	Updated    string        `json:"updated"`
	DueDate    string        `json:"duedate"`
	Components []ComponentField `json:"components"`
}

type StatusField struct {
	Name string `json:"name"`
}

type PriorityField struct {
	Name string `json:"name"`
}

type VersionField struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	ReleaseDate string `json:"releaseDate"`
	Released    bool   `json:"released"`
	Archived    bool   `json:"archived"`
}

type UserField struct {
	DisplayName string `json:"displayName"`
}

type TypeField struct {
	Name string `json:"name"`
}

type ResField struct {
	Name string `json:"name"`
}

type ComponentField struct {
	Name string `json:"name"`
}

type searchResponse struct {
	Total      int     `json:"total"`
	MaxResults int     `json:"maxResults"`
	StartAt    int     `json:"startAt"`
	Issues     []Issue `json:"issues"`
}

// ActiveRelease represents a release discovered from JIRA via the -area/release component.
type ActiveRelease struct {
	FixVersion       string     // e.g. "quay-v3.16.3"
	DueDate          *time.Time // from the release ticket's dueDate field
	ReleaseTicketKey string     // e.g. "PROJQUAY-10276"
	Assignee         string     // display name of the release ticket assignee
	S3Application    string     // e.g. "quay-v3-16" (derived from fixVersion)
}

// BaseURL returns the configured JIRA base URL.
func (c *Client) BaseURL() string {
	return c.baseURL
}

// versionRe matches version patterns like "v3.16.2", "v2.0.10", "3.16.2" in release ticket summaries.
// Examples:
//   - "Release Quay v3.16.2"       → product="quay", version="3.16.2"
//   - "Release OMR v2.0.10"        → product="omr", version="2.0.10"
//   - "⦗konflux⦘ Quay v3.15.3"    → product="quay", version="3.15.3"
var versionRe = regexp.MustCompile(`(?i)(?:(\w+)\s+)?v?(\d+\.\d+(?:\.\d+)?)`)

// ParseVersionFromSummary extracts the product and version from a release ticket summary.
// Returns product (lowercased), version string, and whether a match was found.
func ParseVersionFromSummary(summary string) (product, version string, ok bool) {
	m := versionRe.FindStringSubmatch(summary)
	if m == nil {
		return "", "", false
	}
	product = strings.ToLower(m[1])
	version = m[2]
	return product, version, true
}

// DiscoverActiveReleases queries JIRA for active release tickets using the -area/release component.
// Returns releases that are not Closed/Done, each with their fixVersion (parsed from
// the ticket summary), dueDate, and ticket key.
func (c *Client) DiscoverActiveReleases(ctx context.Context) ([]ActiveRelease, error) {
	jql := fmt.Sprintf(
		`project=%s AND component="-area/release" AND status NOT IN (Closed, Done)`,
		c.project,
	)
	fields := "summary,status,fixVersions,duedate,components,assignee"

	var allIssues []Issue
	startAt := 0
	maxResults := 100

	for {
		params := url.Values{
			"jql":        {jql},
			"fields":     {fields},
			"startAt":    {fmt.Sprintf("%d", startAt)},
			"maxResults": {fmt.Sprintf("%d", maxResults)},
		}

		reqURL := fmt.Sprintf("%s/rest/api/2/search?%s", c.baseURL, params.Encode())
		body, err := c.doGetWithRetry(ctx, reqURL)
		if err != nil {
			return nil, fmt.Errorf("discover releases: %w", err)
		}

		var resp searchResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, fmt.Errorf("decode search response: %w", err)
		}

		allIssues = append(allIssues, resp.Issues...)

		if startAt+len(resp.Issues) >= resp.Total {
			break
		}
		startAt += len(resp.Issues)
	}

	var releases []ActiveRelease
	for _, issue := range allIssues {
		product, version, ok := ParseVersionFromSummary(issue.Fields.Summary)
		if !ok {
			continue
		}

		// JIRA fixVersions always use "{product}-v{version}" format (e.g. "quay-v3.16.2", "omr-v2.0.10")
		fixVersion := version
		if product != "" && product != "release" {
			fixVersion = product + "-v" + version
		}

		s3App := FixVersionToS3App(fixVersion)
		if s3App == "" {
			continue
		}

		assignee := ""
		if issue.Fields.Assignee != nil {
			assignee = issue.Fields.Assignee.DisplayName
		}

		rel := ActiveRelease{
			FixVersion:       fixVersion,
			ReleaseTicketKey: issue.Key,
			Assignee:         assignee,
			S3Application:    s3App,
		}

		if issue.Fields.DueDate != "" {
			t, err := time.Parse("2006-01-02", issue.Fields.DueDate)
			if err == nil {
				rel.DueDate = &t
			}
		}

		releases = append(releases, rel)
	}

	return releases, nil
}

// buildSearchJQL constructs the JQL for searching issues by version.
// When a target version custom field is configured, the JQL uses an OR clause
// to match issues by either fixVersion or the custom field.
func (c *Client) buildSearchJQL(fixVersion string) string {
	if c.targetVersionField == "" {
		return fmt.Sprintf(`project=%s AND fixVersion="%s"`, c.project, fixVersion)
	}
	cfID := strings.TrimPrefix(c.targetVersionField, "customfield_")
	return fmt.Sprintf(`project=%s AND (fixVersion="%s" OR cf[%s]="%s")`,
		c.project, fixVersion, cfID, fixVersion)
}

// SearchIssues queries JIRA for issues matching a fixVersion or Target Version.
// When a target version custom field is configured, issues matching either field
// are returned. It handles pagination automatically and respects rate limits.
func (c *Client) SearchIssues(ctx context.Context, fixVersion string) ([]Issue, error) {
	jql := c.buildSearchJQL(fixVersion)
	fields := "summary,status,priority,labels,fixVersions,assignee,issuetype,resolution,updated"

	var allIssues []Issue
	startAt := 0
	maxResults := 100

	for {
		params := url.Values{
			"jql":        {jql},
			"fields":     {fields},
			"startAt":    {fmt.Sprintf("%d", startAt)},
			"maxResults": {fmt.Sprintf("%d", maxResults)},
		}

		reqURL := fmt.Sprintf("%s/rest/api/2/search?%s", c.baseURL, params.Encode())
		body, err := c.doGetWithRetry(ctx, reqURL)
		if err != nil {
			return nil, fmt.Errorf("search issues: %w", err)
		}

		var resp searchResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, fmt.Errorf("decode search response: %w", err)
		}

		allIssues = append(allIssues, resp.Issues...)

		if startAt+len(resp.Issues) >= resp.Total {
			break
		}
		startAt += len(resp.Issues)
	}

	return allIssues, nil
}

// GetVersion fetches version metadata from JIRA for the given project and version name.
func (c *Client) GetVersion(ctx context.Context, versionName string) (*VersionField, error) {
	reqURL := fmt.Sprintf("%s/rest/api/2/project/%s/versions", c.baseURL, url.PathEscape(c.project))
	body, err := c.doGetWithRetry(ctx, reqURL)
	if err != nil {
		return nil, fmt.Errorf("get versions: %w", err)
	}

	var versions []VersionField
	if err := json.Unmarshal(body, &versions); err != nil {
		return nil, fmt.Errorf("decode versions: %w", err)
	}

	for _, v := range versions {
		if v.Name == versionName {
			return &v, nil
		}
	}
	return nil, fmt.Errorf("version %q not found in project %s", versionName, c.project)
}

// doGetWithRetry performs an HTTP GET with rate limiting and retry on 429 responses.
func (c *Client) doGetWithRetry(ctx context.Context, reqURL string) ([]byte, error) {
	const maxRetries = 3

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 || c.minDelay > 0 {
			delay := c.minDelay
			if attempt > 0 {
				// Exponential backoff: 2s, 4s, 8s
				delay = time.Duration(math.Pow(2, float64(attempt))) * time.Second
			}
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}

		body, err := c.doGet(ctx, reqURL)
		if err == nil {
			return body, nil
		}

		// Check if it's a rate limit error
		if isRateLimitError(err) && attempt < maxRetries {
			retryAfter := parseRetryAfter(err)
			if retryAfter > 0 {
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(retryAfter):
				}
			}
			continue
		}

		return nil, err
	}

	return nil, fmt.Errorf("max retries exceeded for %s", reqURL)
}

func (c *Client) doGet(ctx context.Context, reqURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode == http.StatusTooManyRequests {
		retryAfter := resp.Header.Get("Retry-After")
		return nil, &rateLimitError{
			statusCode: resp.StatusCode,
			retryAfter: retryAfter,
			body:       string(body[:min(len(body), 200)]),
		}
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("JIRA API returned %d: %s", resp.StatusCode, string(body[:min(len(body), 200)]))
	}

	return body, nil
}

// rateLimitError represents a 429 Too Many Requests response.
type rateLimitError struct {
	statusCode int
	retryAfter string
	body       string
}

func (e *rateLimitError) Error() string {
	return fmt.Sprintf("JIRA API returned %d: %s", e.statusCode, e.body)
}

func isRateLimitError(err error) bool {
	_, ok := err.(*rateLimitError)
	return ok
}

func parseRetryAfter(err error) time.Duration {
	rle, ok := err.(*rateLimitError)
	if !ok || rle.retryAfter == "" {
		return 0
	}
	seconds, err := strconv.Atoi(rle.retryAfter)
	if err != nil {
		return 0
	}
	return time.Duration(seconds) * time.Second
}

// FixVersionToS3App maps a JIRA fixVersion to an S3 application prefix.
// It handles two formats:
//   - Plain semver: "3.16.3" → "quay-v3-16" (defaults to "quay" product)
//   - Prefixed:     "omr-v2.0.10" → "omr-v2-0" (product parsed from prefix)
func FixVersionToS3App(fixVersion string) string {
	// Check for "{product}-v{version}" format (e.g. "omr-v2.0.10")
	if idx := strings.Index(fixVersion, "-v"); idx > 0 {
		product := fixVersion[:idx]
		version := fixVersion[idx+2:] // skip "-v"
		parts := strings.Split(version, ".")
		if len(parts) >= 2 {
			return fmt.Sprintf("%s-v%s-%s", product, parts[0], parts[1])
		}
		return ""
	}

	// Plain semver: "3.16.3" → "quay-v3-16"
	parts := strings.Split(fixVersion, ".")
	if len(parts) >= 2 {
		return fmt.Sprintf("quay-v%s-%s", parts[0], parts[1])
	}
	return ""
}
