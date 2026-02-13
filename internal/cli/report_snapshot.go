package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

type snapshotSpec struct {
	Application string `yaml:"application"`
	Components  []struct {
		Name           string `yaml:"name"`
		ContainerImage string `yaml:"containerImage"`
		Source         struct {
			Git struct {
				Revision string `yaml:"revision"`
			} `yaml:"git"`
		} `yaml:"source"`
	} `yaml:"components"`
}

type snapshotFile struct {
	Metadata struct {
		Name string `yaml:"name"`
	} `yaml:"metadata"`
	Spec snapshotSpec `yaml:"spec"`
}

type SnapshotReport struct {
	Server  string
	Version string
	File    string
}

var versionSuffix = regexp.MustCompile(`-v\d+-\d+$`)

// knownComponents is the set of dashboard component names.
var knownComponents = map[string]bool{
	"quay":                               true,
	"clair":                              true,
	"quay-operator":                      true,
	"quay-operator-bundle":               true,
	"quay-bridge-operator":               true,
	"quay-bridge-operator-bundle":        true,
	"container-security-operator":        true,
	"container-security-operator-bundle": true,
	"quay-builder":                       true,
	"quay-builder-qemu":                  true,
}

// MapSnapshotName converts a Konflux Snapshot component name to a dashboard name.
// It strips the -vX-Y version suffix, then strips the quay- app prefix if the
// resulting name matches a known component.
func MapSnapshotName(name string) string {
	// Strip -vX-Y suffix
	mapped := versionSuffix.ReplaceAllString(name, "")

	if knownComponents[mapped] {
		return mapped
	}

	// Try stripping quay- app prefix
	if strings.HasPrefix(mapped, "quay-") {
		without := strings.TrimPrefix(mapped, "quay-")
		if knownComponents[without] {
			return without
		}
	}

	return mapped
}

func ReportSnapshot(r SnapshotReport) error {
	data, err := os.ReadFile(r.File)
	if err != nil {
		return fmt.Errorf("read snapshot: %w", err)
	}

	var snap snapshotFile
	if err := yaml.Unmarshal(data, &snap); err != nil {
		return fmt.Errorf("parse snapshot YAML: %w", err)
	}

	snapshotName := snap.Metadata.Name

	for _, c := range snap.Spec.Components {
		dashName := MapSnapshotName(c.Name)

		// Extract digest from containerImage (after @)
		imageURL := c.ContainerImage
		imageDigest := ""
		if idx := strings.LastIndex(c.ContainerImage, "@"); idx >= 0 {
			imageURL = c.ContainerImage[:idx]
			imageDigest = c.ContainerImage[idx+1:]
		}

		body := map[string]string{
			"component":     dashName,
			"version":       r.Version,
			"git_sha":       c.Source.Git.Revision,
			"image_url":     imageURL,
			"image_digest":  imageDigest,
			"snapshot_name": snapshotName,
		}

		bodyData, err := json.Marshal(body)
		if err != nil {
			return err
		}

		resp, err := http.Post(r.Server+"/api/v1/builds", "application/json", bytes.NewReader(bodyData))
		if err != nil {
			return fmt.Errorf("POST build for %s: %w", dashName, err)
		}

		var result map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&result)
		resp.Body.Close()

		if resp.StatusCode != http.StatusCreated {
			fmt.Printf("WARNING: %s: server returned %d: %v\n", dashName, resp.StatusCode, result["error"])
			continue
		}

		fmt.Printf("Build created: %s id=%v\n", dashName, result["id"])
	}

	return nil
}
