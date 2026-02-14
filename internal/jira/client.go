// Package jira provides a client for querying JIRA Server and Cloud REST APIs.
package jira

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Config holds JIRA connection settings.
type Config struct {
	BaseURL  string // e.g. https://issues.redhat.com
	Token    string // Personal Access Token (Server) or API token (Cloud)
	Project  string // e.g. PROJQUAY
}

// Client is a JIRA REST API client.
type Client struct {
	baseURL    string
	token      string
	project    string
	httpClient *http.Client
}

// New creates a new JIRA client.
func New(cfg Config) *Client {
	return &Client{
		baseURL: strings.TrimRight(cfg.BaseURL, "/"),
		token:   cfg.Token,
		project: cfg.Project,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
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

type searchResponse struct {
	Total      int     `json:"total"`
	MaxResults int     `json:"maxResults"`
	StartAt    int     `json:"startAt"`
	Issues     []Issue `json:"issues"`
}

// BaseURL returns the configured JIRA base URL.
func (c *Client) BaseURL() string {
	return c.baseURL
}

// SearchIssues queries JIRA for issues matching a fixVersion.
// It handles pagination automatically.
func (c *Client) SearchIssues(ctx context.Context, fixVersion string) ([]Issue, error) {
	jql := fmt.Sprintf(`project=%s AND fixVersion="%s"`, c.project, fixVersion)
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
		body, err := c.doGet(ctx, reqURL)
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
	body, err := c.doGet(ctx, reqURL)
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

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("JIRA API returned %d: %s", resp.StatusCode, string(body[:min(len(body), 200)]))
	}

	return body, nil
}
