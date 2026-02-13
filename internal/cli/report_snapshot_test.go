package cli

import "testing"

func TestMapSnapshotName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"quay-quay-v3-16", "quay"},
		{"quay-clair-v3-16", "clair"},
		{"quay-operator-v3-16", "quay-operator"},
		{"quay-operator-bundle-v3-16", "quay-operator-bundle"},
		{"quay-bridge-operator-v3-16", "quay-bridge-operator"},
		{"quay-bridge-operator-bundle-v3-16", "quay-bridge-operator-bundle"},
		{"container-security-operator-v3-16", "container-security-operator"},
		{"container-security-operator-bundle-v3-16", "container-security-operator-bundle"},
		{"quay-builder-v3-16", "quay-builder"},
		{"quay-builder-qemu-v3-16", "quay-builder-qemu"},
	}

	for _, tt := range tests {
		got := MapSnapshotName(tt.input)
		if got != tt.want {
			t.Errorf("MapSnapshotName(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
