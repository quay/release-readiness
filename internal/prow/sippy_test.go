package prow

import (
	"context"
	"encoding/json"
	"github.com/quay/release-readiness/internal/syncstatus"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"
)

type sippyMemory struct {
	activity     map[string]bool
	definitions  []JobDefinition
	runs         map[string]Run
	start        time.Time
	largestBatch int
}

func (m *sippyMemory) ProwApplicationActivity(context.Context) (map[string]bool, error) {
	return m.activity, nil
}
func (m *sippyMemory) JobInventory(context.Context) ([]JobDefinition, string, error) {
	return m.definitions, "revision", nil
}
func (m *sippyMemory) SaveJobInventory(context.Context, []JobDefinition, string) error { return nil }
func (m *sippyMemory) CandidateWindowStart(context.Context, string) (time.Time, error) {
	return m.start, nil
}
func (m *sippyMemory) ListProwRunsByApplication(_ context.Context, app string) ([]Run, error) {
	var out []Run
	for _, r := range m.runs {
		if r.Application == app {
			out = append(out, r)
		}
	}
	return out, nil
}
func (m *sippyMemory) UpsertProwRuns(_ context.Context, runs []*Run) error {
	m.largestBatch = max(m.largestBatch, len(runs))
	for _, r := range runs {
		m.runs[r.JobName+"/"+r.BuildID] = *r
	}
	return nil
}

func productionSippy(t *testing.T, jobs []experimentJob) (*SippySync, *experimentAPI, *sippyMemory) {
	t.Helper()
	a := newExperimentAPI(t, jobs)
	m := &sippyMemory{runs: map[string]Run{}, start: time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)}
	for _, j := range jobs {
		m.definitions = append(m.definitions, JobDefinition{Name: j.Name, Application: j.Application, Target: j.Target})
	}
	s := &SippySync{Client: NewSippyClient(a.server.URL), Artifacts: experimentSyncer(a, nil, jobs), Store: m, Workers: 4, Retention: 30 * 24 * time.Hour, Lookback: 24 * time.Hour, now: func() time.Time { return time.Date(2026, 11, 16, 5, 0, 0, 0, time.UTC) }}
	return s, a, m
}

func TestSippyProductionBatchesAndPersistentCache(t *testing.T) {
	s, a, m := productionSippy(t, experimentInventory(t))
	if err := s.SyncOnce(t.Context()); err != nil {
		t.Fatal(err)
	}
	counts := experimentCategories(a.takeCounts())
	if len(m.runs) != 126 || m.largestBatch > 20 || counts["sippy"] != 9 || counts["prowjob"] != 0 || counts["listing"] != 0 || counts["tested-images"] != 126 {
		t.Fatalf("runs=%d batch=%d counts=%v", len(m.runs), m.largestBatch, counts)
	}
	// Recreate the reader; completed evidence comes from the store, not an
	// in-process cache or high-water mark that disappears on restart.
	s.Client = NewSippyClient(a.server.URL)
	if err := s.SyncOnce(t.Context()); err != nil {
		t.Fatal(err)
	}
	counts = experimentCategories(a.takeCounts())
	for kind, n := range counts {
		if kind != "sippy" && n != 0 {
			t.Fatalf("cached GCS reads: %v", counts)
		}
	}
}

func TestSippyProductionWindowAndLateArtifacts(t *testing.T) {
	jobs := experimentInventory(t)[:1]
	s, a, m := productionSippy(t, jobs)
	rows := a.rows[jobs[0].Release]
	m.start = rows[0].Timestamp.Add(30 * time.Second)
	s.Lookback = 0
	path := jobs[0].prowJob().Prefix + rows[1].ID + "/artifacts/" + jobs[0].Target + "/quay-gather/artifacts/tested-images.json"
	artifact := a.objects[path]
	delete(a.objects, path)
	if err := s.SyncOnce(t.Context()); err != nil {
		t.Fatal(err)
	}
	if len(m.runs) != 2 {
		t.Fatalf("window ingested %d runs", len(m.runs))
	}
	key := jobs[0].Name + "/" + rows[1].ID
	if m.runs[key].ArtifactState == ArtifactPresent {
		t.Fatal("missing identity marked present")
	}
	a.objects[path] = artifact
	next := s.clock().Add(3 * time.Minute)
	s.now = func() time.Time { return next }
	if err := s.SyncOnce(t.Context()); err != nil {
		t.Fatal(err)
	}
	if m.runs[key].ArtifactState != ArtifactPresent {
		t.Fatal("late artifact not retried")
	}
}

