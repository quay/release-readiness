package s3

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"path"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/quay/release-readiness/internal/ctrf"
	"github.com/quay/release-readiness/internal/konflux"
	"github.com/quay/release-readiness/internal/model"
)

// Config holds the settings needed to connect to an S3-compatible store.
type Config struct {
	Endpoint  string // custom endpoint URL (e.g. http://localhost:3900)
	Region    string // "garage" for GarageFS, "us-east-1" for real S3
	Bucket    string // "quay-release-readiness"
	AccessKey string
	SecretKey string
}

// Client wraps an S3 client scoped to a single bucket.
type Client struct {
	s3     *s3.Client
	bucket string
	logger *slog.Logger
}

// New creates an S3 Client from the given Config.
func New(ctx context.Context, cfg Config, logger *slog.Logger) (*Client, error) {
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(cfg.Region),
		awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, ""),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}

	var opts []func(*s3.Options)
	if cfg.Endpoint != "" {
		opts = append(opts, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
			o.UsePathStyle = true
		})
	}

	return &Client{
		s3:     s3.NewFromConfig(awsCfg, opts...),
		bucket: cfg.Bucket,
		logger: logger,
	}, nil
}

// ListApplications returns the top-level application prefixes in the bucket
// (e.g. "quay-v3-17", "quay-v3-16").
func (c *Client) ListApplications(ctx context.Context) ([]string, error) {
	out, err := c.s3.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket:    &c.bucket,
		Delimiter: aws.String("/"),
	})
	if err != nil {
		return nil, fmt.Errorf("list applications: %w", err)
	}

	apps := make([]string, 0, len(out.CommonPrefixes))
	for _, p := range out.CommonPrefixes {
		apps = append(apps, strings.TrimSuffix(*p.Prefix, "/"))
	}
	return apps, nil
}

// ListSnapshots lists snapshot subdirectory names under {application}/snapshots/
// and returns the S3 key for each snapshot.json file.
func (c *Client) ListSnapshots(ctx context.Context, application string) ([]string, error) {
	prefix := application + "/snapshots/"
	delimiter := "/"
	paginator := s3.NewListObjectsV2Paginator(c.s3, &s3.ListObjectsV2Input{
		Bucket:    &c.bucket,
		Prefix:    &prefix,
		Delimiter: &delimiter,
	})

	var keys []string
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list snapshots: %w", err)
		}
		for _, p := range page.CommonPrefixes {
			// Each prefix is {app}/snapshots/{snapshot-name}/
			// The snapshot.json is at {app}/snapshots/{snapshot-name}/snapshot.json
			keys = append(keys, *p.Prefix+"snapshot.json")
		}
	}
	return keys, nil
}

// GetSnapshot fetches a Snapshot spec JSON by its full S3 key,
// parses it, and converts to model.Snapshot. The snapshot name is
// derived from the S3 directory name.
func (c *Client) GetSnapshot(ctx context.Context, key string) (*model.Snapshot, error) {
	data, err := c.getObject(ctx, key)
	if err != nil {
		return nil, err
	}
	var spec konflux.SnapshotSpec
	if err := json.Unmarshal(data, &spec); err != nil {
		return nil, fmt.Errorf("decode snapshot %s: %w", key, err)
	}
	// Extract snapshot name from S3 key.
	// key is "{app}/snapshots/{snapshot-name}/snapshot.json"
	name := path.Base(path.Dir(key))
	snap := konflux.Convert(spec, name)
	return &snap, nil
}

// ListTestSuites discovers test suite subdirectories under snapshotDir
// by looking for keys matching {snapshotDir}{suite}/results/ctrf-report.json.
// Returns the suite directory names (e.g. "api-tests", "ui-tests").
func (c *Client) ListTestSuites(ctx context.Context, snapshotDir string) ([]string, error) {
	paginator := s3.NewListObjectsV2Paginator(c.s3, &s3.ListObjectsV2Input{
		Bucket: &c.bucket,
		Prefix: aws.String(snapshotDir),
	})

	suffix := "/results/ctrf-report.json"
	var suites []string
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list test suites: %w", err)
		}
		for _, obj := range page.Contents {
			key := *obj.Key
			// Match keys like {snapshotDir}{suite}/results/ctrf-report.json
			rel := strings.TrimPrefix(key, snapshotDir)
			if strings.HasSuffix(rel, suffix) {
				suite := strings.TrimSuffix(rel, suffix)
				if suite != "" && !strings.Contains(suite, "/") {
					suites = append(suites, suite)
				}
			}
		}
	}
	return suites, nil
}

// GetCTRFReport fetches and parses a single CTRF JSON report from S3.
func (c *Client) GetCTRFReport(ctx context.Context, key string) (*ctrf.Report, error) {
	data, err := c.getObject(ctx, key)
	if err != nil {
		return nil, err
	}

	var report ctrf.Report
	if err := json.Unmarshal(data, &report); err != nil {
		return nil, fmt.Errorf("decode ctrf report %s: %w", key, err)
	}
	return &report, nil
}

func (c *Client) getObject(ctx context.Context, key string) ([]byte, error) {
	out, err := c.s3.GetObject(ctx, &s3.GetObjectInput{
		Bucket: &c.bucket,
		Key:    &key,
	})
	if err != nil {
		return nil, fmt.Errorf("get %s: %w", key, err)
	}
	defer func() { _ = out.Body.Close() }()
	return io.ReadAll(out.Body)
}
