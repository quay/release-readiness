// Package catalog reads which image tags the public Red Hat container catalog
// has published, the signal that a release version has shipped.
package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/quay/release-readiness/internal/syncstatus"
)

// Repositories lists the registry.access.redhat.com repositories that publish
// each product. Quay 3.9-3.15 publish to quay-rhel8.
var Repositories = map[string][]string{
	"quay": {"quay/quay-rhel9", "quay/quay-rhel8"},
	"omr":  {"openshift/mirror-registry-rhel8"},
}

// Client reads the catalog API.
type Client struct {
	baseURL string
	http    *http.Client
}

func NewClient(baseURL string, hc *http.Client) *Client {
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), http: hc}
}

type imagesPage struct {
	Total int `json:"total"`
	Data  []struct {
		Repositories []struct {
			Published bool      `json:"published"`
			PushDate  time.Time `json:"push_date"`
			Tags      []struct {
				Name string `json:"name"`
			} `json:"tags"`
		} `json:"repositories"`
	} `json:"data"`
}

// PublishedTags returns every tag of a published image in repository, with
// the earliest push date of the images carrying it.
func (c *Client) PublishedTags(ctx context.Context, repository string) (map[string]time.Time, error) {
	tags := make(map[string]time.Time)
	for page, seen := 0, 0; ; page++ {
		q := url.Values{
			"page_size": {"500"},
			"page":      {strconv.Itoa(page)},
			"include":   {"total,data.repositories.published,data.repositories.push_date,data.repositories.tags.name"},
		}
		path := "/repositories/registry/registry.access.redhat.com/repository/" + repository + "/images?" + q.Encode()
		var p imagesPage
		if err := c.get(ctx, path, &p); err != nil {
			return nil, err
		}
		for _, img := range p.Data {
			for _, r := range img.Repositories {
				if !r.Published {
					continue
				}
				for _, t := range r.Tags {
					keepEarliest(tags, t.Name, r.PushDate)
				}
			}
		}
		seen += len(p.Data)
		if len(p.Data) == 0 || seen >= p.Total {
			return tags, nil
		}
	}
}

// keepEarliest records at for key unless an earlier time is recorded. A zero
// time, a push date the catalog left out, never replaces a known one.
func keepEarliest(m map[string]time.Time, key string, at time.Time) {
	if old, ok := m[key]; !ok || !at.IsZero() && (old.IsZero() || at.Before(old)) {
		m[key] = at
	}
}

func (c *Client) get(ctx context.Context, path string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: %s", path, resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(v)
}

// Shipped holds the published versions of each product in memory, refreshed
// in the background so requests never wait on the catalog.
type Shipped struct {
	client   *Client
	logger   *slog.Logger
	mu       sync.RWMutex
	versions map[string]time.Time // "product X.Y.Z" to its first push
	// Status receives each refresh's outcome; nil reports nowhere.
	Status *syncstatus.Source
}

func NewShipped(client *Client, logger *slog.Logger) *Shipped {
	return &Shipped{client: client, logger: logger}
}

// Run refreshes immediately and then every interval until ctx is cancelled.
func (s *Shipped) Run(ctx context.Context, interval time.Duration) {
	s.Refresh(ctx)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.Refresh(ctx)
		}
	}
}

// Refresh replaces the published set only when every repository was read,
// so a failed pass keeps the last good one.
func (s *Shipped) Refresh(ctx context.Context) {
	versions := make(map[string]time.Time)
	dates := make(map[string]time.Time)
	for product, repos := range Repositories {
		for _, repo := range repos {
			tags, err := s.client.PublishedTags(ctx, repo)
			if err != nil {
				s.logger.Warn("catalog refresh", "repository", repo, "error", err)
				s.Status.Report(fmt.Errorf("read %s: %w", repo, err))
				return
			}
			for t, at := range tags {
				// OMR tags drop the v prefix from 2.0.9 on.
				v := strings.TrimPrefix(t, "v")
				// A respin moves the floating X.Y.Z tag to its new image; the
				// X.Y.Z-<build> tag stays on the first, so it dates the version.
				if base, build, ok := strings.Cut(v, "-"); ok {
					if _, err := strconv.ParseUint(build, 10, 64); err == nil {
						keepEarliest(dates, product+" "+base, at)
						continue
					}
				}
				versions[product+" "+v] = time.Time{}
				keepEarliest(dates, product+" "+v, at)
			}
		}
	}
	for k := range versions {
		versions[k] = dates[k]
	}
	s.mu.Lock()
	s.versions = versions
	s.mu.Unlock()
	s.Status.Report(nil)
	s.logger.Info("catalog refreshed", "tags", len(versions))
}

// Has reports whether product version X.Y.Z is published. A nil Shipped
// (catalog disabled) has nothing published.
func (s *Shipped) Has(product, version string) bool {
	_, ok := s.ShippedAt(product, version)
	return ok
}

// ShippedAt returns when product version X.Y.Z was first pushed to the
// catalog, and whether it is published at all; the time is zero when the
// catalog gave no push date. A nil Shipped has nothing published.
func (s *Shipped) ShippedAt(product, version string) (time.Time, bool) {
	if s == nil {
		return time.Time{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	at, ok := s.versions[product+" "+version]
	return at, ok
}
