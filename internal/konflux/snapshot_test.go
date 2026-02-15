package konflux

import (
	"testing"
	"time"
)

func TestConvert_FieldMapping(t *testing.T) {
	cr := SnapshotCR{}
	cr.Metadata.Name = "snapshot-abc123"
	cr.Metadata.CreationTimestamp = time.Date(2026, 2, 14, 15, 30, 0, 0, time.UTC)
	cr.Spec.Application = "quay-v3-17"
	cr.Spec.Components = append(cr.Spec.Components, struct {
		Name           string `json:"name"`
		ContainerImage string `json:"containerImage"`
		Source         struct {
			Git struct {
				URL      string `json:"url"`
				Revision string `json:"revision"`
			} `json:"git"`
		} `json:"source"`
	}{
		Name:           "quay-server",
		ContainerImage: "quay.io/quay/quay:sha-abc123",
	})
	cr.Spec.Components[0].Source.Git.URL = "https://github.com/quay/quay"
	cr.Spec.Components[0].Source.Git.Revision = "abc123def456"

	cr.Metadata.Annotations = map[string]string{
		"build.appstudio.openshift.io/component":  "quay-server",
		"pac.test.appstudio.openshift.io/log-url": "https://console.example.com/run/123",
		"test.appstudio.openshift.io/status":      `[{"scenario":"e2e-tests","status":"Succeeded"}]`,
	}

	snap := Convert(cr)

	if snap.Application != "quay-v3-17" {
		t.Errorf("Application = %q, want %q", snap.Application, "quay-v3-17")
	}
	if snap.Snapshot != "snapshot-abc123" {
		t.Errorf("Snapshot = %q, want %q", snap.Snapshot, "snapshot-abc123")
	}
	if !snap.CreatedAt.Equal(cr.Metadata.CreationTimestamp) {
		t.Errorf("CreatedAt = %v, want %v", snap.CreatedAt, cr.Metadata.CreationTimestamp)
	}
	if len(snap.Components) != 1 {
		t.Fatalf("len(Components) = %d, want 1", len(snap.Components))
	}
	c := snap.Components[0]
	if c.Name != "quay-server" {
		t.Errorf("Component.Name = %q, want %q", c.Name, "quay-server")
	}
	if c.ContainerImage != "quay.io/quay/quay:sha-abc123" {
		t.Errorf("Component.ContainerImage = %q", c.ContainerImage)
	}
	if c.GitRevision != "abc123def456" {
		t.Errorf("Component.GitRevision = %q", c.GitRevision)
	}
	if c.GitURL != "https://github.com/quay/quay" {
		t.Errorf("Component.GitURL = %q", c.GitURL)
	}
}

func TestConvert_TriggerFromAnnotation(t *testing.T) {
	cr := SnapshotCR{}
	cr.Metadata.Annotations = map[string]string{
		"build.appstudio.openshift.io/component":  "quay-server",
		"pac.test.appstudio.openshift.io/log-url": "https://console.example.com/run/123",
	}
	cr.Spec.Components = append(cr.Spec.Components, struct {
		Name           string `json:"name"`
		ContainerImage string `json:"containerImage"`
		Source         struct {
			Git struct {
				URL      string `json:"url"`
				Revision string `json:"revision"`
			} `json:"git"`
		} `json:"source"`
	}{Name: "quay-server"})
	cr.Spec.Components[0].Source.Git.Revision = "sha-from-annotation"

	snap := Convert(cr)

	if snap.Trigger.Component != "quay-server" {
		t.Errorf("Trigger.Component = %q, want %q", snap.Trigger.Component, "quay-server")
	}
	if snap.Trigger.GitSHA != "sha-from-annotation" {
		t.Errorf("Trigger.GitSHA = %q, want %q", snap.Trigger.GitSHA, "sha-from-annotation")
	}
	if snap.Trigger.PipelineRun != "https://console.example.com/run/123" {
		t.Errorf("Trigger.PipelineRun = %q", snap.Trigger.PipelineRun)
	}
}

func TestConvert_TriggerFallbackToFirstComponent(t *testing.T) {
	cr := SnapshotCR{}
	cr.Spec.Components = append(cr.Spec.Components, struct {
		Name           string `json:"name"`
		ContainerImage string `json:"containerImage"`
		Source         struct {
			Git struct {
				URL      string `json:"url"`
				Revision string `json:"revision"`
			} `json:"git"`
		} `json:"source"`
	}{Name: "quay-builder"})
	cr.Spec.Components[0].Source.Git.Revision = "fallback-sha"

	snap := Convert(cr)

	if snap.Trigger.Component != "quay-builder" {
		t.Errorf("Trigger.Component = %q, want %q", snap.Trigger.Component, "quay-builder")
	}
	if snap.Trigger.GitSHA != "fallback-sha" {
		t.Errorf("Trigger.GitSHA = %q, want %q", snap.Trigger.GitSHA, "fallback-sha")
	}
}

func TestNormalizeStatus(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Succeeded", "passed"},
		{"SUCCEEDED", "passed"},
		{"passed", "passed"},
		{"Failed", "failed"},
		{"EnvironmentProvisionError", "failed"},
		{"InProgress", "pending"},
		{"Pending", "pending"},
		{"unknown", "unknown"},
	}

	for _, tt := range tests {
		got := normalizeStatus(tt.input)
		if got != tt.want {
			t.Errorf("normalizeStatus(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestConvert_ReadinessAllPassed(t *testing.T) {
	cr := SnapshotCR{}
	cr.Metadata.Annotations = map[string]string{
		"test.appstudio.openshift.io/status": `[{"scenario":"a","status":"Succeeded"},{"scenario":"b","status":"Succeeded"}]`,
	}

	snap := Convert(cr)
	if !snap.Readiness.TestsPassed {
		t.Error("Readiness.TestsPassed = false, want true when all tests pass")
	}
}

func TestConvert_ReadinessNotAllPassed(t *testing.T) {
	cr := SnapshotCR{}
	cr.Metadata.Annotations = map[string]string{
		"test.appstudio.openshift.io/status": `[{"scenario":"a","status":"Succeeded"},{"scenario":"b","status":"Failed"}]`,
	}

	snap := Convert(cr)
	if snap.Readiness.TestsPassed {
		t.Error("Readiness.TestsPassed = true, want false when a test fails")
	}
}

func TestConvert_ReadinessNoTests(t *testing.T) {
	cr := SnapshotCR{}
	snap := Convert(cr)
	if snap.Readiness.TestsPassed {
		t.Error("Readiness.TestsPassed = true, want false when no tests present")
	}
}
