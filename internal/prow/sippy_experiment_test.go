package prow

// Loopback mock infrastructure and request-count regression comparison.
// Artifact enrichment now exercises the production Sippy-assisted reader.
import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

type experimentJob struct {
	Name        string `json:"name"`
	Application string `json:"application"`
	Release     string `json:"release"`
	Target      string `json:"target"`
}

func (j experimentJob) prowJob() Job {
	return Job{Name: j.Name, Application: j.Application, Prefix: "logs/" + j.Name + "/"}
}

type experimentRow struct {
	ID        string    `json:"prow_id"`
	Job       string    `json:"job"`
	URL       string    `json:"url"`
	Timestamp time.Time `json:"timestamp"`
	Result    string    `json:"overall_result"`
	Succeeded bool      `json:"succeeded"`
}

type experimentPage struct {
	Rows     []experimentRow `json:"rows"`
	Page     int             `json:"page"`
	PageSize int             `json:"page_size"`
	Total    int             `json:"total_rows"`
}

type experimentWindow struct {
	From, Until time.Time
}

type experimentFilter struct {
	LinkOperator string `json:"linkOperator"`
	Items        []struct {
		Field    string `json:"columnField"`
		Operator string `json:"operatorValue"`
		Value    string `json:"value"`
	} `json:"items"`
}

// experimentAPI supplies separate Sippy and GCS endpoints on one loopback
// server. Every run and artifact is generated locally, including large IDs.
type experimentAPI struct {
	server   *httptest.Server
	rows     map[string][]experimentRow
	objects  map[string][]byte
	lists    map[string][]string
	fail     map[string]int
	mu       sync.Mutex
	counts   map[string]int
	releases []string
}

func experimentJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func experimentInventory(t *testing.T) []experimentJob {
	t.Helper()
	var inventory struct {
		Jobs []experimentJob `json:"jobs"`
	}
	if err := json.Unmarshal(readFixture(t, "sippy-experiment/periodics.json"), &inventory); err != nil {
		t.Fatal(err)
	}
	if len(inventory.Jobs) != 42 {
		t.Fatalf("inventory = %d jobs, want 42", len(inventory.Jobs))
	}
	return inventory.Jobs
}

