package clair

// Report is a Clair vulnerability report for a single container image.
type Report struct {
	Vulnerabilities        map[string]Vulnerability `json:"vulnerabilities"`
	PackageVulnerabilities map[string][]string      `json:"package_vulnerabilities"`
	Packages               map[string]Package       `json:"packages"`
}

// Vulnerability describes a single CVE found by Clair.
type Vulnerability struct {
	Name               string `json:"name"`
	NormalizedSeverity string `json:"normalized_severity"`
	Description        string `json:"description"`
	Links              string `json:"links"`
	FixedInVersion     string `json:"fixed_in_version"`
	Package            struct {
		Name string `json:"name"`
	} `json:"package"`
}

// Package describes a software package detected in the image.
type Package struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// ScanSummaryEntry is one element of the scans/summary.json array.
type ScanSummaryEntry struct {
	Component string `json:"component"`
	Status    string `json:"status"`
	Reports   int    `json:"reports"`
}
