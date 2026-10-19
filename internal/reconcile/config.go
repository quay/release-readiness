// Package reconcile matches the commits an FBC ships to the Jira issues they
// fix.
package reconcile

// component is an allowlisted ART build's public repository. Its commits
// land on redhat-{stream}, or, with a branchPrefix, on the one branch of
// that prefix holding the commit.
type component struct {
	repo, branchPrefix string
}

// allowlist holds the ART builds, by ART name, whose source commits are
// reconciled. Anything it does not list is uncertain.
var allowlist = map[string]component{
	"quay-quay":                               {repo: "https://github.com/quay/quay"},
	"quay-clair":                              {repo: "https://github.com/quay/clair", branchPrefix: "release-"},
	"quay-builder":                            {repo: "https://github.com/quay/quay-builder"},
	"quay-builder-qemu":                       {repo: "https://github.com/quay/quay-builder-qemu"},
	"quay-operator":                           {repo: "https://github.com/quay/quay-operator"},
	"quay-operator-bundle":                    {repo: "https://github.com/quay/quay-operator"},
	"quay-operator-fbc":                       {repo: "https://github.com/quay/quay-operator"},
	"quay-bridge-operator":                    {repo: "https://github.com/quay/quay-bridge-operator"},
	"quay-bridge-operator-bundle":             {repo: "https://github.com/quay/quay-bridge-operator"},
	"quay-bridge-operator-fbc":                {repo: "https://github.com/quay/quay-bridge-operator"},
	"container-security-operator":             {repo: "https://github.com/quay/container-security-operator"},
	"quay-container-security-operator-bundle": {repo: "https://github.com/quay/container-security-operator"},
	"container-security-operator-fbc":         {repo: "https://github.com/quay/container-security-operator"},
}