func newExperimentAPI(t *testing.T, jobs []experimentJob) *experimentAPI {
	t.Helper()
	a := &experimentAPI{
		rows: map[string][]experimentRow{}, objects: map[string][]byte{},
		lists: map[string][]string{}, fail: map[string]int{}, counts: map[string]int{},
	}
	for i, job := range jobs {
		for n := range 3 {
			id := strconv.FormatUint(2100388335968063488+uint64(i*3+n), 10)
			start := time.Date(2026, 11, 16, 0, n, 0, 0, time.UTC)
			state, result := "success", "S"
			if n == 2 {
				state, result = "failure", "F"
			}
			row := experimentRow{ID: id, Job: job.Name, URL: "https://prow.ci.openshift.org/view/gcs/" + bucket + "/logs/" + job.Name + "/" + id, Timestamp: start, Result: result, Succeeded: state == "success"}
			a.rows[job.Release] = append(a.rows[job.Release], row)
			dir := job.prowJob().Prefix + id + "/"
			a.lists[job.prowJob().Prefix] = append(a.lists[job.prowJob().Prefix], dir)
			pj := prowJob{}
			pj.Status.State, pj.Status.StartTime, pj.Status.URL = state, &start, row.URL
			// Build the real prowjob.json shape without adding Sippy-only fields.
			a.objects[dir+"prowjob.json"] = experimentJSON(t, map[string]any{
				"status": pj.Status,
				"spec":   map[string]any{"pod_spec": map[string]any{"containers": []any{map[string]any{"args": []string{"--target=" + job.Target}}}}},
			})
			a.objects[dir+"finished.json"] = experimentJSON(t, finished{Timestamp: start.Add(time.Hour).Unix(), Result: strings.ToUpper(state)})
			artifact := testedImages{SchemaVersion: 1}
			artifact.Catalog.ResolvedRef = "quay.io/example/fbc@sha256:" + strings.Repeat("a", 64)
			artifact.Operator.BundleRef = "quay.io/example/bundle@sha256:" + strings.Repeat("b", 64)
			if err := json.Unmarshal([]byte(`{"images":[{"role":"quay","requested_ref":"quay.io/example/quay@sha256:`+strings.Repeat("c", 64)+`","source":"pod"}]}`), &artifact); err != nil {
				t.Fatal(err)
			}
			a.objects[dir+"artifacts/"+job.Target+"/quay-gather/artifacts/tested-images.json"] = experimentJSON(t, artifact)
			if step := installSteps[job.Target]; step != "" {
				a.objects[dir+"artifacts/"+job.Target+"/"+step+"/finished.json"] = experimentJSON(t, finished{Timestamp: start.Add(10 * time.Minute).Unix(), Result: strings.ToUpper(state)})
			}
		}
	}
	a.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a.mu.Lock()
		a.counts[r.URL.Path]++
		if r.URL.Path == "/api/jobs/runs" {
			a.releases = append(a.releases, r.URL.Query().Get("release"))
		}
		a.mu.Unlock()
		if code := a.fail[r.URL.Path]; code != 0 {
			http.Error(w, "simulated source failure", code)
			return
		}
		if code := a.fail[r.URL.Path+"?page="+r.URL.Query().Get("page")]; code != 0 {
			http.Error(w, "simulated page failure", code)
			return
		}
		if r.URL.Path == "/api/jobs/runs" {
			q := r.URL.Query()
			page, e1 := strconv.Atoi(q.Get("page"))
			size, e2 := strconv.Atoi(q.Get("perPage"))
			if e1 != nil || e2 != nil || page < 0 || size < 1 || q.Get("release") == "" {
				http.Error(w, "invalid pagination or missing release", http.StatusBadRequest)
				return
			}
			rows := a.rows[q.Get("release")]
			if raw := q.Get("filter"); raw != "" {
				var filter experimentFilter
				if err := json.Unmarshal([]byte(raw), &filter); err != nil || filter.LinkOperator != "and" {
					http.Error(w, "invalid filter", http.StatusBadRequest)
					return
				}
				rows = slices.Clone(rows)
				for _, item := range filter.Items {
					boundary, err := time.Parse(time.RFC3339Nano, item.Value)
					if err != nil || item.Field != "timestamp" || (item.Operator != ">=" && item.Operator != "<=") {
						http.Error(w, "unsupported mock filter", http.StatusBadRequest)
						return
					}
					rows = slices.DeleteFunc(rows, func(row experimentRow) bool {
						return (item.Operator == ">=" && row.Timestamp.Before(boundary)) || (item.Operator == "<=" && row.Timestamp.After(boundary))
					})
				}
			}
			start := min(page*size, len(rows))
			_, _ = w.Write(experimentJSON(t, experimentPage{Rows: rows[start:min(start+size, len(rows))], Page: page, PageSize: size, Total: len(rows)}))
			return
		}
		if r.URL.Path == "/storage/v1/b/"+bucket+"/o" {
			_, _ = w.Write(experimentJSON(t, map[string]any{"prefixes": a.lists[r.URL.Query().Get("prefix")]}))
			return
		}
		if body, ok := a.objects[strings.TrimPrefix(r.URL.Path, "/"+bucket+"/")]; ok {
			_, _ = w.Write(body)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(a.server.Close)
	return a
}

func (a *experimentAPI) takeCounts() map[string]int {
	a.mu.Lock()
	defer a.mu.Unlock()
	counts := a.counts
	a.counts = map[string]int{}
	return counts
}

func experimentCategories(counts map[string]int) map[string]int {
	categories := map[string]int{"sippy": 0, "listing": 0, "prowjob": 0, "finished": 0, "tested-images": 0, "install": 0, "csv": 0}
	for path, count := range counts {
		switch {
		case path == "/api/jobs/runs":
			categories["sippy"] += count
		case strings.HasPrefix(path, "/storage/v1/"):
			categories["listing"] += count
		case strings.HasSuffix(path, "/prowjob.json"):
			categories["prowjob"] += count
		case strings.HasSuffix(path, "/tested-images.json"):
			categories["tested-images"] += count
		case strings.HasSuffix(path, "/olm-csvs.yaml"):
			categories["csv"] += count
		case strings.Contains(path, "/artifacts/"):
			categories["install"] += count
		default:
			categories["finished"] += count
		}
	}
	return categories
}

// experimentRows validates the complete paginated snapshot before enrichment.
// No Sippy row can add a new requirement or silently remove a configured job.
func experimentRows(ctx context.Context, client *Client, release string, size int, windows ...experimentWindow) ([]experimentRow, error) {
	var rows []experimentRow
	total := -1
	for page := 0; page < 1000; page++ {
		q := url.Values{"release": {release}, "page": {strconv.Itoa(page)}, "perPage": {strconv.Itoa(size)}, "sortField": {"timestamp"}, "sort": {"desc"}}
		if len(windows) != 0 {
			filter := experimentFilter{LinkOperator: "and"}
			for _, bound := range []struct {
				op string
				t  time.Time
			}{{">=", windows[0].From}, {"<=", windows[0].Until}} {
				filter.Items = append(filter.Items, struct {
					Field    string `json:"columnField"`
					Operator string `json:"operatorValue"`
					Value    string `json:"value"`
				}{"timestamp", bound.op, bound.t.UTC().Format(time.RFC3339Nano)})
			}
			body, err := json.Marshal(filter)
			if err != nil {
				return nil, err
			}
			q.Set("filter", string(body))
		}
		body, err := client.get(ctx, client.baseURL+"/api/jobs/runs?"+q.Encode())
		if err != nil {
			return nil, err
		}
		var response experimentPage
		if err := json.Unmarshal(body, &response); err != nil {
			return nil, err
		}
		if response.Page != page || response.PageSize != size || response.Total < 0 || len(response.Rows) > size || (total >= 0 && total != response.Total) {
			return nil, fmt.Errorf("inconsistent Sippy pagination")
		}
		total = response.Total
		rows = append(rows, response.Rows...)
		if len(rows) == total {
			return rows, nil
		}
		if len(rows) > total || len(response.Rows) == 0 {
			return nil, fmt.Errorf("incomplete Sippy pagination")
		}
	}
	return nil, fmt.Errorf("Sippy page budget exceeded")
}

func experimentHybrid(ctx context.Context, sippy *Client, gcs *Syncer, jobs []experimentJob, cache map[string]*Run, pageSize int, windows ...experimentWindow) (map[string]*Run, error) {
	byName := map[string]experimentJob{}
	var releases []string
	for _, job := range jobs {
		byName[job.Name] = job
		if !slices.Contains(releases, job.Release) {
			releases = append(releases, job.Release)
		}
	}
	var allRows []experimentRow
	for _, release := range releases {
		rows, err := experimentRows(ctx, sippy, release, pageSize, windows...)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			if job, ok := byName[row.Job]; ok && job.Release == release {
				allRows = append(allRows, row)
			}
		}
	}
	runs := map[string]*Run{}
	for _, row := range allRows {
		job := byName[row.Job]
		if _, err := strconv.ParseUint(row.ID, 10, 64); err != nil {
			return nil, fmt.Errorf("invalid Prow ID")
		}
		// Never fetch URLs supplied by Sippy. Check identity, then use the
		// configured GCS client and inventory-derived object prefix.
		u, err := url.Parse(row.URL)
		if err != nil || u.Scheme != "https" || u.Host != "prow.ci.openshift.org" || u.Path != "/view/gcs/"+bucket+"/logs/"+job.Name+"/"+row.ID || row.Timestamp.IsZero() {
			return nil, fmt.Errorf("invalid Sippy run identity")
		}
		key := row.Job + "/" + row.ID
		if _, seen := runs[key]; seen {
			continue
		}
		if cached := cache[key]; cached != nil {
			runs[key] = cached
			continue
		}
		run, err := experimentEnrich(ctx, gcs, job, row)
		if err != nil {
			return nil, err
		}
		runs[key] = run
		if run.CompletedAt != nil && run.ArtifactState == ArtifactPresent && digest(run.CatalogRef) != "" {
			cache[key] = run
		}
	}
	return runs, nil
}

