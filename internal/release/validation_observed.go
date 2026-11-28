package release

import (
	"regexp"
	"time"

	"github.com/quay/release-readiness/internal/artbuild"
	"github.com/quay/release-readiness/internal/model"
	"github.com/quay/release-readiness/internal/prow"
)

var fbcBuildStamp = regexp.MustCompile(`-(\d{14})\.ocp`)

// Observed CI results are informational: never modify the selected-FBC state
// or gate counters when a run tested another catalog.
func populateObservedValidation(row *model.ValidationRow, run *prow.Run, candidate *model.ReleaseCandidate, selected map[string]bool) {
	if run.CompletedAt != nil {
		switch run.State {
		case "success":
			row.CIResult = "passed"
			if run.InstallFailed {
				row.CIResult = "failed"
			}
		case "failure", "error", "aborted":
			row.CIResult = "failed"
		}
	} else if run.State == "running" || run.State == "pending" || run.State == "triggered" {
		row.CIResult = "running"
	}
	row.FBCMatch = "unverified"
	digest := ImageDigest(run.CatalogRef)
	if run.ArtifactState != prow.ArtifactPresent || digest == "" {
		return
	}
	row.TestedFBC = &model.ValidationFBC{Digest: digest}
	if len(selected) == 0 || selected[digest] {
		return
	}
	row.FBCMatch = "different"
	if candidate.FBCStream == nil || candidate.FBCCandidate == nil {
		return
	}
	for _, op := range candidate.FBCStream.Operators {
		for _, tested := range op.Members {
			if tested.Digest != digest {
				continue
			}
			row.TestedFBC.NVR = tested.NVR
			operator, version, ocp, ok := artbuild.ParseFBC(tested.NVR)
			if !ok {
				return
			}
			for _, selectedOp := range candidate.FBCCandidate.Operators {
				for _, member := range selectedOp.Members {
					selectedOperator, selectedVersion, selectedOCP, valid := artbuild.ParseFBC(member.NVR)
					if !valid || operator != selectedOperator || version != selectedVersion || ocp != selectedOCP {
						continue
					}
					row.SelectedFBC = &model.ValidationFBC{Digest: member.Digest, NVR: member.NVR}
					if newerFBCBuild(tested.NVR, member.NVR) {
						row.FBCMatch = "newer"
						row.Evidence = "This run tested a newer ART FBC than the selected candidate. Its CI result does not validate the selected FBC."
					}
					return
				}
			}
			return
		}
	}
}

func newerFBCBuild(tested, selected string) bool {
	a, b := fbcBuildStamp.FindStringSubmatch(tested), fbcBuildStamp.FindStringSubmatch(selected)
	if a == nil || b == nil {
		return false
	}
	ta, ea := time.Parse("20060102150405", a[1])
	tb, eb := time.Parse("20060102150405", b[1])
	return ea == nil && eb == nil && ta.After(tb)
}