func TestSippyProductionPageFailureAndUntrustedIdentity(t *testing.T) {
	for _, bad := range []string{"page", "identity"} {
		t.Run(bad, func(t *testing.T) {
			jobs := experimentInventory(t)[:1]
			s, a, m := productionSippy(t, jobs)
			s.Client.PageSize = 1
			if bad == "page" {
				a.fail["/api/jobs/runs?page=1"] = http.StatusServiceUnavailable
			} else {
				a.rows[jobs[0].Release][0].URL = "https://example.invalid/credential"
			}
			if err := s.SyncOnce(t.Context()); err == nil {
				t.Fatal("accepted incomplete or untrusted response")
			}
			if len(m.runs) != 0 {
				t.Fatal("persisted partial stream")
			}
			counts := experimentCategories(a.takeCounts())
			if counts["tested-images"] != 0 || counts["finished"] != 0 {
				t.Fatalf("fetched unvalidated evidence %v", counts)
			}
		})
	}
}

func TestSippyLegacyRefreshFindsUnindexedPendingRun(t *testing.T) {
	jobs := experimentInventory(t)[:1]
	s, a, _ := productionSippy(t, jobs)
	legacy := &memStore{runs: map[string]*Run{}}
	s.Artifacts.store = legacy
	id := "2100388335968063999"
	dir := jobs[0].prowJob().Prefix + id + "/"
	a.lists[jobs[0].prowJob().Prefix] = append(a.lists[jobs[0].prowJob().Prefix], dir)
	start := s.clock().Add(-time.Minute)
	a.objects[dir+"prowjob.json"] = experimentJSON(t, map[string]any{"status": map[string]any{"state": "pending", "startTime": start, "url": "https://prow.ci.openshift.org/view/gcs/" + bucket + "/" + dir}})
	s.LegacyInterval = time.Hour
	s.lastLegacy = s.clock().Add(-2 * time.Hour)
	if err := s.SyncOnce(t.Context()); err != nil {
		t.Fatal(err)
	}
	if legacy.runs[id] == nil || legacy.runs[id].State != "pending" {
		t.Fatal("unindexed pending run not discovered")
	}
	counts := experimentCategories(a.takeCounts())
	if counts["listing"] != 1 {
		t.Fatalf("legacy discovery counts=%v", counts)
	}
	if err := s.SyncOnce(t.Context()); err != nil {
		t.Fatal(err)
	}
	if counts = experimentCategories(a.takeCounts()); counts["listing"] != 0 {
		t.Fatalf("legacy refresh repeated before interval: %v", counts)
	}
}

func TestUnreleasedFirstAndHistoryPacedAndRotated(t *testing.T) {
	jobs := experimentInventory(t)
	s, a, m := productionSippy(t, jobs)
	// Two known historical streams; unknown streams stay foreground.
	m.activity = map[string]bool{"quay-3-10": false, "quay-3-12": false, "quay-3-18": true}
	s.HistoryInterval = time.Hour
	s.HistoryBudget = time.Second
	getCalls := func() []string {
		a.mu.Lock()
		defer a.mu.Unlock()
		out := slices.Clone(a.releases)
		a.releases = nil
		return out
	}
	if err := s.SyncOnce(t.Context()); err != nil {
		t.Fatal(err)
	}
	calls := getCalls()
	if len(calls) != 8 || calls[0] != "quay-3.18" || calls[len(calls)-1] != "quay-3.10" || slices.Contains(calls, "quay-3.12") {
		t.Fatalf("foreground then one history: %v", calls)
	}
	if err := s.SyncOnce(t.Context()); err != nil {
		t.Fatal(err)
	}
	calls = getCalls()
	if len(calls) != 7 || slices.Contains(calls, "quay-3.10") || slices.Contains(calls, "quay-3.12") {
		t.Fatalf("history not paced: %v", calls)
	}
	s.lastHistory = s.clock().Add(-2 * time.Hour)
	if err := s.SyncOnce(t.Context()); err != nil {
		t.Fatal(err)
	}
	calls = getCalls()
	if calls[len(calls)-1] != "quay-3.12" {
		t.Fatalf("history did not rotate: %v", calls)
	}
}