func experimentEnrich(ctx context.Context, s *Syncer, job experimentJob, row experimentRow) (*Run, error) {
	r, err := s.fetchIndexedRun(ctx, JobDefinition{Name: job.Name, Application: job.Application, Target: job.Target}, IndexedRun{ID: row.ID, Job: row.Job, URL: row.URL, Timestamp: row.Timestamp, Result: row.Result})
	if r != nil {
		r.FetchedAt = time.Time{}
	}
	return r, err
}

func experimentSyncer(a *experimentAPI, store Store, jobs []experimentJob) *Syncer {
	var configured []Job
	for _, j := range jobs {
		configured = append(configured, j.prowJob())
	}
	s := NewSyncer(NewClient(a.server.URL), store, time.Minute, slog.New(slog.DiscardHandler))
	s.jobs = configured
	return s
}

func TestSippyExperimentRequestComparison(t *testing.T) {
	jobs := experimentInventory(t)
	a := newExperimentAPI(t, jobs)
	store := &memStore{runs: map[string]*Run{}}
	s := experimentSyncer(a, store, jobs)
	// Exercise the existing production reader as the baseline.
	for _, job := range jobs {
		if _, err := s.syncJob(t.Context(), job.prowJob()); err != nil {
			t.Fatal(err)
		}
	}
	baseline := experimentCategories(a.takeCounts())
	cache := map[string]*Run{}
	runs, err := experimentHybrid(t.Context(), s.client, s, jobs, cache, 100)
	if err != nil {
		t.Fatal(err)
	}
	hybrid := experimentCategories(a.takeCounts())
	if len(runs) != 126 || len(store.runs) != len(runs) {
		t.Fatalf("runs: hybrid=%d baseline=%d, want 126", len(runs), len(store.runs))
	}
	for _, got := range runs {
		want := *store.runs[got.BuildID]
		want.FetchedAt = time.Time{}
		if !reflect.DeepEqual(*got, want) {
			t.Fatalf("evidence differs for %s: hybrid=%+v baseline=%+v", got.BuildID, *got, want)
		}
	}
	if baseline["listing"] != 42 || baseline["prowjob"] != 126 || hybrid["sippy"] != 9 || hybrid["listing"] != 0 || hybrid["prowjob"] != 0 {
		t.Fatalf("unexpected request counts: baseline=%v hybrid=%v", baseline, hybrid)
	}
	for _, category := range []string{"finished", "tested-images", "install", "csv"} {
		if baseline[category] != hybrid[category] {
			t.Fatalf("lost %s evidence: baseline=%v hybrid=%v", category, baseline, hybrid)
		}
	}
	for _, job := range jobs {
		if _, err := s.syncJob(t.Context(), job.prowJob()); err != nil {
			t.Fatal(err)
		}
	}
	warmBaseline := experimentCategories(a.takeCounts())
	if _, err := experimentHybrid(t.Context(), s.client, s, jobs, cache, 100); err != nil {
		t.Fatal(err)
	}
	warmHybrid := experimentCategories(a.takeCounts())
	for category, n := range warmHybrid {
		if category != "sippy" && n != 0 {
			t.Fatalf("warm cache fetched GCS: %v", warmHybrid)
		}
	}
	t.Logf("cold baseline: %v", baseline)
	t.Logf("cold hybrid:   %v", hybrid)
	t.Logf("warm baseline: %v", warmBaseline)
	t.Logf("warm hybrid:   %v", warmHybrid)
}

