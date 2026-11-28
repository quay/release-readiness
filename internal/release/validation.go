package release

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync/atomic"

	"github.com/quay/release-readiness/internal/model"
	"github.com/quay/release-readiness/internal/prow"
)

// PlanRegistry publishes immutable complete inventories to concurrent API reads.
type planSnapshot struct {
	plans    map[string]ValidationPlan
	revision string
}

type PlanRegistry struct {
	current atomic.Pointer[planSnapshot]
}

func NewPlanRegistry() *PlanRegistry {
	r := &PlanRegistry{}
	r.Update(nil, "")
	return r
}

func (r *PlanRegistry) Update(jobs []prow.JobDefinition, revision string) {
	snapshot := &planSnapshot{plans: map[string]ValidationPlan{}}
	if revision != "" {
		snapshot.revision = "periodics:" + revision
	}
	for _, j := range jobs {
		if j.Removed {
			continue
		}
		p := snapshot.plans[j.Application]
		p.Revision = snapshot.revision
		p.Jobs = append(p.Jobs, validationJobRow(j))
		snapshot.plans[j.Application] = p
	}
	r.current.Store(snapshot)
}

// validationConfiguration formats metadata from the periodic configuration.
// These labels never add requirements or infer a job's result. Unknown storage
// stays unspecified rather than being guessed from the cloud provider.
func validationConfiguration(job prow.JobDefinition) (string, string) {
	cloud := job.Cloud
	switch cloud {
	case "aws":
		cloud = "AWS"
	case "gcp":
		cloud = "GCP"
	case "azure", "azure4":
		cloud = "Azure"
	case "libvirt", "libvirt-s390x-vpn":
		cloud = "Libvirt"
	}
	storage := ""
	for _, token := range strings.Split(job.Target, "-") {
		switch token {
		case "s3":
			storage = "S3"
		case "odf":
			storage = "ODF"
		case "gcs":
			storage = "GCS"
		case "blob":
			storage = "Blob"
		}
	}
	// Upgrade targets name the source version (from-318); their periodic
	// name, rather than the target, identifies the configured storage.
	if storage == "" && strings.Contains(job.Name, "-s3-upgrade-") {
		storage = "S3"
	}
	return cloud, storage
}

func (r *PlanRegistry) Plan(app string) ValidationPlan {
	snapshot := r.current.Load()
	if p, ok := snapshot.plans[app]; ok {
		return p
	}
	// A complete inventory can positively establish that no jobs exist.
	return ValidationPlan{Revision: snapshot.revision}
}

// ValidationPlan comes from reviewed job configurations, never inferred from
// the runs that happened to execute. Applications share a minor-stream plan.
type ValidationPlan struct {
	Revision string                `json:"revision"`
	Jobs     []model.ValidationRow `json:"jobs"`
}

