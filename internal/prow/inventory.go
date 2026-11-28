package prow

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/quay/release-readiness/internal/syncstatus"
	"github.com/quay/release-readiness/internal/workpool"
	"sigs.k8s.io/yaml"
)

// JobDefinition is a required periodic configuration, independent of its runs.
// Removed entries stay in storage for audit but no longer count as
// configured requirements.
type JobDefinition struct {
	Name, Application, Target, Revision, SourceURL, OCP, Cloud string
	Removed                                                    bool
}

func (d JobDefinition) Job() Job {
	return Job{Name: d.Name, Application: d.Application, Prefix: "logs/" + d.Name + "/"}
}

type InventoryStore interface {
	JobInventory(context.Context) ([]JobDefinition, string, error)
	SaveJobInventory(context.Context, []JobDefinition, string) error
}

type InventoryClient struct {
	BaseURL, RawBaseURL, Token string
	HTTP                       *http.Client
}

func NewInventoryClient(token string) *InventoryClient {
	return &InventoryClient{BaseURL: "https://api.github.com", RawBaseURL: "https://raw.githubusercontent.com/openshift/release", Token: token, HTTP: &http.Client{Timeout: 30 * time.Second}}
}

func (c *InventoryClient) get(ctx context.Context, p string, raw bool) ([]byte, error) {
	address := strings.TrimRight(c.BaseURL, "/") + "/repos/openshift/release/" + p
	publicRaw := raw && c.RawBaseURL != ""
	if publicRaw {
		file, query, _ := strings.Cut(p, "?")
		values, err := url.ParseQuery(query)
		if err != nil {
			return nil, err
		}
		address = strings.TrimRight(c.RawBaseURL, "/") + "/" + values.Get("ref") + "/" + strings.TrimPrefix(file, "contents/")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if raw {
		req.Header.Set("Accept", "application/vnd.github.raw+json")
	}
	if c.Token != "" && !publicRaw {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("job inventory %s: HTTP %d", p, resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, (16<<20)+1))
	if len(b) > 16<<20 {
		return nil, fmt.Errorf("inventory response too large")
	}
	return b, err
}

var artifactTarget = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*$`)

var inventorySHA = regexp.MustCompile(`^[0-9a-f]{40}$`)
var periodicFile = regexp.MustCompile(`^quay-quay(?:-operator)?-redhat-\d+\.\d+-periodics\.yaml$`)
var releaseJob = regexp.MustCompile(`^periodic-ci-quay-quay(?:-operator)?-redhat-(\d+)\.(\d+)-`)

func (c *InventoryClient) Fetch(ctx context.Context, previous string) ([]JobDefinition, string, error) {
	b, err := c.get(ctx, "commits/main", false)
	if err != nil {
		return nil, "", err
	}
	var commit struct {
		SHA string `json:"sha"`
	}
	if err = json.Unmarshal(b, &commit); err != nil {
		return nil, "", err
	}
	if !inventorySHA.MatchString(commit.SHA) {
		return nil, "", fmt.Errorf("invalid inventory commit")
	}
	if commit.SHA == previous {
		return nil, previous, nil
	}
	var files []string
	for _, directory := range []string{"ci-operator/jobs/quay/quay", "ci-operator/jobs/quay/quay-operator"} {
		b, err = c.get(ctx, "contents/"+directory+"?ref="+commit.SHA, false)
		if err != nil {
			return nil, "", err
		}
		var entries []struct{ Name, Path, Type string }
		if err = json.Unmarshal(b, &entries); err != nil {
			return nil, "", err
		}
		if len(entries) == 0 || len(entries) >= 1000 {
			return nil, "", fmt.Errorf("empty or potentially truncated inventory directory %s", directory)
		}
		found := false
		for _, entry := range entries {
			if entry.Type != "file" || !periodicFile.MatchString(entry.Name) {
				continue
			}
			if entry.Path != directory+"/"+entry.Name {
				return nil, "", fmt.Errorf("invalid inventory path")
			}
			files = append(files, entry.Path)
			found = true
		}
		if !found {
			return nil, "", fmt.Errorf("no release periodics in %s", directory)
		}
	}
	var jobs []JobDefinition
	err = workpool.Run(ctx, files, 4, func(ctx context.Context, file string) ([]JobDefinition, error) {
		body, err := c.get(ctx, "contents/"+file+"?ref="+url.QueryEscape(commit.SHA), true)
		if err != nil {
			return nil, err
		}
		return parsePeriodics(body, file, commit.SHA)
	}, func(_ string, parsed []JobDefinition) error { jobs = append(jobs, parsed...); return nil })
	if err != nil {
		return nil, "", err
	}
	seen := map[string]bool{}
	for _, job := range jobs {
		if seen[job.Name] {
			return nil, "", fmt.Errorf("duplicate periodic %s", job.Name)
		}
		seen[job.Name] = true
	}
	if len(jobs) == 0 {
		return nil, "", fmt.Errorf("empty periodic inventory")
	}
	return jobs, commit.SHA, nil
}

func parsePeriodics(body []byte, file, revision string) ([]JobDefinition, error) {
	var manifest struct {
		Periodics []struct {
			Name   string            `json:"name"`
			Labels map[string]string `json:"labels"`
			Spec   struct {
				Containers []struct {
					Args []string `json:"args"`
				} `json:"containers"`
			} `json:"spec"`
		} `json:"periodics"`
	}
	if err := yaml.Unmarshal(body, &manifest); err != nil {
		return nil, fmt.Errorf("%s: %w", file, err)
	}
	if len(manifest.Periodics) == 0 {
		return nil, fmt.Errorf("no periodics in %s", file)
	}
	var jobs []JobDefinition
	prefix := "periodic-ci-" + strings.TrimSuffix(path.Base(file), "-periodics.yaml") + "-"
	for _, j := range manifest.Periodics {
		m := releaseJob.FindStringSubmatch(j.Name)
		if m == nil || !strings.HasPrefix(j.Name, prefix) || strings.ContainsAny(j.Name, "/\\ \t\n") {
			return nil, fmt.Errorf("invalid release periodic %q in %s", j.Name, file)
		}
		target := ""
		for _, container := range j.Spec.Containers {
			for _, arg := range container.Args {
				if value, ok := strings.CutPrefix(arg, "--target="); ok {
					if target != "" && target != value {
						return nil, fmt.Errorf("multiple targets for %s", j.Name)
					}
					target = value
				}
			}
		}
		if !artifactTarget.MatchString(target) {
			return nil, fmt.Errorf("missing or invalid target for %s", j.Name)
		}
		jobs = append(jobs, JobDefinition{Name: j.Name, Application: "quay-" + m[1] + "-" + m[2], Target: target, Revision: revision, SourceURL: "https://github.com/openshift/release/blob/" + revision + "/" + file, OCP: j.Labels["job-release"], Cloud: j.Labels["ci-operator.openshift.io/cloud"]})
	}
	return jobs, nil
}

type InventorySync struct {
	Client  *InventoryClient
	Store   InventoryStore
	Status  *syncstatus.Source
	Updated func([]JobDefinition, string)
}

func (s *InventorySync) SyncOnce(ctx context.Context) error {
	_, previous, err := s.Store.JobInventory(ctx)
	if err != nil {
		return err
	}
	jobs, revision, err := s.Client.Fetch(ctx, previous)
	if err != nil {
		return err
	}
	if jobs == nil {
		return nil
	}
	if err := s.Store.SaveJobInventory(ctx, jobs, revision); err != nil {
		return err
	}
	all, rev, err := s.Store.JobInventory(ctx)
	if err != nil {
		return err
	}
	if s.Updated != nil {
		s.Updated(all, rev)
	}
	return nil
}

func (s *InventorySync) Run(ctx context.Context, interval time.Duration) {
	for {
		pass, cancel := context.WithTimeout(ctx, 3*time.Minute)
		s.Status.Report(s.SyncOnce(pass))
		cancel()
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}