func TestSippyExperimentPaginationAndLargeIDs(t *testing.T) {
	jobs := experimentInventory(t)[:1]
	a := newExperimentAPI(t, jobs)
	rows := a.rows[jobs[0].Release]
	// Duplicate boundary rows and unrelated jobs must not duplicate artifact
	// fetching or add jobs to the validation inventory.
	foreign := rows[0]
	foreign.Job = "unconfigured-job"
	a.rows[jobs[0].Release] = append(rows, rows[0], foreign)
	s := experimentSyncer(a, nil, jobs)
	runs, err := experimentHybrid(t.Context(), s.client, s, jobs, map[string]*Run{}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 3 || runs[jobs[0].Name+"/2100388335968063489"] == nil {
		t.Fatalf("lost/rounded run IDs or duplicated runs: %v", runs)
	}
	counts := experimentCategories(a.takeCounts())
	if counts["sippy"] != 5 || counts["finished"] != 3 || counts["tested-images"] != 3 {
		t.Fatalf("pagination/deduplication requests = %v", counts)
	}
}

func TestSippyExperimentEvidenceGaps(t *testing.T) {
	for _, scenario := range []string{"unknown target", "changed target", "missing artifact", "invalid artifact", "missing completion", "unknown result", "running", "Sippy error", "later page error", "absent stream", "untrusted URL"} {
		t.Run(scenario, func(t *testing.T) {
			jobs := experimentInventory(t)[:1]
			a := newExperimentAPI(t, jobs)
			row := a.rows[jobs[0].Release][0]
			a.rows[jobs[0].Release] = []experimentRow{row}
			dir := jobs[0].prowJob().Prefix + row.ID + "/"
			artifactPath := dir + "artifacts/" + jobs[0].Target + "/quay-gather/artifacts/tested-images.json"
			expectError := false
			switch scenario {
			case "unknown target":
				jobs[0].Target = ""
			case "changed target":
				jobs[0].Target = "new-target-not-used-by-old-run"
			case "missing artifact":
				delete(a.objects, artifactPath)
			case "invalid artifact":
				a.objects[artifactPath] = []byte(`{"schema_version":999}`)
			case "missing completion":
				delete(a.objects, dir+"finished.json")
			case "unknown result":
				a.rows[jobs[0].Release][0].Result = "new-status"
			case "running":
				a.rows[jobs[0].Release][0].Result = "R"
				a.rows[jobs[0].Release][0].Succeeded = false
				delete(a.objects, dir+"finished.json")
				delete(a.objects, artifactPath)
				var pj map[string]any
				if err := json.Unmarshal(a.objects[dir+"prowjob.json"], &pj); err != nil {
					t.Fatal(err)
				}
				pj["status"].(map[string]any)["state"] = "pending"
				a.objects[dir+"prowjob.json"] = experimentJSON(t, pj)
			case "Sippy error":
				a.fail["/api/jobs/runs"] = http.StatusServiceUnavailable
				expectError = true
			case "later page error":
				a.rows[jobs[0].Release] = []experimentRow{row, row}
				a.fail["/api/jobs/runs?page=1"] = http.StatusServiceUnavailable
				expectError = true
			case "absent stream":
				delete(a.rows, jobs[0].Release)
			case "untrusted URL":
				a.rows[jobs[0].Release][0].URL = "https://example.invalid/private"
				expectError = true
			}
			s := experimentSyncer(a, nil, jobs)
			cache := map[string]*Run{}
			runs, err := experimentHybrid(t.Context(), s.client, s, jobs, cache, 1)
			if (err != nil) != expectError {
				t.Fatalf("error = %v, want error %v", err, expectError)
			}
			if scenario == "unknown target" || scenario == "changed target" {
				if len(cache) != 1 || runs[row.Job+"/"+row.ID].ArtifactState != ArtifactPresent {
					t.Fatalf("target fallback lost identity: %v", runs)
				}
				counts := experimentCategories(a.takeCounts())
				if counts["prowjob"] != 1 {
					t.Fatalf("target fallback must read run-specific config: %v", counts)
				}
				return
			}
			if len(cache) != 0 {
				t.Fatalf("incomplete/invalid evidence cached as complete: %v", cache)
			}
			if scenario == "absent stream" && len(runs) != 0 {
				t.Fatal("missing stream fabricated runs")
			}
			if scenario == "missing completion" || scenario == "running" || scenario == "unknown result" {
				run := runs[row.Job+"/"+row.ID]
				if run.CompletedAt != nil || run.State == "success" || run.CatalogRef != "" {
					t.Fatalf("fabricated terminal success/identity: %+v", run)
				}
			}
		})
	}
}

func TestSippyExperimentLateArtifactRetry(t *testing.T) {
	jobs := experimentInventory(t)[:1]
	a := newExperimentAPI(t, jobs)
	row := a.rows[jobs[0].Release][0]
	a.rows[jobs[0].Release] = []experimentRow{row}
	path := jobs[0].prowJob().Prefix + row.ID + "/artifacts/" + jobs[0].Target + "/quay-gather/artifacts/tested-images.json"
	artifact := a.objects[path]
	delete(a.objects, path)
	s := experimentSyncer(a, nil, jobs)
	cache := map[string]*Run{}
	if _, err := experimentHybrid(t.Context(), s.client, s, jobs, cache, 100); err != nil {
		t.Fatal(err)
	}
	if len(cache) != 0 {
		t.Fatal("missing artifact was permanently cached")
	}
	a.objects[path] = artifact
	if _, err := experimentHybrid(t.Context(), s.client, s, jobs, cache, 100); err != nil {
		t.Fatal(err)
	}
	if len(cache) != 1 {
		t.Fatal("late artifact was not retried")
	}
}

func TestSippyExperimentRejectsIncompletePagination(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"rows":[],"page":0,"page_size":100,"total_rows":2}`))
	}))
	t.Cleanup(srv.Close)
	if _, err := experimentRows(t.Context(), NewClient(srv.URL), "quay-3.18", 100); err == nil {
		t.Fatal("accepted incomplete pagination")
	}
}

func TestSippyExperimentFBCWindowAndDigest(t *testing.T) {
	var jobs []experimentJob
	for _, job := range experimentInventory(t) {
		if job.Application == "quay-3-16" {
			jobs = append(jobs, job)
		}
	}
	a := newExperimentAPI(t, jobs[:1]) // other configured 3.16 jobs have no runs
	rows := a.rows["quay-3.16"]
	// All three runs are green. Only the third tests the selected FBC.
	for i, row := range rows {
		a.rows["quay-3.16"][i].Result = "S"
		a.rows["quay-3.16"][i].Succeeded = true
		dir := jobs[0].prowJob().Prefix + row.ID + "/"
		a.objects[dir+"finished.json"] = experimentJSON(t, finished{Timestamp: row.Timestamp.Add(time.Hour).Unix(), Result: "SUCCESS"})
		if i != 2 {
			path := dir + "artifacts/" + jobs[0].Target + "/quay-gather/artifacts/tested-images.json"
			a.objects[path] = []byte(strings.ReplaceAll(string(a.objects[path]), strings.Repeat("a", 64), strings.Repeat("d", 64)))
		}
	}
	// Mock ART says the 3.16.8 FBC became available between runs 0 and 1.
	window := experimentWindow{From: rows[0].Timestamp.Add(30 * time.Second), Until: rows[2].Timestamp.Add(time.Minute)}
	s := experimentSyncer(a, nil, jobs)
	runs, err := experimentHybrid(t.Context(), s.client, s, jobs, map[string]*Run{}, 100, window)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 2 {
		t.Fatalf("date filter returned %d runs, want 2", len(runs))
	}
	matches := map[string][]string{}
	for _, job := range jobs {
		matches[job.Name] = nil // keep configured jobs with no matching evidence
	}
	for _, run := range runs {
		if run.State == "success" && digest(run.CatalogRef) == "sha256:"+strings.Repeat("a", 64) {
			matches[run.JobName] = append(matches[run.JobName], run.BuildID)
		}
	}
	if !slices.Equal(matches[jobs[0].Name], []string{rows[2].ID}) {
		t.Fatalf("date alone accepted an old FBC: %v", matches)
	}
	for _, job := range jobs[1:] {
		if len(matches[job.Name]) != 0 {
			t.Fatal("configured job without a run received evidence")
		}
	}
	counts := experimentCategories(a.takeCounts())
	if counts["sippy"] != 1 || counts["tested-images"] != 2 || counts["finished"] != 2 || counts["listing"] != 0 || counts["prowjob"] != 0 {
		t.Fatalf("window request counts = %v", counts)
	}
	t.Logf("3.16.8 candidate: configured=%d, runs in window=%d, exact-FBC matching runs=1; requests=%v", len(jobs), len(runs), counts)
}
