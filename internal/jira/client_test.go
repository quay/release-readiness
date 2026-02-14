package jira

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSearchIssues(t *testing.T) {
	issues := []Issue{
		{
			Key: "PROJQUAY-100",
			Fields: IssueFields{
				Summary:  "Fix auth bug",
				Status:   StatusField{Name: "Closed"},
				Priority: PriorityField{Name: "Major"},
				Labels:   []string{"qe-approved"},
				FixVersions: []VersionField{{Name: "3.16.2"}},
				Assignee: &UserField{DisplayName: "Jane Doe"},
				IssueType: TypeField{Name: "Bug"},
				Resolution: &ResField{Name: "Done"},
				Updated: "2026-01-15T10:00:00.000+0000",
			},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/2/search" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.Error(w, "not found", 404)
			return
		}

		// Check auth header
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("unexpected auth: %s", r.Header.Get("Authorization"))
		}

		resp := searchResponse{
			Total:      1,
			MaxResults: 100,
			StartAt:    0,
			Issues:     issues,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := New(Config{
		BaseURL: srv.URL,
		Token:   "test-token",
		Project: "PROJQUAY",
	})
	client.minDelay = 0 // disable delay for tests

	result, err := client.SearchIssues(context.Background(), "3.16.2")
	if err != nil {
		t.Fatalf("SearchIssues: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("got %d issues, want 1", len(result))
	}
	if result[0].Key != "PROJQUAY-100" {
		t.Errorf("key: got %q, want PROJQUAY-100", result[0].Key)
	}
	if result[0].Fields.Status.Name != "Closed" {
		t.Errorf("status: got %q, want Closed", result[0].Fields.Status.Name)
	}
}

func TestGetVersion(t *testing.T) {
	versions := []VersionField{
		{Name: "3.16.1", Released: true},
		{Name: "3.16.2", Description: "z-stream", ReleaseDate: "2026-02-20", Released: false},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(versions)
	}))
	defer srv.Close()

	client := New(Config{
		BaseURL: srv.URL,
		Token:   "test-token",
		Project: "PROJQUAY",
	})
	client.minDelay = 0

	v, err := client.GetVersion(context.Background(), "3.16.2")
	if err != nil {
		t.Fatalf("GetVersion: %v", err)
	}
	if v.Name != "3.16.2" {
		t.Errorf("name: got %q, want 3.16.2", v.Name)
	}
	if v.Released {
		t.Error("released: got true, want false")
	}

	// Test version not found
	_, err = client.GetVersion(context.Background(), "99.99.99")
	if err == nil {
		t.Error("expected error for non-existent version")
	}
}

