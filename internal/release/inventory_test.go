package release

import (
	"github.com/quay/release-readiness/internal/model"
	"github.com/quay/release-readiness/internal/prow"
	"testing"
)

func TestInventoryOnlyCurrentPeriodicsAndKnownEmpty(t *testing.T) {
	r := NewPlanRegistry()
	c := &model.ReleaseCandidate{Verdict: model.Verdict{State: "ready"}}
	if EvaluateValidation(r.Plan("quay-3-18"), c, nil, true).PlanKnown {
		t.Fatal("unloaded inventory looks known")
	}
	r.Update([]prow.JobDefinition{{Name: "new", Application: "quay-3-18"}, {Name: "old", Application: "quay-3-17", Removed: true}}, "rev")
	v := EvaluateValidation(r.Plan("quay-3-18"), c, nil, true)
	if !v.PlanKnown || v.Total != 1 || v.Missing != 1 || v.Ready {
		t.Fatalf("configured job without runs: %+v", v)
	}
	for _, app := range []string{"quay-3-17", "omr-2-0", "quay-99-0"} {
		v = EvaluateValidation(r.Plan(app), c, nil, true)
		if !v.PlanKnown || v.Total != 0 || v.Ready {
			t.Fatalf("no CI configured: %+v", v)
		}
	}
}

func TestInventoryConfigurationLabels(t *testing.T) {
	for _, tc := range []struct{ cloud, target, name, wantCloud, wantStorage string }{
		{"aws", "aws-s3-nightly", "install", "AWS", "S3"},
		{"aws", "aws-metal-s3-nightly", "install", "AWS", "S3"},
		{"aws", "aws-odf-nightly", "install", "AWS", "ODF"},
		{"aws", "aws-s3-rds-nightly", "install", "AWS", "S3"},
		{"gcp", "gcp-gcs-nightly", "install", "GCP", "GCS"},
		{"azure4", "azure-blob-postgres-nightly", "install", "Azure", "Blob"},
		{"libvirt-s390x-vpn", "libvirt-s3-nightly", "install", "Libvirt", "S3"},
		{"aws", "from-318", "periodic-ci-quay-quay-redhat-5.0-aws-ocp50-s3-upgrade-from-318", "AWS", "S3"},
		{"aws", "e2e", "chainsaw", "AWS", ""},
		{"custom", "custom-newstorage", "install", "custom", ""},
	} {
		t.Run(tc.target, func(t *testing.T) {
			r := NewPlanRegistry()
			r.Update([]prow.JobDefinition{{Name: tc.name, Application: "quay-5-0", Cloud: tc.cloud, Target: tc.target}}, "rev")
			row := r.Plan("quay-5-0").Jobs[0]
			if row.Cloud != tc.wantCloud || row.Storage != tc.wantStorage {
				t.Fatalf("configuration = %q / %q, want %q / %q", row.Cloud, row.Storage, tc.wantCloud, tc.wantStorage)
			}
		})
	}
}

func TestInventoryScenarioNames(t *testing.T) {
	for _, tc := range []struct{ scenario, target, name, ocp, arch, mode string }{
		{"aws-ocp50-e2e-install-aws-s3-nightly", "aws-s3-nightly", "Install", "5.0", "", "Operator"},
		{"aws-arm64-ocp50-e2e-install-aws-s3-nightly", "aws-s3-nightly", "Install · ARM64", "5.0", "arm64", "Operator"},
		{"aws-metal-ocp50-e2e-install-aws-metal-s3-nightly", "aws-metal-s3-nightly", "Install · Bare metal", "5.0", "", "Operator"},
		{"aws-ocp50-e2e-install-aws-s3-nightly-fips", "aws-s3-nightly-fips", "Install · FIPS", "5.0", "", "Operator"},
		{"aws-ocp50-e2e-install-aws-s3-rds-nightly", "aws-s3-rds-nightly", "Install · RDS", "5.0", "", "Operator"},
		{"azure-ocp50-e2e-install-azure-blob-postgres-nightly", "azure-blob-postgres-nightly", "Install · PostgreSQL", "5.0", "", "Operator"},
		{"aws-ocp50-s3-upgrade-from-318", "from-318", "Upgrade 3.18 → 5.0", "5.0", "", "Operator"},
		{"aws-ocp50-e2e-chainsaw-aws-odf-nightly", "aws-odf-nightly", "Operator tests · Chainsaw", "5.0", "", "Operator"},
		{"aws-standalone-ha-e2e", "e2e", "Standalone · High availability", "", "", "Standalone"},
		{"libvirt-s390x-ocp422-e2e-install-libvirt-s3-nightly", "libvirt-s3-nightly", "Install · s390x", "4.22", "s390x", "Operator"},
		{"aws-ocp50-e2e-install-aws-s3-nightly-dualstack", "aws-s3-nightly-dualstack", "Install · Dual stack", "5.0", "", "Operator"},
		{"aws-ocp419-e2e-legacy-aws-s3", "aws-s3", "Legacy installation", "4.19", "", "Operator"},
		{"aws-ocp50-e2e-install-aws-s3-newfeature", "aws-s3-newfeature", "Install · newfeature", "5.0", "", "Operator"},
		{"aws-new-scenario", "new-scenario", "aws-new-scenario", "", "", ""},
	} {
		t.Run(tc.scenario, func(t *testing.T) {
			row := validationJobRow(prow.JobDefinition{Name: "periodic-ci-quay-quay-redhat-5.0-" + tc.scenario, Target: tc.target})
			if row.DisplayName != tc.name || row.OCP != tc.ocp || row.Architecture != tc.arch || row.Mode != tc.mode {
				t.Fatalf("unexpected scenario metadata: %+v", row)
			}
		})
	}
	row := validationJobRow(prow.JobDefinition{Name: "future-job", Target: "test", OCP: "4.22"})
	if row.DisplayName != "future-job" || row.OCP != "4.22" {
		t.Fatalf("unknown job lost identity: %+v", row)
	}
}
