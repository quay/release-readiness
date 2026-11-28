package server

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/quay/release-readiness/internal/db"
	"github.com/quay/release-readiness/internal/model"
	"github.com/quay/release-readiness/internal/prow"
	"github.com/quay/release-readiness/internal/release"
)

type historyRun struct {
	Job         string               `json:"job"`
	BuildID     string               `json:"build_id"`
	DisplayName string               `json:"display_name,omitempty"`
	State       string               `json:"state"`
	StartedAt   string               `json:"started_at"`
	ProwURL     string               `json:"prow_url"`
	FBCMatch    string               `json:"fbc_match"`
	FBC         *model.ValidationFBC `json:"fbc,omitempty"`
}

type historyPage struct {
	Runs         []historyRun          `json:"runs"`
	Job          *model.ValidationRow  `json:"job,omitempty"`
	SelectedFBCs []model.ValidationFBC `json:"selected_fbcs"`
	NextCursor   string                `json:"next_cursor,omitempty"`
}

// handleCIHistory reads stored observations only. Unknown identities stay
// visible in the optional stream-wide scope and never establish readiness.
func (s *Server) handleCIHistory(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit := 50
	if q.Has("limit") {
		var err error
		limit, err = strconv.Atoi(q.Get("limit"))
		if err != nil || limit < 1 || limit > 100 {
			writeError(w, http.StatusBadRequest, fmt.Errorf("limit must be between 1 and 100"))
			return
		}
	}
	if scope := q.Get("scope"); scope != "" && scope != "selected" && scope != "all" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("scope must be selected or all"))
		return
	}
	var before [3]string
	if cursor := q.Get("cursor"); cursor != "" {
		if len(cursor) > 4096 {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid history cursor"))
			return
		}
		decoded, err := base64.RawURLEncoding.DecodeString(cursor)
		if err != nil || json.Unmarshal(decoded, &before) != nil || before[1] == "" || before[2] == "" {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid history cursor"))
			return
		}
	}
	rel, err := s.db.GetReleaseVersion(r.Context(), r.PathValue("version"))
	if err != nil {
		writeError(w, http.StatusNotFound, fmt.Errorf("release not found"))
		return
	}
	page := historyPage{Runs: []historyRun{}, SelectedFBCs: []model.ValidationFBC{}}
	if rel.KonfluxApplication == "" {
		writeJSON(w, http.StatusOK, page)
		return
	}
	candidate, err := s.selector.SelectedFBCs(r.Context(), rel)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	selected := map[string]string{}
	var digests []string
	if candidate != nil {
		for _, op := range candidate.Operators {
			for _, m := range op.Members {
				if m.Digest != "" {
					if _, seen := selected[m.Digest]; !seen {
						selected[m.Digest] = m.NVR
						digests = append(digests, m.Digest)
						page.SelectedFBCs = append(page.SelectedFBCs, model.ValidationFBC{Digest: m.Digest, NVR: m.NVR})
					}
				}
			}
		}
	}
	plan := s.ValidationPlans[rel.KonfluxApplication]
	if s.InventoryPlans != nil {
		plan = s.InventoryPlans.Plan(rel.KonfluxApplication)
	}
	names := map[string]string{}
	for _, job := range plan.Jobs {
		ocp := ""
		if job.OCP != "" {
			ocp = "OCP " + job.OCP
		}
		names[job.Job] = strings.Join(nonempty(job.DisplayName, strings.Join(nonempty(job.Cloud, job.Storage), " / "), ocp, job.Architecture), " · ")
		if job.Job == q.Get("job") {
			copy := job
			page.Job = &copy
		}
	}
	runs, err := s.db.CIHistory(r.Context(), db.CIHistoryFilter{Application: rel.KonfluxApplication, Job: q.Get("job"), SelectedDigests: digests, IncludeOther: q.Get("scope") == "all", Before: before, Limit: limit + 1})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if len(runs) > limit {
		last := runs[limit-1]
		cursor, _ := json.Marshal([3]string{last.StartedAt, last.Job, last.BuildID})
		page.NextCursor = base64.RawURLEncoding.EncodeToString(cursor)
		runs = runs[:limit]
	}
	for _, run := range runs {
		row := historyRun{Job: run.Job, BuildID: run.BuildID, DisplayName: names[run.Job], State: run.State, StartedAt: run.StartedAt, ProwURL: run.ProwURL, FBCMatch: "unverified"}
		if run.InstallFailed {
			row.State = "failure"
		}
		digest := release.ImageDigest(run.CatalogRef)
		if run.ArtifactState == prow.ArtifactPresent && strings.HasPrefix(digest, "sha256:") {
			row.FBCMatch = "different"
			row.FBC = &model.ValidationFBC{Digest: digest, NVR: run.FBCNVR}
			if nvr, ok := selected[digest]; ok {
				row.FBCMatch = "exact"
				row.FBC.NVR = nvr
			}
		}
		page.Runs = append(page.Runs, row)
	}
	writeJSON(w, http.StatusOK, page)
}

func nonempty(values ...string) []string {
	result := []string{}
	for _, value := range values {
		if value != "" {
			result = append(result, value)
		}
	}
	return result
}
