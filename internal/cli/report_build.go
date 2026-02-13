package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

type BuildReport struct {
	Server       string
	Component    string
	Version      string
	GitSHA       string
	GitBranch    string
	ImageURL     string
	ImageDigest  string
	PipelineRun  string
	SnapshotName string
}

func ReportBuild(r BuildReport) error {
	body := map[string]string{
		"component":     r.Component,
		"version":       r.Version,
		"git_sha":       r.GitSHA,
		"git_branch":    r.GitBranch,
		"image_url":     r.ImageURL,
		"image_digest":  r.ImageDigest,
		"pipeline_run":  r.PipelineRun,
		"snapshot_name": r.SnapshotName,
	}

	data, err := json.Marshal(body)
	if err != nil {
		return err
	}

	resp, err := http.Post(r.Server+"/api/v1/builds", "application/json", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("POST builds: %w", err)
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("server returned %d: %v", resp.StatusCode, result["error"])
	}

	fmt.Printf("Build created: id=%v\n", result["id"])
	return nil
}
