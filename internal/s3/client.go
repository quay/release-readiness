package s3

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/quay/release-readiness/internal/junit"
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
}

// New creates an S3 Client from the given Config.
func New(ctx context.Context, cfg Config) (*Client, error) {
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

// GetLatestSnapshot fetches {application}/latest.json and decodes it.
func (c *Client) GetLatestSnapshot(ctx context.Context, application string) (*model.Snapshot, error) {
	return c.getSnapshot(ctx, application+"/latest.json")
}

// ListSnapshots lists all .json keys under {application}/snapshots/.
func (c *Client) ListSnapshots(ctx context.Context, application string) ([]string, error) {
	prefix := application + "/snapshots/"
	paginator := s3.NewListObjectsV2Paginator(c.s3, &s3.ListObjectsV2Input{
		Bucket: &c.bucket,
		Prefix: &prefix,
	})

	var keys []string
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list snapshots: %w", err)
		}
		for _, obj := range page.Contents {
			if strings.HasSuffix(*obj.Key, ".json") {
				keys = append(keys, *obj.Key)
			}
		}
	}
	return keys, nil
}

// GetSnapshot fetches a specific snapshot JSON by its full S3 key.
func (c *Client) GetSnapshot(ctx context.Context, key string) (*model.Snapshot, error) {
	return c.getSnapshot(ctx, key)
}

// GetTestResults fetches all JUnit XML files under the given prefix,
// parses each, and returns a merged result.
func (c *Client) GetTestResults(ctx context.Context, junitPath string) (*junit.Result, error) {
	prefix := junitPath
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	paginator := s3.NewListObjectsV2Paginator(c.s3, &s3.ListObjectsV2Input{
		Bucket: &c.bucket,
		Prefix: &prefix,
	})

	var results []*junit.Result
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list junit files: %w", err)
		}
		for _, obj := range page.Contents {
			if !strings.HasSuffix(*obj.Key, ".xml") {
				continue
			}
			data, err := c.getObject(ctx, *obj.Key)
			if err != nil {
				log.Printf("warning: skipping junit file %s: %v", *obj.Key, err)
				continue
			}
			r, err := junit.Parse(data)
			if err != nil {
				log.Printf("warning: skipping junit file %s: %v", *obj.Key, err)
				continue
			}
			results = append(results, r)
		}
	}

	if len(results) == 0 {
		return nil, fmt.Errorf("no junit xml files found under %s", junitPath)
	}
	return junit.MergeResults(results...), nil
}

func (c *Client) getSnapshot(ctx context.Context, key string) (*model.Snapshot, error) {
	data, err := c.getObject(ctx, key)
	if err != nil {
		return nil, err
	}
	var snap model.Snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return nil, fmt.Errorf("decode snapshot %s: %w", key, err)
	}
	return &snap, nil
}

func (c *Client) getObject(ctx context.Context, key string) ([]byte, error) {
	out, err := c.s3.GetObject(ctx, &s3.GetObjectInput{
		Bucket: &c.bucket,
		Key:    &key,
	})
	if err != nil {
		return nil, fmt.Errorf("get %s: %w", key, err)
	}
	defer out.Body.Close()
	return io.ReadAll(out.Body)
}
