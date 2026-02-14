package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/quay/build-dashboard/internal/model"
)

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"jira_base_url": s.jiraBaseURL,
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if err := s.db.Ping(); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unhealthy", "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "healthy"})
}

// --- Components ---

func (s *Server) handleListComponents(w http.ResponseWriter, r *http.Request) {
	components, err := s.db.ListComponents()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, components)
}

// --- Snapshots ---

func (s *Server) handleListSnapshots(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))
	if limit <= 0 {
		limit = 50
	}
	snapshots, err := s.db.ListSnapshots(q.Get("application"), limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, snapshots)
}

func (s *Server) handleGetSnapshot(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	snap, err := s.db.GetSnapshotByName(name)
	if err != nil {
		writeError(w, http.StatusNotFound, fmt.Errorf("snapshot not found"))
		return
	}
	writeJSON(w, http.StatusOK, snap)
}

// --- Applications ---

func (s *Server) handleListApplications(w http.ResponseWriter, r *http.Request) {
	summaries, err := s.db.LatestSnapshotPerApplication()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, summaries)
}

// --- Releases (version-centric) ---

func (s *Server) handleListReleases(w http.ResponseWriter, r *http.Request) {
	releases, err := s.db.ListAllReleaseVersions()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if releases == nil {
		releases = []model.ReleaseVersion{}
	}
	writeJSON(w, http.StatusOK, releases)
}

func (s *Server) handleGetRelease(w http.ResponseWriter, r *http.Request) {
	version := r.PathValue("version")
	release, err := s.db.GetReleaseVersion(version)
	if err != nil {
		writeError(w, http.StatusNotFound, fmt.Errorf("release %q not found", version))
		return
	}
	writeJSON(w, http.StatusOK, release)
}

func (s *Server) handleGetReleaseSnapshot(w http.ResponseWriter, r *http.Request) {
	version := r.PathValue("version")
	release, err := s.db.GetReleaseVersion(version)
	if err != nil {
		writeError(w, http.StatusNotFound, fmt.Errorf("release %q not found", version))
		return
	}

	if release.S3Application == "" {
		writeError(w, http.StatusNotFound, fmt.Errorf("no S3 application mapped for release %q", version))
		return
	}

	// Get the latest snapshot for this release's S3 application
	apps, err := s.db.LatestSnapshotPerApplication()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	for _, app := range apps {
		if app.Application == release.S3Application {
			if app.LatestSnapshot == nil {
				writeError(w, http.StatusNotFound, fmt.Errorf("no snapshots found for %s", release.S3Application))
				return
			}
			// Get full snapshot with components and test results
			snap, err := s.db.GetSnapshotByName(app.LatestSnapshot.Name)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err)
				return
			}
			writeJSON(w, http.StatusOK, snap)
			return
		}
	}

	writeError(w, http.StatusNotFound, fmt.Errorf("no snapshots found for application %s", release.S3Application))
}

func (s *Server) handleListReleaseIssues(w http.ResponseWriter, r *http.Request) {
	version := r.PathValue("version")
	q := r.URL.Query()
	issues, err := s.db.ListJiraIssues(version, q.Get("type"), q.Get("status"), q.Get("label"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if issues == nil {
		issues = []model.JiraIssueRecord{}
	}
	writeJSON(w, http.StatusOK, issues)
}

func (s *Server) handleGetReleaseIssueSummary(w http.ResponseWriter, r *http.Request) {
	version := r.PathValue("version")
	summary, err := s.db.GetIssueSummary(version)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

// ReadinessResponse represents the computed readiness signal for a release.
type ReadinessResponse struct {
	Signal  string `json:"signal"`  // "green", "yellow", "red"
	Message string `json:"message"` // human-readable reason
}

func (s *Server) handleGetReleaseReadiness(w http.ResponseWriter, r *http.Request) {
	version := r.PathValue("version")

	release, err := s.db.GetReleaseVersion(version)
	if err != nil {
		writeError(w, http.StatusNotFound, fmt.Errorf("release %q not found", version))
		return
	}

	// Already shipped — no further checks needed
	if release.Released {
		writeJSON(w, http.StatusOK, ReadinessResponse{
			Signal:  "green",
			Message: "Released",
		})
		return
	}

	issueSummary, _ := s.db.GetIssueSummary(version)

	// Check test status from latest snapshot
	testsPassed := false
	if release.S3Application != "" {
		apps, err := s.db.LatestSnapshotPerApplication()
		if err == nil {
			for _, app := range apps {
				if app.Application == release.S3Application && app.LatestSnapshot != nil {
					testsPassed = app.LatestSnapshot.TestsPassed
					break
				}
			}
		}
	}

	now := time.Now()
	signal := "green"
	message := "All checks passing"

	openIssues := issueSummary != nil && issueSummary.Open > 0

	// Check past due date first
	if release.DueDate != nil && now.After(*release.DueDate) {
		signal = "red"
		message = "Past due date"
	} else if !testsPassed && openIssues {
		signal = "red"
		message = "Tests failing and open issues remain"
	} else if !testsPassed {
		signal = "yellow"
		message = "Integration tests failing"
	} else if openIssues {
		signal = "yellow"
		message = "Open issues remain"
	} else if release.DueDate != nil {
		daysUntil := int(release.DueDate.Sub(now).Hours() / 24)
		if daysUntil <= 3 {
			signal = "yellow"
			message = fmt.Sprintf("Due date in %d days", daysUntil)
		}
	}

	writeJSON(w, http.StatusOK, ReadinessResponse{
		Signal:  signal,
		Message: message,
	})
}

// --- Helpers ---

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("json encode: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}
