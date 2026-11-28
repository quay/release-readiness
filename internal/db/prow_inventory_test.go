package db

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/quay/release-readiness/internal/artbuild"
	"github.com/quay/release-readiness/internal/model"
	"github.com/quay/release-readiness/internal/prow"
)

func TestProwInventoryRevisionRetention(t *testing.T) {
	path := filepath.Join(t.TempDir(), "inventory.db")
	d, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	jobs := []prow.JobDefinition{{Name: "a", Application: "quay-3-18", Target: "test", Revision: "first"}, {Name: "b", Application: "quay-3-18", Target: "test", Revision: "first"}}
	if err = d.SaveJobInventory(t.Context(), jobs, "first"); err != nil {
		t.Fatal(err)
	}
	if err = d.SaveJobInventory(t.Context(), nil, "bad"); err == nil {
		t.Fatal("accepted empty inventory")
	}
	jobs = jobs[:1]
	jobs[0].Revision = "second"
	if err = d.SaveJobInventory(t.Context(), jobs, "second"); err != nil {
		t.Fatal(err)
	}
	_ = d.Close()
	d, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = d.Close() }()
	got, revision, err := d.JobInventory(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if revision != "second" || len(got) != 2 || got[0].Removed || !got[1].Removed {
		t.Fatalf("revision=%s jobs=%+v", revision, got)
	}
}

func TestCandidateWindowUsesARTAndDefersUnknownRelease(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "window.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = d.Close() }()
	if err = d.UpsertReleaseVersion(t.Context(), &model.ReleaseVersion{Name: "quay-v3.16.8", KonfluxApplication: "quay-3-16"}); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)
	if err = d.UpsertArtHistoryBuild(t.Context(), artbuild.HistoryBuild{RecordID: "r", Group: "quay-3.16", Type: artbuild.TypeFBC, NVR: "quay-operator-fbc-3.16.8-20261115120000.ocp4.21", StartTime: at.Format(time.RFC3339)}); err != nil {
		t.Fatal(err)
	}
	got, err := d.CandidateWindowStart(t.Context(), "quay-3-16")
	if err != nil || !got.Equal(at) {
		t.Fatalf("window=%v err=%v", got, err)
	}
	if err = d.UpsertReleaseVersion(t.Context(), &model.ReleaseVersion{Name: "quay-v3.16.9", KonfluxApplication: "quay-3-16"}); err != nil {
		t.Fatal(err)
	}
	got, err = d.CandidateWindowStart(t.Context(), "quay-3-16")
	if err != nil || !got.Equal(at) {
		t.Fatalf("unknown candidate changed known candidate window=%v err=%v", got, err)
	}
}

func TestProwApplicationActivity(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "activity.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = d.Close() }()
	for _, v := range []model.ReleaseVersion{
		{Name: "quay-v3.16.8", KonfluxApplication: "quay-3-16", Released: true},
		{Name: "quay-v3.16.9", KonfluxApplication: "quay-3-16"},
		{Name: "quay-v3.10.1", KonfluxApplication: "quay-3-10", Released: true},
		{Name: "quay-v3.10.2", KonfluxApplication: "quay-3-10", Archived: true},
	} {
		if err = d.UpsertReleaseVersion(t.Context(), &v); err != nil {
			t.Fatal(err)
		}
	}
	activity, err := d.ProwApplicationActivity(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(activity) != 2 || !activity["quay-3-16"] || activity["quay-3-10"] {
		t.Fatalf("wrong lifecycle priorities: %v", activity)
	}
}

func TestCandidateWindowAcceptsARTUTCWithoutZone(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "art-time.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = d.Close() }()
	for _, b := range []artbuild.HistoryBuild{
		{RecordID: "valid", Group: "quay-3.18", Type: artbuild.TypeFBC, NVR: "quay-operator-fbc-3.18.3-20261115120000.ocp4.21", StartTime: "2026-11-15T12:00:00"},
		{RecordID: "old", Group: "quay-3.18", Type: artbuild.TypeFBC, NVR: "quay-operator-fbc-3.18.2-20261101000000.ocp4.21", StartTime: "2026-11-01T00:00:00Z"},
	} {
		if err := d.UpsertArtHistoryBuild(t.Context(), b); err != nil {
			t.Fatal(err)
		}
	}
	got, err := d.CandidateWindowStartForVersions(t.Context(), "quay-3-18", []string{"quay-v3.18.3", "quay-v3.18.4"})
	if err != nil || got.Format(time.RFC3339) != "2026-11-15T12:00:00Z" {
		t.Fatalf("date=%v err=%v", got, err)
	}
	got, err = d.CandidateWindowStartForVersions(t.Context(), "quay-3-18", []string{"quay-v3.18.4"})
	if err != nil || !got.IsZero() {
		t.Fatalf("version without FBC scheduled lookup: %v %v", got, err)
	}
}
