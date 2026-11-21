package artbuild

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

var artGroupName = regexp.MustCompile(`^[a-z][a-z0-9-]*-[0-9]+\.[0-9]+$`)

// ParseGroups validates explicit ART inventory. Empty configuration is allowed
// so the other source collectors can continue; history reports waiting.
func ParseGroups(value string) ([]string, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	var groups []string
	for _, value := range strings.Split(value, ",") {
		group := strings.TrimSpace(value)
		if !artGroupName.MatchString(group) {
			return nil, fmt.Errorf("invalid ART group %q: expected product-X.Y", group)
		}
		groups = append(groups, group)
	}
	slices.Sort(groups)
	return slices.Compact(groups), nil
}
