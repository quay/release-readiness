package konflux

import (
	"encoding/json"
	"log"
	"strings"
	"time"

	"github.com/quay/release-readiness/internal/model"
)

// SnapshotCR represents a raw Konflux Snapshot custom resource as stored in S3.
type SnapshotCR struct {
	Metadata struct {
		Name              string            `json:"name"`
		CreationTimestamp time.Time         `json:"creationTimestamp"`
		Annotations       map[string]string `json:"annotations"`
		DeletionTimestamp *time.Time        `json:"deletionTimestamp"`
	} `json:"metadata"`
	Spec struct {
		Application string `json:"application"`
		Components  []struct {
			Name           string `json:"name"`
			ContainerImage string `json:"containerImage"`
			Source         struct {
				Git struct {
					URL      string `json:"url"`
					Revision string `json:"revision"`
				} `json:"git"`
			} `json:"source"`
		} `json:"components"`
	} `json:"spec"`
}

// TestStatus represents a single test scenario status from the Snapshot CR annotation.
type TestStatus struct {
	Scenario       string `json:"scenario"`
	Status         string `json:"status"`
	LastUpdateTime string `json:"lastUpdateTime"`
}

// Convert transforms a raw Konflux SnapshotCR into a model.Snapshot.
// TestResult.Summary fields are left nil; JUnit data is populated separately
// during ingestion.
func Convert(cr SnapshotCR) model.Snapshot {
	snap := model.Snapshot{
		Application: cr.Spec.Application,
		Snapshot:    cr.Metadata.Name,
		CreatedAt:   cr.Metadata.CreationTimestamp,
	}

	for _, c := range cr.Spec.Components {
		snap.Components = append(snap.Components, model.SnapshotComponent{
			Name:           c.Name,
			ContainerImage: c.ContainerImage,
			GitRevision:    c.Source.Git.Revision,
			GitURL:         c.Source.Git.URL,
		})
	}

	snap.Trigger = deriveTrigger(cr)
	snap.TestResults = parseTestResults(cr)

	allPassed := len(snap.TestResults) > 0
	for _, tr := range snap.TestResults {
		if tr.Status != "passed" {
			allPassed = false
			break
		}
	}
	snap.Readiness = model.Readiness{
		TestsPassed: allPassed,
	}

	return snap
}

func deriveTrigger(cr SnapshotCR) model.Trigger {
	annotations := cr.Metadata.Annotations
	t := model.Trigger{
		Component:   annotations["build.appstudio.openshift.io/component"],
		PipelineRun: annotations["pac.test.appstudio.openshift.io/log-url"],
	}

	for _, c := range cr.Spec.Components {
		if c.Name == t.Component {
			t.GitSHA = c.Source.Git.Revision
			break
		}
	}

	if t.Component == "" && len(cr.Spec.Components) > 0 {
		first := cr.Spec.Components[0]
		t.Component = first.Name
		t.GitSHA = first.Source.Git.Revision
	}

	return t
}

func parseTestResults(cr SnapshotCR) []model.TestResult {
	raw, ok := cr.Metadata.Annotations["test.appstudio.openshift.io/status"]
	if !ok || raw == "" {
		return nil
	}

	var statuses []TestStatus
	if err := json.Unmarshal([]byte(raw), &statuses); err != nil {
		log.Printf("warning: failed to parse test status annotation for %s: %v", cr.Metadata.Name, err)
		return nil
	}

	results := make([]model.TestResult, 0, len(statuses))
	for _, ts := range statuses {
		results = append(results, model.TestResult{
			Scenario: ts.Scenario,
			Status:   normalizeStatus(ts.Status),
		})
	}
	return results
}

// normalizeStatus maps Konflux test status strings to dashboard statuses.
func normalizeStatus(status string) string {
	s := strings.ToLower(status)
	switch {
	case strings.Contains(s, "succeeded") || strings.Contains(s, "passed"):
		return "passed"
	case strings.Contains(s, "fail") || strings.Contains(s, "error"):
		return "failed"
	case strings.Contains(s, "inprogress") || strings.Contains(s, "pending"):
		return "pending"
	default:
		return status
	}
}