func LoadValidationPlans(path string) (map[string]ValidationPlan, error) {
	if path == "" {
		return nil, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var plans map[string]ValidationPlan
	if err := json.Unmarshal(b, &plans); err != nil {
		return nil, err
	}
	for app, plan := range plans {
		if app == "" || plan.Revision == "" || len(plan.Jobs) == 0 {
			return nil, fmt.Errorf("validation plan %q needs a revision and required jobs", app)
		}
		seen := map[string]bool{}
		for _, row := range plan.Jobs {
			if strings.TrimSpace(row.Job) == "" || seen[row.Job] {
				return nil, fmt.Errorf("validation plan %q has an empty or duplicate job %q", app, row.Job)
			}
			seen[row.Job] = true
		}
	}
	return plans, nil
}

// EvaluateValidation accepts exact selected-FBC evidence only. Equivalent
// bundles and unverified image identities remain available in CI drill-downs,
// but cannot satisfy a shipment gate. Unsupported identities remain missing.
func EvaluateValidation(plan ValidationPlan, candidate *model.ReleaseCandidate, runs []prow.Run, fresh bool) model.Validation {
	v := model.Validation{PlanKnown: plan.Revision != "", Revision: plan.Revision, SourceFresh: fresh, Total: len(plan.Jobs)}
	exact := map[string]map[string]bool{}
	selectedDigests := map[string]bool{}
	if candidate.FBCCandidate != nil {
		for _, op := range candidate.FBCCandidate.Operators {
			for _, member := range op.Members {
				if member.Digest != "" {
					selectedDigests[member.Digest] = true
				}
				if member.CI == nil {
					continue
				}
				for _, job := range member.CI.Exact {
					if exact[job.JobName] == nil {
						exact[job.JobName] = map[string]bool{}
					}
					for _, run := range job.Runs {
						if run.ProwURL != "" {
							exact[job.JobName][run.ProwURL] = true
						}
					}
				}
			}
		}
	}
	// Index candidate-matched runs once, rather than scanning every run for
	// every required matrix row.
	completedByJob, runningByJob := map[string]*prow.Run{}, map[string]*prow.Run{}
	observedByJob := map[string]*prow.Run{}
	for i := range runs {
		r := &runs[i]
		old := observedByJob[r.JobName]
		at := r.FetchedAt
		if r.StartedAt != nil {
			at = *r.StartedAt
		}
		if old == nil {
			observedByJob[r.JobName] = r
		} else {
			oldAt := old.FetchedAt
			if old.StartedAt != nil {
				oldAt = *old.StartedAt
			}
			if at.After(oldAt) {
				observedByJob[r.JobName] = r
			}
		}
		if !exact[r.JobName][r.ProwURL] {
			continue
		}
		if r.CompletedAt != nil && (r.State == "success" || r.State == "failure" || r.State == "error" || r.State == "aborted") {
			old := completedByJob[r.JobName]
			if old == nil || r.CompletedAt.After(*old.CompletedAt) || (r.CompletedAt.Equal(*old.CompletedAt) && (r.State != "success" || r.InstallFailed)) {
				completedByJob[r.JobName] = r
			}
		} else if r.State == "pending" || r.State == "running" || r.State == "triggered" {
			old := runningByJob[r.JobName]
			if old == nil || (r.StartedAt != nil && (old.StartedAt == nil || r.StartedAt.After(*old.StartedAt))) {
				runningByJob[r.JobName] = r
			}
		}
	}
	for _, spec := range plan.Jobs {
		row := spec
		row.State, row.RunURL = "missing", ""
		observed := observedByJob[spec.Job]
		row.Evidence = missingEvidence(observed, selectedDigests)
		if observed != nil {
			row.RunURL = observed.ProwURL
		}
		completed, running := completedByJob[spec.Job], runningByJob[spec.Job]
		if spec.Removed || spec.Unconfigured {
			completed, running = nil, nil
			row.Evidence = "This job is no longer configured."
		}
		// A newly started, exact-FBC rerun is the current validation state.
		// Do not let an older still-running execution hide a later result.
		if running != nil && (completed == nil || (running.StartedAt != nil && running.StartedAt.After(*completed.CompletedAt))) {
			row.State, row.RunURL = "running", running.ProwURL
			row.Evidence = "The selected-FBC run is still in progress."
		} else if completed != nil {
			row.RunURL = completed.ProwURL
			if completed.State == "success" && !completed.InstallFailed {
				row.State = "passed"
				row.Evidence = "Verified against the selected FBC."
			} else {
				row.State = "failed"
				row.Evidence = "The latest completed run against the selected FBC failed."
			}
		}
		if row.State != "missing" {
			row.CIResult, row.FBCMatch = row.State, "selected"
		} else if observed != nil && !spec.Removed && !spec.Unconfigured {
			populateObservedValidation(&row, observed, candidate, selectedDigests)
		}
		switch row.State {
		case "passed":
			v.Passed++
		case "failed":
			v.Failed++
		case "running":
			v.Running++
		default:
			v.Missing++
		}
		v.Rows = append(v.Rows, row)
	}
	v.Ready = v.PlanKnown && v.Total > 0 && v.Passed == v.Total && fresh && candidate.Verdict.State == "ready"
	return v
}

func missingEvidence(run *prow.Run, selected map[string]bool) string {
	if run == nil {
		return "No run has been ingested for this configured job."
	}
	if run.State == "running" || run.State == "pending" || run.State == "triggered" {
		return "A run is in progress; its candidate identity is not yet verified."
	}
	if run.ArtifactState == prow.ArtifactInvalid {
		return "The run's image evidence is invalid; it will be retried."
	}
	if run.ArtifactState == prow.ArtifactMissing || run.ArtifactState == prow.ArtifactNone {
		return "The run is recorded, but its image evidence is unavailable; retry is scheduled."
	}
	digest := ImageDigest(run.CatalogRef)
	if digest == "" {
		return "The run has evidence without an exact FBC identity; it cannot satisfy this gate."
	}
	if len(selected) == 0 {
		return "The run is recorded; selected FBC identity is not yet available for matching."
	}
	if !selected[digest] {
		return "The latest observed run tested a different FBC; no verified match was found."
	}
	return "The run names the selected FBC; candidate correlation is not yet complete."
}
