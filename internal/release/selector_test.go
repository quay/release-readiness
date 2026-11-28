package release

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/quay/release-readiness/internal/artbuild"
	"github.com/quay/release-readiness/internal/catalog"
	"github.com/quay/release-readiness/internal/db"
	"github.com/quay/release-readiness/internal/model"
)

// Shipment MRs are polled for unshipped quay-vX.Y.Z versions, archived ones
// included, and for any whose MR is still open, since a version can ship
// before ART merges its MR.
func TestShipmentVersions(t *testing.T) {
	ctx := t.Context()
	database, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	cat := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"total":1,"data":[{"repositories":[{"published":true,"tags":[{"name":"v3.17.5"}]}]}]}`))
	}))
	defer cat.Close()
	shipped := catalog.NewShipped(catalog.NewClient(cat.URL, cat.Client()), slog.Default())
	shipped.Refresh(ctx)

	for _, rv := range []model.ReleaseVersion{
		{Name: "quay-v3.18.0", Released: true},
		{Name: "quay-v3.17.4", Released: true},
		{Name: "quay-v3.17.5"},
		{Name: "quay-v3.17.6"},
		{Name: "quay-v3.16.9", Archived: true},
		{Name: "omr-v1.2.3"},
		{Name: "quay-v3.18"},
	} {
		if err := database.UpsertReleaseVersion(ctx, &rv); err != nil {
			t.Fatalf("upsert release: %v", err)
		}
	}
	for _, mr := range []model.ShipmentMR{
		{Version: "quay-v3.18.0", IID: 1, State: "opened", Source: "pointer"},
		{Version: "quay-v3.17.4", IID: 2, State: "merged", Source: "pointer"},
	} {
		if err := database.UpsertShipmentMR(ctx, &mr); err != nil {
			t.Fatalf("upsert shipment MR: %v", err)
		}
	}

	polled, err := NewSelector(database, shipped).ShipmentVersions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"quay-v3.16.9", "quay-v3.17.6", "quay-v3.18.0"}; !slices.Equal(polled, want) {
		t.Errorf("polled %v, want %v", polled, want)
	}
}

func TestIngestionPriorityUsesFreshPublicationAndMixedStreams(t *testing.T) {
	database, err := db.Open(filepath.Join(t.TempDir(), "priority.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = database.Close() }()
	var failing atomic.Bool
	cat := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if failing.Load() {
			w.WriteHeader(503)
			return
		}
		_, _ = w.Write([]byte(`{"total":1,"data":[{"repositories":[{"published":true,"tags":[{"name":"3.17.5"},{"name":"3.16.7"}]}]}]}`))
	}))
	defer cat.Close()
	shipped := catalog.NewShipped(catalog.NewClient(cat.URL, cat.Client()), slog.Default())
	shipped.Refresh(t.Context())
	for _, v := range []model.ReleaseVersion{
		{Name: "quay-v3.17.5", KonfluxApplication: "quay-3-17"},
		{Name: "quay-v3.16.7", KonfluxApplication: "quay-3-16"},
		{Name: "quay-v3.16.8", KonfluxApplication: "quay-3-16"},
		{Name: "quay-v5.0.0", KonfluxApplication: "quay-5-0"},
	} {
		if err := database.UpsertReleaseVersion(t.Context(), &v); err != nil {
			t.Fatal(err)
		}
	}
	s := NewSelector(database, shipped)
	active, err := s.IngestionActivity(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if active["quay-3-17"] || !active["quay-3-16"] || !active["quay-5-0"] {
		t.Fatalf("wrong priorities %v", active)
	}
	if _, known := active["quay-9-0"]; known {
		t.Fatal("unknown stream classified historical")
	}
	if !shipped.Fresh(time.Now()) || shipped.Fresh(time.Now().Add(4*time.Hour)) {
		t.Fatal("publication freshness not bounded")
	}
	// The published z-stream must not widen the upcoming candidate window.
	for _, b := range []artbuild.HistoryBuild{
		{RecordID: "old", Group: "quay-3.16", Type: artbuild.TypeFBC, NVR: "quay-operator-fbc-3.16.7-20261101000000.ocp4.21", StartTime: "2026-11-01T00:00:00Z"},
		{RecordID: "new", Group: "quay-3.16", Type: artbuild.TypeFBC, NVR: "quay-operator-fbc-3.16.8-20261115000000.ocp4.21", StartTime: "2026-11-15T00:00:00Z"},
	} {
		if err := database.UpsertArtHistoryBuild(t.Context(), b); err != nil {
			t.Fatal(err)
		}
	}
	start, err := s.CIWindowStart(t.Context(), "quay-3-16")
	if err != nil || start.Format(time.RFC3339) != "2026-11-15T00:00:00Z" {
		t.Fatalf("published release widened CI window: %v %v", start, err)
	}
	failing.Store(true)
	shipped.Refresh(t.Context())
	active, err = s.IngestionActivity(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if !active["quay-3-17"] {
		t.Fatal("failed catalog demoted stream")
	}
}
