package release

import (
	"regexp"
	"strings"

	"github.com/quay/release-readiness/internal/model"
	"github.com/quay/release-readiness/internal/prow"
)

var periodicScenario = regexp.MustCompile(`^periodic-ci-quay-quay(?:-operator)?-redhat-(\d+\.\d+)-(.+)$`)
var scenarioOCP = regexp.MustCompile(`(?:^|-)ocp(\d)(\d+)(?:-|$)`)
var upgradeSource = regexp.MustCompile(`^from-(\d)(\d+)$`)

// Labels describe the configured test, never inferred runtime capabilities.
// Keep unrecognized targets visible rather than inventing a scenario for them.
func validationJobRow(job prow.JobDefinition) model.ValidationRow {
	cloud, storage := validationConfiguration(job)
	row := model.ValidationRow{Job: job.Name, OCP: job.OCP, Cloud: cloud, Storage: storage, SourceURL: job.SourceURL, DisplayName: job.Name}
	match := periodicScenario.FindStringSubmatch(job.Name)
	if match == nil {
		return row
	}
	stream, scenario := match[1], match[2]
	if row.OCP == "" {
		if ocp := scenarioOCP.FindStringSubmatch(scenario); ocp != nil {
			row.OCP = ocp[1] + "." + ocp[2]
		}
	}
	var title string
	switch {
	case strings.Contains(scenario, "-e2e-chainsaw-"):
		title, row.Mode = "Operator tests · Chainsaw", "Operator"
	case strings.Contains(scenario, "-standalone-ha-"):
		title, row.Mode = "Standalone · High availability", "Standalone"
	case strings.Contains(scenario, "-s3-upgrade-"):
		title, row.Mode = "Upgrade", "Operator"
		if from := upgradeSource.FindStringSubmatch(job.Target); from != nil {
			title += " " + from[1] + "." + from[2] + " → " + stream
		} else {
			title += " · " + job.Target
		}
	case strings.Contains(scenario, "-e2e-legacy-"):
		title, row.Mode = "Legacy installation", "Operator"
	case strings.Contains(scenario, "-e2e-install-"):
		title, row.Mode = "Install", "Operator"
	default:
		// Preserve the unfamiliar scenario, dropping only the repeated prefix.
		row.DisplayName = scenario
		return row
	}
	var variants []string
	seen := map[string]bool{}
	add := func(label string) {
		if !seen[label] {
			variants = append(variants, label)
			seen[label] = true
		}
	}
	for _, token := range strings.Split(scenario, "-") {
		switch token {
		case "arm64":
			row.Architecture = "arm64"
			add("ARM64")
		case "s390x":
			row.Architecture = "s390x"
			add("s390x")
		case "amd64":
			row.Architecture = "amd64"
			add("AMD64")
		case "metal":
			add("Bare metal")
		case "fips":
			add("FIPS")
		case "dualstack":
			add("Dual stack")
		case "rds":
			add("RDS")
		case "postgres":
			add("PostgreSQL")
		}
	}
	if !strings.HasPrefix(job.Target, "from-") {
		for _, token := range strings.Split(job.Target, "-") {
			switch token {
			case "aws", "azure", "gcp", "libvirt", "s3", "odf", "gcs", "blob", "nightly", "e2e", "arm64", "s390x", "amd64", "metal", "fips", "dualstack", "rds", "postgres":
			default:
				if token != "" {
					add(token)
				}
			}
		}
	}
	row.DisplayName = strings.Join(append([]string{title}, variants...), " · ")
	return row
}
