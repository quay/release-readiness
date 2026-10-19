package reconcile

import "testing"

func TestStreamOf(t *testing.T) {
	for _, tc := range []struct {
		assembly, stream string
		ok               bool
	}{
		{"3.10.28", "3.10", true},
		{"5.0.0", "5.0", true},
		{"3.18", "", false},
		{"3.18.1-rc", "", false},
		{"x.y.z", "", false},
	} {
		if stream, ok := streamOf(tc.assembly); stream != tc.stream || ok != tc.ok {
			t.Errorf("streamOf(%q) = %q, %v; want %q, %v", tc.assembly, stream, ok, tc.stream, tc.ok)
		}
	}
}

func TestAllowlisted(t *testing.T) {
	for _, tc := range []struct {
		component, repo, commit, prefix, reason string
	}{
		{"quay-quay", quayRepo, sha(1), "", ""},
		{"quay-operator-fbc", "https://github.com/quay/quay-operator", sha(1), "", ""},
		{"quay-clair", "https://github.com/quay/clair", sha(1), "release-", ""},
		{"quay-quay", opRepo, sha(1), "", "source repo " + opRepo + " is not " + quayRepo},
		{"quay-quay", quayRepo, "abc123", "", `commit "abc123" is not a full SHA`},
		{"base-rhel9", quayRepo, sha(1), "", "not in the allowlist"},
	} {
		c, reason := allowlisted(Source{Name: tc.component, SourceRepo: tc.repo, Commitish: tc.commit})
		if c.branchPrefix != tc.prefix || reason != tc.reason {
			t.Errorf("allowlisted(%s, %s) = %q, %q; want %q, %q", tc.component, tc.repo, c.branchPrefix, reason, tc.prefix, tc.reason)
		}
	}
}