func TestHistoryTimeoutDoesNotFailForeground(t *testing.T) {
	jobs := experimentInventory(t)[:1]
	s, _, m := productionSippy(t, jobs)
	m.activity = map[string]bool{jobs[0].Application: false}
	s.HistoryInterval = time.Hour
	s.HistoryBudget = 20 * time.Millisecond
	status := syncstatus.New()
	s.HistoryStatus = status.Track("prow-history", 0)
	slow := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer slow.Close()
	s.Client = NewSippyClient(slow.URL)
	start := time.Now()
	if err := s.SyncOnce(t.Context()); err != nil {
		t.Fatalf("history failed foreground: %v", err)
	}
	if time.Since(start) > time.Second {
		t.Fatal("history exceeded budget")
	}
	problems := status.Problems(time.Now())
	if len(problems) != 1 || problems[0].Source != "prow-history" {
		t.Fatalf("missing history failure: %v", problems)
	}
}

func TestForegroundFailureSkipsHistory(t *testing.T) {
	s, a, m := productionSippy(t, experimentInventory(t))
	m.activity = map[string]bool{"quay-3-10": false}
	s.HistoryInterval = time.Hour
	a.fail["/api/jobs/runs"] = http.StatusServiceUnavailable
	if err := s.SyncOnce(t.Context()); err == nil {
		t.Fatal("foreground error ignored")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if slices.Contains(a.releases, "quay-3.10") {
		t.Fatal("history ran while foreground failed")
	}
}

func TestCompletionResultCaseAndUnknownRemainConservative(t *testing.T) {
	for _, result := range []string{"success", "failure", "error", "aborted", "SUCCESS", "unexpected"} {
		t.Run(result, func(t *testing.T) {
			jobs := experimentInventory(t)[:1]
			s, a, m := productionSippy(t, jobs)
			row := a.rows[jobs[0].Release][0]
			path := jobs[0].prowJob().Prefix + row.ID + "/finished.json"
			a.objects[path] = experimentJSON(t, finished{Timestamp: row.Timestamp.Add(time.Hour).Unix(), Result: result})
			err := s.SyncOnce(t.Context())
			if result == "unexpected" {
				if err == nil {
					t.Fatal("unknown terminal state accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			run := m.runs[row.Job+"/"+row.ID]
			if run.State != strings.ToLower(result) || run.CompletedAt == nil {
				t.Fatalf("wrong terminal result: %+v", run)
			}
		})
	}
}

func TestLegacyEvidenceBackoffSurvivesReaderRestart(t *testing.T) {
	jobs := experimentInventory(t)[:1]
	s, a, m := productionSippy(t, jobs)
	row := a.rows[jobs[0].Release][0]
	path := jobs[0].prowJob().Prefix + row.ID + "/artifacts/" + jobs[0].Target + "/quay-gather/artifacts/tested-images.json"
	var artifact map[string]any
	if err := json.Unmarshal(a.objects[path], &artifact); err != nil {
		t.Fatal(err)
	}
	artifact["catalog"] = map[string]any{}
	a.objects[path] = experimentJSON(t, artifact)
	later := s.clock().Add(72 * time.Hour)
	s.now = func() time.Time { return later }
	if err := s.SyncOnce(t.Context()); err != nil {
		t.Fatal(err)
	}
	a.takeCounts()
	key := row.Job + "/" + row.ID
	if m.runs[key].CatalogRef != "" {
		t.Fatal("fixture still has exact catalog")
	}
	s.Client = NewSippyClient(a.server.URL)
	if err := s.SyncOnce(t.Context()); err != nil {
		t.Fatal(err)
	}
	counts := experimentCategories(a.takeCounts())
	if counts["tested-images"] != 0 || counts["finished"] != 0 {
		t.Fatalf("legacy evidence refetched: %v", counts)
	}
	later = later.Add(25 * time.Hour)
	if err := s.SyncOnce(t.Context()); err != nil {
		t.Fatal(err)
	}
	counts = experimentCategories(a.takeCounts())
	if counts["tested-images"] != 1 {
		t.Fatalf("late identity never retried: %v", counts)
	}
	if m.runs[key].CatalogRef != "" {
		t.Fatal("backoff fabricated readiness identity")
	}
}

func TestEvidenceRetryDelays(t *testing.T) {
	now := time.Now()
	old := now.Add(-48 * time.Hour)
	recent := now.Add(-time.Hour)
	for _, tc := range []struct {
		run  Run
		want time.Duration
	}{
		{Run{}, 0},
		{Run{CompletedAt: &recent, ArtifactState: ArtifactNone}, 2 * time.Minute},
		{Run{CompletedAt: &old, ArtifactState: ArtifactNone}, 6 * time.Hour},
		{Run{CompletedAt: &old, ArtifactState: ArtifactPresent}, 24 * time.Hour},
		{Run{CompletedAt: &old, ArtifactState: ArtifactOLMCSVs}, 24 * time.Hour},
	} {
		if got := evidenceRetryDelay(tc.run, now); got != tc.want {
			t.Fatalf("got %v want %v", got, tc.want)
		}
	}
}

func TestActiveRunReadsFBCBeforeCompletion(t *testing.T) {
	for _, indexed := range []bool{false, true} {
		for _, artifactPresent := range []bool{false, true} {
			jobs := experimentInventory(t)[:1]
			s, a, _ := productionSippy(t, jobs)
			row := a.rows[jobs[0].Release][0]
			dir := jobs[0].prowJob().Prefix + row.ID + "/"
			delete(a.objects, dir+"finished.json")
			var pj map[string]any
			if err := json.Unmarshal(a.objects[dir+"prowjob.json"], &pj); err != nil {
				t.Fatal(err)
			}
			pj["status"].(map[string]any)["state"] = "pending"
			a.objects[dir+"prowjob.json"] = experimentJSON(t, pj)
			if !artifactPresent {
				delete(a.objects, dir+"artifacts/"+jobs[0].Target+"/quay-gather/artifacts/tested-images.json")
			}
			var run *Run
			var err error
			if indexed {
				run, err = s.Artifacts.fetchIndexedRun(t.Context(), s.Store.(*sippyMemory).definitions[0], IndexedRun{ID: row.ID, Job: row.Job, URL: row.URL, Timestamp: row.Timestamp, Result: "R"})
			} else {
				run, err = s.Artifacts.fetchRun(t.Context(), jobs[0].prowJob(), row.ID)
			}
			if err != nil {
				t.Fatal(err)
			}
			if run == nil || run.State != "pending" || run.CompletedAt != nil {
				t.Fatalf("active state lost: %+v", run)
			}
			if (digest(run.CatalogRef) != "") != artifactPresent {
				t.Fatalf("wrong identity: %+v", run)
			}
			counts := experimentCategories(a.takeCounts())
			if counts["tested-images"] != 1 || counts["install"] != 0 || counts["csv"] != 0 {
				t.Fatalf("active reads: %v", counts)
			}
		}
	}
}

func TestCIDiscoveryWaitsForCandidateThenResumes(t *testing.T) {
	jobs := experimentInventory(t)[:1]
	s, a, m := productionSippy(t, jobs)
	m.start = time.Time{}
	s.Artifacts.store = &memStore{runs: map[string]*Run{}}
	s.LegacyInterval = time.Minute
	if err := s.SyncOnce(t.Context()); err != nil {
		t.Fatal(err)
	}
	if counts := a.takeCounts(); len(counts) != 0 {
		t.Fatalf("inventory alone fetched CI: %v", counts)
	}
	if !s.idle || !s.lastLegacy.IsZero() {
		t.Fatal("idle CI pass reported work or delayed first live lookup")
	}
	m.start = a.rows[jobs[0].Release][0].Timestamp
	if err := s.SyncOnce(t.Context()); err != nil {
		t.Fatal(err)
	}
	counts := experimentCategories(a.takeCounts())
	if counts["sippy"] == 0 || counts["tested-images"] == 0 || counts["listing"] == 0 || s.idle {
		t.Fatalf("candidate did not schedule lookup: %v", counts)
	}
}

func TestOnlyMatchedStreamsScheduleCI(t *testing.T) {
	jobs := experimentInventory(t)
	s, a, m := productionSippy(t, jobs)
	s.WindowStart = func(_ context.Context, app string) (time.Time, error) {
		if app == "quay-3-18" {
			return m.start, nil
		}
		return time.Time{}, nil
	}
	if err := s.SyncOnce(t.Context()); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(a.releases, []string{"quay-3.18"}) {
		t.Fatalf("unmatched streams queried or duplicate query: %v", a.releases)
	}
}

func TestLiveDiscoverySkipsArtifactsOutsideCandidateWindow(t *testing.T) {
	jobs := experimentInventory(t)[:1]
	s, a, _ := productionSippy(t, jobs)
	s.Artifacts.store = &memStore{runs: map[string]*Run{}}
	rows := a.rows[jobs[0].Release]
	from := rows[0].Timestamp.Add(time.Second)
	_, err := s.Artifacts.syncJobWindow(t.Context(), jobs[0].prowJob(), from, s.clock())
	if err != nil {
		t.Fatal(err)
	}
	counts := a.takeCounts()
	for path, n := range counts {
		if n > 0 && strings.Contains(path, rows[0].ID+"/artifacts/") {
			t.Fatalf("read old run artifact %s", path)
		}
	}
}

func TestLegacyModeWaitsForCandidate(t *testing.T) {
	s, a, _ := productionSippy(t, experimentInventory(t)[:1])
	s.Artifacts.WindowStart = func(context.Context, string) (time.Time, error) { return time.Time{}, nil }
	s.Artifacts.SyncOnce(t.Context())
	if counts := a.takeCounts(); len(counts) != 0 {
		t.Fatalf("direct GCS bypassed candidate scheduling: %v", counts)
	}
}

func TestSlowStreamDoesNotStarveAnotherCandidate(t *testing.T) {
	jobs := experimentInventory(t)
	s, a, m := productionSippy(t, jobs)
	s.StreamBudget = 20 * time.Millisecond
	s.WindowStart = func(_ context.Context, app string) (time.Time, error) {
		if app == "quay-3-18" || app == "quay-5-0" {
			return m.start, nil
		}
		return time.Time{}, nil
	}
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("release") == "quay-3.18" {
			<-r.Context().Done()
			return
		}
		a.server.Config.Handler.ServeHTTP(w, r)
	}))
	defer slow.Close()
	s.Client = NewSippyClient(slow.URL)
	if err := s.SyncOnce(t.Context()); err == nil {
		t.Fatal("slow stream error hidden")
	}
	if !slices.Contains(a.releases, "quay-5.0") {
		t.Fatalf("healthy stream starved: %v", a.releases)
	}
}

func TestBadCandidateDateDoesNotBlockOtherStreams(t *testing.T) {
	s, a, m := productionSippy(t, experimentInventory(t))
	s.WindowStart = func(_ context.Context, app string) (time.Time, error) {
		if app == "quay-3-18" {
			return s.clock().Add(time.Hour), nil
		}
		if app == "quay-5-0" {
			return m.start, nil
		}
		return time.Time{}, nil
	}
	if err := s.SyncOnce(t.Context()); err == nil {
		t.Fatal("future date accepted")
	}
	if !slices.Equal(a.releases, []string{"quay-5.0"}) {
		t.Fatalf("healthy candidate not queried: %v", a.releases)
	}
}

func TestForegroundRotation(t *testing.T) {
	s, a, _ := productionSippy(t, experimentInventory(t))
	if err := s.SyncOnce(t.Context()); err != nil {
		t.Fatal(err)
	}
	first := slices.Clone(a.releases)
	a.releases = nil
	if err := s.SyncOnce(t.Context()); err != nil {
		t.Fatal(err)
	}
	if len(a.releases) != len(first) || a.releases[0] != first[1] {
		t.Fatalf("foreground did not rotate: %v -> %v", first, a.releases)
	}
}

func TestDirectGCSRejectsInvalidCompletion(t *testing.T) {
	jobs := experimentInventory(t)[:1]
	s, a, _ := productionSippy(t, jobs)
	row := a.rows[jobs[0].Release][0]
	a.objects[jobs[0].prowJob().Prefix+row.ID+"/finished.json"] = []byte(`{"result":"SUCCESS","timestamp":0}`)
	if run, err := s.Artifacts.fetchRun(t.Context(), jobs[0].prowJob(), row.ID); err == nil || run != nil {
		t.Fatal("invalid completion accepted")
	}
}