func TestSearchIssuesPagination(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		startAt := r.URL.Query().Get("startAt")

		var resp searchResponse
		if startAt == "0" {
			resp = searchResponse{
				Total:      3,
				MaxResults: 2,
				StartAt:    0,
				Issues: []Issue{
					{Key: "PROJ-1"},
					{Key: "PROJ-2"},
				},
			}
		} else {
			resp = searchResponse{
				Total:      3,
				MaxResults: 2,
				StartAt:    2,
				Issues: []Issue{
					{Key: "PROJ-3"},
				},
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := New(Config{BaseURL: srv.URL, Project: "PROJ"})
	client.minDelay = 0
	result, err := client.SearchIssues(context.Background(), "1.0")
	if err != nil {
		t.Fatalf("SearchIssues: %v", err)
	}
	if len(result) != 3 {
		t.Fatalf("got %d issues, want 3", len(result))
	}
	if callCount != 2 {
		t.Errorf("expected 2 API calls for pagination, got %d", callCount)
	}
}

func TestDiscoverActiveReleases(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		jql := r.URL.Query().Get("jql")
		if jql == "" {
			t.Error("expected JQL parameter")
		}

		// Simulate real JIRA tickets: no fixVersions set, version only in summary
		resp := searchResponse{
			Total:      3,
			MaxResults: 100,
			StartAt:    0,
			Issues: []Issue{
				{
					Key: "PROJQUAY-10276",
					Fields: IssueFields{
						Summary: "Release Quay v3.16.2",
						Status:  StatusField{Name: "In Progress"},
						DueDate: "2026-02-28",
						Components: []ComponentField{
							{Name: "-area/release"},
						},
					},
				},
				{
					Key: "PROJQUAY-10170",
					Fields: IssueFields{
						Summary: "Release Quay v3.17.0",
						Status:  StatusField{Name: "New"},
						DueDate: "2026-03-15",
						Components: []ComponentField{
							{Name: "-area/release"},
						},
					},
				},
				{
					Key: "PROJQUAY-10278",
					Fields: IssueFields{
						Summary: "Release OMR v2.0.10",
						Status:  StatusField{Name: "Testing"},
						DueDate: "2026-02-20",
						Components: []ComponentField{
							{Name: "-area/release"},
						},
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := New(Config{
		BaseURL: srv.URL,
		Token:   "test-token",
		Project: "PROJQUAY",
	})
	client.minDelay = 0

	releases, err := client.DiscoverActiveReleases(context.Background())
	if err != nil {
		t.Fatalf("DiscoverActiveReleases: %v", err)
	}
	if len(releases) != 3 {
		t.Fatalf("got %d releases, want 3", len(releases))
	}

	// Check Quay release
	if releases[0].FixVersion != "quay-v3.16.2" {
		t.Errorf("release[0].FixVersion: got %q, want quay-v3.16.2", releases[0].FixVersion)
	}
	if releases[0].ReleaseTicketKey != "PROJQUAY-10276" {
		t.Errorf("release[0].ReleaseTicketKey: got %q, want PROJQUAY-10276", releases[0].ReleaseTicketKey)
	}
	if releases[0].S3Application != "quay-v3-16" {
		t.Errorf("release[0].S3Application: got %q, want quay-v3-16", releases[0].S3Application)
	}
	if releases[0].DueDate == nil {
		t.Fatal("release[0].DueDate: got nil, want 2026-02-28")
	}
	if releases[0].DueDate.Format("2006-01-02") != "2026-02-28" {
		t.Errorf("release[0].DueDate: got %s, want 2026-02-28", releases[0].DueDate.Format("2006-01-02"))
	}

	// Check second Quay release
	if releases[1].FixVersion != "quay-v3.17.0" {
		t.Errorf("release[1].FixVersion: got %q, want quay-v3.17.0", releases[1].FixVersion)
	}
	if releases[1].S3Application != "quay-v3-17" {
		t.Errorf("release[1].S3Application: got %q, want quay-v3-17", releases[1].S3Application)
	}

	// Check OMR release
	if releases[2].FixVersion != "omr-v2.0.10" {
		t.Errorf("release[2].FixVersion: got %q, want omr-v2.0.10", releases[2].FixVersion)
	}
	if releases[2].S3Application != "omr-v2-0" {
		t.Errorf("release[2].S3Application: got %q, want omr-v2-0", releases[2].S3Application)
	}
}

func TestParseVersionFromSummary(t *testing.T) {
	tests := []struct {
		summary     string
		wantProduct string
		wantVersion string
		wantOK      bool
	}{
		{"Release Quay v3.16.2", "quay", "3.16.2", true},
		{"Release Quay v3.17.0", "quay", "3.17.0", true},
		{"Release OMR v2.0.10", "omr", "2.0.10", true},
		{"Release Quay v3.9.18", "quay", "3.9.18", true},
		{"⦗konflux⦘ Quay v3.15.3", "quay", "3.15.3", true},
		{"Release Quay v3.15.4", "quay", "3.15.4", true},
		{"Release Quay v3.12.14", "quay", "3.12.14", true},
		{"no version here", "", "", false},
	}

	for _, tc := range tests {
		product, version, ok := ParseVersionFromSummary(tc.summary)
		if ok != tc.wantOK {
			t.Errorf("ParseVersionFromSummary(%q): ok=%v, want %v", tc.summary, ok, tc.wantOK)
			continue
		}
		if !ok {
			continue
		}
		if product != tc.wantProduct {
			t.Errorf("ParseVersionFromSummary(%q): product=%q, want %q", tc.summary, product, tc.wantProduct)
		}
		if version != tc.wantVersion {
			t.Errorf("ParseVersionFromSummary(%q): version=%q, want %q", tc.summary, version, tc.wantVersion)
		}
	}
}

func TestFixVersionToS3App(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"3.16.3", "quay-v3-16"},
		{"3.17.0", "quay-v3-17"},
		{"3.16", "quay-v3-16"},
		{"4.0.1", "quay-v4-0"},
		{"omr-v2.0.10", "omr-v2-0"},
		{"omr-v1.5.3", "omr-v1-5"},
		{"invalid", ""},
	}

	for _, tc := range tests {
		got := FixVersionToS3App(tc.input)
		if got != tc.want {
			t.Errorf("FixVersionToS3App(%q): got %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestBuildSearchJQL(t *testing.T) {
	tests := []struct {
		name               string
		targetVersionField string
		fixVersion         string
		wantJQL            string
	}{
		{
			name:       "no target version field",
			fixVersion: "quay-v3.16.2",
			wantJQL:    `project=PROJQUAY AND fixVersion="quay-v3.16.2"`,
		},
		{
			name:               "with target version field",
			targetVersionField: "customfield_12319940",
			fixVersion:         "quay-v3.17.0",
			wantJQL:            `project=PROJQUAY AND (fixVersion="quay-v3.17.0" OR cf[12319940]="quay-v3.17.0")`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			client := New(Config{
				Project:            "PROJQUAY",
				TargetVersionField: tc.targetVersionField,
			})
			got := client.buildSearchJQL(tc.fixVersion)
			if got != tc.wantJQL {
				t.Errorf("buildSearchJQL(%q):\n got %q\nwant %q", tc.fixVersion, got, tc.wantJQL)
			}
		})
	}
}

func TestSearchIssuesWithTargetVersion(t *testing.T) {
	var capturedJQL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedJQL = r.URL.Query().Get("jql")
		resp := searchResponse{
			Total:      1,
			MaxResults: 100,
			StartAt:    0,
			Issues:     []Issue{{Key: "PROJQUAY-10157"}},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := New(Config{
		BaseURL:            srv.URL,
		Project:            "PROJQUAY",
		TargetVersionField: "customfield_12319940",
	})
	client.minDelay = 0

	result, err := client.SearchIssues(context.Background(), "quay-v3.17.0")
	if err != nil {
		t.Fatalf("SearchIssues: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("got %d issues, want 1", len(result))
	}

	wantJQL := `project=PROJQUAY AND (fixVersion="quay-v3.17.0" OR cf[12319940]="quay-v3.17.0")`
	if capturedJQL != wantJQL {
		t.Errorf("JQL:\n got %q\nwant %q", capturedJQL, wantJQL)
	}
}

func TestRateLimitRetry(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount <= 2 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte("rate limited"))
			return
		}
		resp := searchResponse{
			Total:      1,
			MaxResults: 100,
			StartAt:    0,
			Issues:     []Issue{{Key: "PROJ-1"}},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := New(Config{BaseURL: srv.URL, Project: "PROJ"})
	client.minDelay = 0

	result, err := client.SearchIssues(context.Background(), "1.0")
	if err != nil {
		t.Fatalf("SearchIssues after retries: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("got %d issues, want 1", len(result))
	}
	if callCount != 3 {
		t.Errorf("expected 3 calls (2 retries + 1 success), got %d", callCount)
	}
}
