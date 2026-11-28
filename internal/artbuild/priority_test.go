package artbuild

import (
	"context"
	"fmt"
	"github.com/quay/release-readiness/internal/syncstatus"
	"net/url"
	"slices"
	"testing"
	"time"
)

func TestARTForegroundPrecedesBoundedHistoricalWork(t *testing.T) {
	f := &fakeART{}
	h, _ := syncWith(t, f)
	h.Groups = []string{"quay-3.17", "quay-3.16", "quay-5.0"}
	h.Activity = func(context.Context) (map[string]bool, error) {
		return map[string]bool{"quay-3-17": false, "quay-3-16": true, "quay-5-0": true}, nil
	}
	h.BackfillInterval = time.Hour
	h.BackfillBudget = time.Second
	h.SyncOnce(t.Context())
	// Six foreground searches, then every active group backfills on the
	// same visit (7 days each) plus one rotating historical group.
	if len(f.searches) != 27 {
		t.Fatalf("searches=%d", len(f.searches))
	}
	for _, q := range f.searches[:6] {
		if q.Get("group") == "quay-3.17" {
			t.Fatal("historical stream entered foreground")
		}
	}
	if f.searches[6].Get("dateRange") != "2026-10-09 to 2026-10-09" {
		t.Fatal("history started before current days completed")
	}
	f.searches = nil
	h.SyncOnce(t.Context())
	if len(f.searches) != 6 {
		t.Fatalf("backfill was not paced: %d", len(f.searches))
	}
	// After the interval, active groups backfill again without waiting for
	// the rotation, and the historical stream keeps its turn in it.
	h.lastBackfill = h.now().Add(-2 * time.Hour)
	h.SyncOnce(t.Context())
	for _, group := range []string{"quay-3.16", "quay-5.0", "quay-3.17"} {
		if !slices.ContainsFunc(f.searches, func(q url.Values) bool { return q.Get("group") == group }) {
			t.Fatalf("backfill visit skipped %s", group)
		}
	}
}

func TestARTBackfillFailureDoesNotInvalidateCurrent(t *testing.T) {
	f := &fakeART{failDate: "2026-10-09 to 2026-10-09"}
	h, store := syncWith(t, f)
	h.Activity = func(context.Context) (map[string]bool, error) { return map[string]bool{"quay-3-17": true}, nil }
	h.BackfillInterval = time.Hour
	status := syncstatus.New()
	h.Status = status.Track("art-history", time.Minute)
	h.BackfillStatus = status.Track("art-backfill", 0)
	h.SyncOnce(t.Context())
	problems := status.Problems(time.Now())
	if len(problems) != 1 || problems[0].Source != "art-backfill" {
		t.Fatalf("wrong freshness: %+v", problems)
	}
	if slices.Contains(store.scanned, "quay-3.17 fbc 2026-10-09") {
		t.Fatal("failed historical day marked complete")
	}
}

func TestARTForegroundTimeoutIsNotSuccess(t *testing.T) {
	h, store := syncWith(t, &fakeART{})
	h.pace.Reset(time.Hour)
	h.Activity = func(context.Context) (map[string]bool, error) { return nil, nil }
	h.ForegroundBudget = 10 * time.Millisecond
	h.BackfillInterval = time.Hour
	status := syncstatus.New()
	h.Status = status.Track("art-history", time.Minute)
	h.SyncOnce(t.Context())
	if len(store.scanned) != 0 || len(status.Problems(time.Now())) != 1 || !h.lastBackfill.IsZero() {
		t.Fatal("timeout reported success or ran history")
	}
}

