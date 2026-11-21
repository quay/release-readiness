package release

import (
	"testing"
	"time"

	"github.com/quay/release-readiness/internal/model"
	"github.com/quay/release-readiness/internal/prow"
)

func TestObservedResultsDoNotPassSelectedFBCGate(t *testing.T) {
	at := time.Now().UTC()
	for _, tc := range []struct {
		name, state, artifact, testedNVR, wantResult, wantMatch string
		completed                                               bool
	}{
		{"newer success", "success", prow.ArtifactPresent, "quay-operator-fbc-5.0.0-20261113043804.ocp5.0", "passed", "newer", true},
		{"newer failure", "failure", prow.ArtifactPresent, "quay-operator-fbc-5.0.0-20261113043804.ocp5.0", "failed", "newer", true},
		{"newer running", "pending", prow.ArtifactPresent, "quay-operator-fbc-5.0.0-20261113043804.ocp5.0", "running", "newer", false},
		{"older catalog", "success", prow.ArtifactPresent, "quay-operator-fbc-5.0.0-20261101043804.ocp5.0", "passed", "different", true},
		{"different slot", "success", prow.ArtifactPresent, "quay-operator-fbc-5.0.0-20261113043804.ocp4.22", "passed", "different", true},
		{"different operator", "success", prow.ArtifactPresent, "quay-bridge-operator-fbc-5.0.0-20261113043804.ocp5.0", "passed", "different", true},
		{"unknown date", "success", prow.ArtifactPresent, "quay-operator-fbc-5.0.0-20269999043804.ocp5.0", "passed", "different", true},
		{"invalid artifact", "success", prow.ArtifactInvalid, "quay-operator-fbc-5.0.0-20261113043804.ocp5.0", "passed", "unverified", true},
		{"no completion", "success", prow.ArtifactPresent, "quay-operator-fbc-5.0.0-20261113043804.ocp5.0", "", "newer", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			selected := model.FBCMember{Digest: "sha256:selected", NVR: "quay-operator-fbc-5.0.0-20261105171721.ocp5.0"}
			tested := model.FBCMember{Digest: "sha256:tested", NVR: tc.testedNVR}
			candidate := &model.ReleaseCandidate{Verdict: model.Verdict{State: "ready"}, FBCCandidate: &model.FBCCandidate{Operators: []model.OperatorFBC{{Members: []model.FBCMember{selected}}}}, FBCStream: &model.FBCCandidate{Operators: []model.OperatorFBC{{Members: []model.FBCMember{tested}}}}}
			run := prow.Run{JobName: "arm64", State: tc.state, StartedAt: &at, CatalogRef: "quay.io/catalog@sha256:tested", ArtifactState: tc.artifact, ProwURL: "https://prow.example/run"}
			if tc.completed {
				run.CompletedAt = &at
			}
			v := EvaluateValidation(ValidationPlan{Revision: "rev", Jobs: []model.ValidationRow{{Job: "arm64"}}}, candidate, []prow.Run{run}, true)
			r := v.Rows[0]
			if r.State != "missing" || v.Missing != 1 || v.Passed != 0 || v.Running != 0 || v.Failed != 0 || v.Ready {
				t.Fatalf("observed result changed gate: %+v", v)
			}
			if r.CIResult != tc.wantResult || r.FBCMatch != tc.wantMatch || r.RunURL != run.ProwURL {
				t.Fatalf("wrong observed result: %+v", r)
			}
			if tc.wantMatch == "newer" && (r.SelectedFBC == nil || r.TestedFBC == nil || r.SelectedFBC.Digest != selected.Digest || r.TestedFBC.NVR != tested.NVR) {
				t.Fatalf("comparison identities missing: %+v", r)
			}
		})
	}
}
