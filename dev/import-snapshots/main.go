package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/quay/release-readiness/internal/model"
)

// Konflux Snapshot CR structures (no k8s client-go dependency).
type snapshotList struct {
	Items []snapshotCR `json:"items"`
}

type snapshotCR struct {
	Metadata struct {
		Name              string            `json:"name"`
		CreationTimestamp  time.Time         `json:"creationTimestamp"`
		Annotations       map[string]string `json:"annotations"`
		DeletionTimestamp *time.Time         `json:"deletionTimestamp"`
	} `json:"metadata"`
	Spec struct {
		Application string `json:"application"`
		Components  []struct {
			Name           string `json:"name"`
			ContainerImage string `json:"containerImage"`
			Source         struct {
				Git struct {
					URL      string `json:"url"`
					Revision string `json:"revision"`
				} `json:"git"`
			} `json:"source"`
		} `json:"components"`
	} `json:"spec"`
}

type testStatus struct {
	Scenario       string `json:"scenario"`
	Status         string `json:"status"`
	LastUpdateTime string `json:"lastUpdateTime"`
}

const maxSnapshotsPerApp = 5

func main() {
	namespace := flag.String("namespace", "quay-eng-tenant", "Kubernetes namespace to fetch snapshots from")
	kubeconfig := flag.String("kubeconfig", "/tmp/kubeconfig", "Path to kubeconfig file")
	flag.Parse()

	snapshots, err := fetchSnapshots(*kubeconfig, *namespace)
	if err != nil {
		log.Fatalf("fetch snapshots: %v", err)
	}
	log.Printf("fetched %d snapshots from cluster", len(snapshots))

	// Group by application and keep latest N per app.
	grouped := groupByApp(snapshots)

	// Convert to model.Snapshot and collect for upload.
	type uploadEntry struct {
		app      string
		snapshot model.Snapshot
	}
	var uploads []uploadEntry
	for app, crs := range grouped {
		for _, cr := range crs {
			snap := convertSnapshot(cr)
			uploads = append(uploads, uploadEntry{app: app, snapshot: snap})
		}
	}

	if len(uploads) == 0 {
		log.Println("no snapshots to upload")
		return
	}

	// Connect to S3.
	env, err := loadEnv("dev/s3.env")
	if err != nil {
		log.Fatalf("load s3.env: %v", err)
	}

	ctx := context.Background()
	client, bucket, err := newS3Client(ctx, env)
	if err != nil {
		log.Fatalf("create s3 client: %v", err)
	}

	// Upload each snapshot.
	for _, u := range uploads {
		data, err := json.MarshalIndent(u.snapshot, "", "  ")
		if err != nil {
			log.Fatalf("marshal snapshot %s: %v", u.snapshot.Snapshot, err)
		}

		key := fmt.Sprintf("%s/snapshots/%s.json", u.app, u.snapshot.Snapshot)
		if err := putObject(ctx, client, bucket, key, data); err != nil {
			log.Fatalf("upload %s: %v", key, err)
		}
		fmt.Printf("uploaded: s3://%s/%s\n", bucket, key)
	}

	// Set latest.json per application to the most recent snapshot.
	for app, crs := range grouped {
		latest := convertSnapshot(crs[0]) // already sorted newest-first
		data, err := json.MarshalIndent(latest, "", "  ")
		if err != nil {
			log.Fatalf("marshal latest for %s: %v", app, err)
		}

		key := fmt.Sprintf("%s/latest.json", app)
		if err := putObject(ctx, client, bucket, key, data); err != nil {
			log.Fatalf("upload %s: %v", key, err)
		}
		fmt.Printf("uploaded: s3://%s/%s\n", bucket, key)
	}

	fmt.Printf("\nsummary: %d snapshots across %d applications\n", len(uploads), len(grouped))
}

func fetchSnapshots(kubeconfig, namespace string) ([]snapshotCR, error) {
	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfig, "get", "snapshots", "-n", namespace, "-o", "json")
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("kubectl get snapshots: %w", err)
	}

	var list snapshotList
	if err := json.Unmarshal(out, &list); err != nil {
		return nil, fmt.Errorf("parse snapshot list: %w", err)
	}

	// Filter out snapshots being deleted.
	var active []snapshotCR
	for _, s := range list.Items {
		if s.Metadata.DeletionTimestamp == nil {
			active = append(active, s)
		}
	}
	return active, nil
}

