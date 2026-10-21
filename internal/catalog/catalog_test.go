package catalog

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const repoPrefix = "/repositories/registry/registry.access.redhat.com/repository/"

// omrPage publishes 2.0.12 in two images, the earlier push first; 2.0.11 with
// a known push date, then without one; 2.0.10 respun, the floating tag on the
// later image; and one unpublished image, which must not count as shipped.
const omrPage = `{"total":7,"data":[
	{"repositories":[{"published":true,"push_date":"2025-11-17T18:44:43.148000+00:00","tags":[{"name":"2.0.12"}]}]},
	{"repositories":[{"published":true,"push_date":"2025-11-17T18:44:53.304000+00:00","tags":[{"name":"2.0.12"}]}]},
	{"repositories":[{"published":true,"push_date":"2025-10-01T00:00:00Z","tags":[{"name":"2.0.11"}]}]},
	{"repositories":[{"published":true,"tags":[{"name":"2.0.11"}]}]},
	{"repositories":[{"published":true,"push_date":"2025-09-01T00:00:00Z","tags":[{"name":"2.0.10-1756684800"}]}]},
	{"repositories":[{"published":true,"push_date":"2025-09-15T00:00:00Z","tags":[{"name":"2.0.10"},{"name":"2.0.10-1757894400"}]}]},
	{"repositories":[{"published":false,"push_date":"2025-11-01T00:00:00Z","tags":[{"name":"2.0.13"}]}]}]}`

// catalogServer answers the quay repositories with the recorded quay-rhel9
// pages and OMR with omrPage. Setting *fail makes every request a 503.
func catalogServer(t *testing.T, fail *bool) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if *fail {
			http.Error(w, "down", http.StatusServiceUnavailable)
			return
		}
		repo := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, repoPrefix), "/images")
		switch {
		case repo == "openshift/mirror-registry-rhel8":
			_, _ = w.Write([]byte(omrPage))
		case strings.HasPrefix(repo, "quay/"):
			http.ServeFile(w, r, "testdata/quay-rhel9-page"+r.URL.Query().Get("page")+".json")
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestShipped(t *testing.T) {
	fail := false
	srv := catalogServer(t, &fail)
	s := NewShipped(NewClient(srv.URL, srv.Client()), slog.Default())
	s.Refresh(t.Context())

	for _, tc := range []struct {
		product, version string
		want             bool
	}{
		{"quay", "3.17.5", true},
		{"quay", "3.17.6", false},
		{"quay", "3.18.0", true},
		{"omr", "2.0.12", true},  // bare tag
		{"omr", "2.0.13", false}, // unpublished
		{"omr", "3.17.5", false}, // quay tag, other product
	} {
		if got := s.Has(tc.product, tc.version); got != tc.want {
			t.Errorf("Has(%s, %s) = %v, want %v", tc.product, tc.version, got, tc.want)
		}
	}

	for _, tc := range []struct {
		version string
		want    time.Time
	}{
		{"2.0.12", time.Date(2025, 11, 17, 18, 44, 43, 148000000, time.UTC)}, // earliest image
		{"2.0.11", time.Date(2025, 10, 1, 0, 0, 0, 0, time.UTC)},             // missing date ignored
		{"2.0.10", time.Date(2025, 9, 1, 0, 0, 0, 0, time.UTC)},              // first image of a respin
	} {
		if at, ok := s.ShippedAt("omr", tc.version); !ok || !at.Equal(tc.want) {
			t.Errorf("ShippedAt(omr, %s) = %v, %v, want %v", tc.version, at, ok, tc.want)
		}
	}

	fail = true
	s.Refresh(t.Context())
	if !s.Has("quay", "3.17.5") {
		t.Error("failed refresh dropped the last good set")
	}

	var disabled *Shipped
	if disabled.Has("quay", "3.17.5") {
		t.Error("nil Shipped reports a version published")
	}
}
