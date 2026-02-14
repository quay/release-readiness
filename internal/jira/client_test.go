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