// groupByApp groups snapshots by application and keeps the latest N,
// sorted newest-first.
func groupByApp(snapshots []snapshotCR) map[string][]snapshotCR {
	grouped := make(map[string][]snapshotCR)
	for _, s := range snapshots {
		app := s.Spec.Application
		grouped[app] = append(grouped[app], s)
	}

	for app, crs := range grouped {
		sort.Slice(crs, func(i, j int) bool {
			return crs[i].Metadata.CreationTimestamp.After(crs[j].Metadata.CreationTimestamp)
		})
		if len(crs) > maxSnapshotsPerApp {
			crs = crs[:maxSnapshotsPerApp]
		}
		grouped[app] = crs
	}
	return grouped
}

func convertSnapshot(cr snapshotCR) model.Snapshot {
	snap := model.Snapshot{
		Application: cr.Spec.Application,
		Snapshot:    cr.Metadata.Name,
		CreatedAt:   cr.Metadata.CreationTimestamp,
	}

	// Components.
	for _, c := range cr.Spec.Components {
		snap.Components = append(snap.Components, model.SnapshotComponent{
			Name:           c.Name,
			ContainerImage: c.ContainerImage,
			GitRevision:    c.Source.Git.Revision,
			GitURL:         c.Source.Git.URL,
		})
	}

	// Trigger: use build annotation if available, otherwise derive from first component.
	snap.Trigger = deriveTrigger(cr)

	// Test results from annotation.
	snap.TestResults = parseTestResults(cr)

	// Readiness: all tests must have passed.
	allPassed := len(snap.TestResults) > 0
	for _, tr := range snap.TestResults {
		if tr.Status != "passed" {
			allPassed = false
			break
		}
	}
	snap.Readiness = model.Readiness{
		TestsPassed: allPassed,
	}

	return snap
}

func deriveTrigger(cr snapshotCR) model.Trigger {
	annotations := cr.Metadata.Annotations
	t := model.Trigger{
		Component:   annotations["build.appstudio.openshift.io/component"],
		PipelineRun: annotations["pac.test.appstudio.openshift.io/log-url"],
	}

	// Find the git SHA for the trigger component.
	for _, c := range cr.Spec.Components {
		if c.Name == t.Component {
			t.GitSHA = c.Source.Git.Revision
			break
		}
	}

	// Fallback to first component if annotation wasn't set.
	if t.Component == "" && len(cr.Spec.Components) > 0 {
		first := cr.Spec.Components[0]
		t.Component = first.Name
		t.GitSHA = first.Source.Git.Revision
	}

	return t
}

func parseTestResults(cr snapshotCR) []model.TestResult {
	raw, ok := cr.Metadata.Annotations["test.appstudio.openshift.io/status"]
	if !ok || raw == "" {
		return nil
	}

	var statuses []testStatus
	if err := json.Unmarshal([]byte(raw), &statuses); err != nil {
		log.Printf("warning: failed to parse test status annotation for %s: %v", cr.Metadata.Name, err)
		return nil
	}

	results := make([]model.TestResult, 0, len(statuses))
	for _, ts := range statuses {
		results = append(results, model.TestResult{
			Scenario: ts.Scenario,
			Status:   normalizeStatus(ts.Status),
		})
	}
	return results
}

// normalizeStatus maps Konflux test status strings to dashboard statuses.
func normalizeStatus(status string) string {
	s := strings.ToLower(status)
	switch {
	case strings.Contains(s, "succeeded") || strings.Contains(s, "passed"):
		return "passed"
	case strings.Contains(s, "fail") || strings.Contains(s, "error"):
		return "failed"
	case strings.Contains(s, "inprogress") || strings.Contains(s, "pending"):
		return "pending"
	default:
		return status
	}
}

func newS3Client(ctx context.Context, env map[string]string) (*s3.Client, string, error) {
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(env["S3_REGION"]),
		awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(env["AWS_ACCESS_KEY_ID"], env["AWS_SECRET_ACCESS_KEY"], ""),
		),
	)
	if err != nil {
		return nil, "", fmt.Errorf("load aws config: %w", err)
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(env["S3_ENDPOINT"])
		o.UsePathStyle = true
	})

	return client, env["S3_BUCKET"], nil
}

func putObject(ctx context.Context, client *s3.Client, bucket, key string, data []byte) error {
	_, err := client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      &bucket,
		Key:         &key,
		Body:        strings.NewReader(string(data)),
		ContentType: aws.String("application/json"),
	})
	return err
}

func loadEnv(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	env := make(map[string]string)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		env[k] = v
	}
	return env, scanner.Err()
}
