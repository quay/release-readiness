package reconcile

import "testing"

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