func TestARTBudgetPauseIsNotSourceFailure(t *testing.T) {
	f := &fakeART{slowDate: "2026-10-09 to 2026-10-09"}
	h, store := syncWith(t, f)
	h.Activity = func(context.Context) (map[string]bool, error) { return nil, nil }
	h.BackfillInterval = time.Hour
	h.BackfillBudget = 20 * time.Millisecond
	status := syncstatus.New()
	h.Status = status.Track("art-history", time.Minute)
	h.BackfillStatus = status.Track("art-backfill", 0)
	h.SyncOnce(t.Context())
	if p := status.Problems(time.Now()); len(p) != 0 {
		t.Fatalf("intentional pause marked unhealthy: %+v", p)
	}
	if slices.Contains(store.scanned, "quay-3.17 fbc 2026-10-09") {
		t.Fatal("partial day was marked complete")
	}
	for _, source := range status.Sources() {
		if source.Source == "art-backfill" && source.LastSuccess != nil {
			t.Fatal("incomplete backfill reported complete")
		}
	}
}

// Discovered groups join the configured ones in scheduling: a snapshot-derived
// current stream runs in the foreground, a configured historical one keeps to
// the backfill rotation.
func TestARTDiscoveredGroupsJoinScheduling(t *testing.T) {
	f := &fakeART{}
	h, store := syncWith(t, f) // Groups: quay-3.17
	store.apps = []string{"quay-3-18"}
	h.Activity = func(context.Context) (map[string]bool, error) {
		return map[string]bool{"quay-3-17": false, "quay-3-18": true}, nil
	}
	h.BackfillInterval = time.Hour
	h.BackfillBudget = time.Second
	h.SyncOnce(t.Context())
	// The foreground phase scans only the discovered active stream.
	for _, q := range f.searches[:refreshDays] {
		if q.Get("group") != "quay-3.18" {
			t.Fatalf("foreground searched %v, want only the discovered stream", q)
		}
	}
	// The configured historical stream keeps its backfill turn.
	if !slices.ContainsFunc(f.searches, func(q url.Values) bool { return q.Get("group") == "quay-3.17" }) {
		t.Fatal("backfill skipped the configured historical stream")
	}
}

func TestARTConfiguredGroupsWithoutSnapshotsOrPrioritySource(t *testing.T) {
	f := &fakeART{}
	h, store := syncWith(t, f)
	store.apps = []string{}
	h.Groups = []string{"quay-3.18"}
	h.Activity = func(context.Context) (map[string]bool, error) {
		return nil, fmt.Errorf("priority database unavailable")
	}
	h.SyncOnce(t.Context())
	if len(f.searches) != refreshDays {
		t.Fatalf("configured ART collection was blocked: %d requests", len(f.searches))
	}
	for _, q := range f.searches {
		if q.Get("group") != "quay-3.18" {
			t.Fatalf("queried unexpected group: %v", q)
		}
	}
}

func TestARTMissingGroupsDoesNotReportSuccess(t *testing.T) {
	for _, prioritized := range []bool{false, true} {
		t.Run(fmt.Sprint(prioritized), func(t *testing.T) {
			f := &fakeART{}
			h, _ := syncWith(t, f)
			h.Groups = nil
			if prioritized {
				h.Activity = func(context.Context) (map[string]bool, error) { return nil, nil }
			}
			status := syncstatus.New()
			h.Status = status.Track("art-history", time.Minute)
			h.SyncOnce(t.Context())
			if len(f.searches) != 0 || len(status.Problems(time.Now())) != 1 {
				t.Fatal("missing ART configuration not reported")
			}
			for _, source := range status.Sources() {
				if source.LastSuccess != nil {
					t.Fatal("no work reported success")
				}
			}
		})
	}
}

func TestParseARTGroups(t *testing.T) {
	groups, err := ParseGroups("quay-5.0, quay-3.18,quay-5.0")
	if err != nil || !slices.Equal(groups, []string{"quay-3.18", "quay-5.0"}) {
		t.Fatalf("groups=%v err=%v", groups, err)
	}
	for _, value := range []string{"quay-3-18", "quay-3.18,", "../quay-3.18"} {
		if _, err := ParseGroups(value); err == nil {
			t.Fatalf("accepted %q", value)
		}
	}
}
