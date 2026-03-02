package konflux

import (
	"github.com/quay/release-readiness/internal/model"
)

// SnapshotSpec is the Konflux Snapshot spec as stored in S3.
// This is the spec section of the Snapshot CR, not the full Kubernetes resource.
type SnapshotSpec struct {
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
}

// Convert transforms a SnapshotSpec into a model.Snapshot.
// The name parameter is the snapshot directory name from S3 (since
// the spec does not include the snapshot name).
func Convert(spec SnapshotSpec, name string) model.Snapshot {
	snap := model.Snapshot{
		Application: spec.Application,
		Snapshot:    name,
	}

	for _, c := range spec.Components {
		snap.Components = append(snap.Components, model.SnapshotComponent{
			Name:           c.Name,
			ContainerImage: c.ContainerImage,
			GitRevision:    c.Source.Git.Revision,
			GitURL:         c.Source.Git.URL,
		})
	}

	return snap
}
