package reconcile

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

type fakeStore struct {
	fbcs      []FBCBuild
	members   map[string][]string
	records   map[string][]ARTRecord
	sources   map[string][]Source
	unchecked map[string]bool
	baselines map[slot]Baseline
	actions   map[ActionKey]Action
	runs      map[[2]string]bool
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		members:   map[string][]string{},
		records:   map[string][]ARTRecord{},
		sources:   map[string][]Source{},
		unchecked: map[string]bool{},
		baselines: map[slot]Baseline{},
		actions:   map[ActionKey]Action{},
		runs:      map[[2]string]bool{},
	}
}

func (f *fakeStore) SourcesByDigest(_ context.Context, digest string) ([]Source, error) {
	return f.sources[digest], nil
}

func (f *fakeStore) ARTRecords(_ context.Context, digests []string) ([]ARTRecord, error) {
	var records []ARTRecord
	for _, d := range digests {
		records = append(records, f.records[d]...)
	}
	return records, nil
}

func (f *fakeStore) SourceState(_ context.Context, digest string) (bool, bool, error) {
	return len(f.sources[digest]) > 0 || f.unchecked[digest], f.unchecked[digest], nil
}

func (f *fakeStore) GetBaseline(_ context.Context, stream, operator, ocp string) (string, bool, error) {
	b, ok := f.baselines[slot{stream, operator, ocp}]
	return b.FBCDigest, ok, nil
}

func (f *fakeStore) InsertBaseline(_ context.Context, b Baseline) error {
	k := slot{b.Stream, b.Operator, b.OCP}
	if _, ok := f.baselines[k]; !ok {
		f.baselines[k] = b
	}
	return nil
}

func (f *fakeStore) UpsertAction(_ context.Context, a Action) error {
	old, ok := f.actions[a.Key]
	if !ok || a.Attempts == 0 && old.Attempts == 0 && old.Status != StatusProposed ||
		a.Attempts > 0 && a.Attempts >= old.Attempts && (old.Status == StatusProposed || old.Status == StatusError) {
		f.actions[a.Key] = a
	}
	return nil
}

func (f *fakeStore) PendingActions(_ context.Context, maxAttempts int) ([]Action, error) {
	var pending []Action
	for _, a := range f.actions {
		if a.Status == StatusProposed || a.Status == StatusError && a.Attempts < maxAttempts {
			pending = append(pending, a)
		}
	}
	return pending, nil
}

func (f *fakeStore) FirstIncludedAction(_ context.Context, stream, issueKey string) (Action, bool, error) {
	var first Action
	var found bool
	for _, a := range f.actions {
		if a.Key.IssueKey != issueKey || !strings.HasPrefix(a.Assembly, stream+".") || !slices.Contains([]string{StatusProposed, StatusApplied, StatusError}, a.Status) {
			continue
		}
		if !found || a.UpdatedAt.Before(first.UpdatedAt) || a.UpdatedAt.Equal(first.UpdatedAt) && a.Key.Release < first.Key.Release {
			first, found = a, true
		}
	}
	return first, found, nil
}

func (f *fakeStore) ListFBCBuilds(context.Context) ([]FBCBuild, error) { return f.fbcs, nil }

func (f *fakeStore) FBCMemberDigests(_ context.Context, digest string) ([]string, error) {
	return f.members[digest], nil
}

func (f *fakeStore) RunDone(_ context.Context, nvr, digest string) (bool, error) {
	return f.runs[[2]string{nvr, digest}], nil
}

func (f *fakeStore) MarkRun(_ context.Context, nvr, digest string) error {
	f.runs[[2]string{nvr, digest}] = true
	return nil
}

type fakeCommits struct {
	ranges map[string][]Commit
	errs   map[string]error
	// branches holds each resolved Branch by repo, prefix and commit; any
	// other is unresolved.
	branches map[string]string
	calls    []string
}

func (f *fakeCommits) Range(_ context.Context, repo, branch, base, head string) ([]Commit, error) {
	k := strings.Join([]string{repo, branch, base, head}, " ")
	f.calls = append(f.calls, k)
	if err := f.errs[k]; err != nil {
		return nil, err
	}
	return f.ranges[k], nil
}

func sha(n int) string { return fmt.Sprintf("%040x", n) }

const (
	quayRepo = "https://github.com/quay/quay"
	opRepo   = "https://github.com/quay/quay-operator"
)

type fixture struct {
	store   *fakeStore
	commits *fakeCommits
	sync    *Sync
	clock   time.Time
	logs    bytes.Buffer
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	fx := &fixture{store: newFakeStore(), commits: &fakeCommits{ranges: map[string][]Commit{}, errs: map[string]error{}}, clock: time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)}
	fx.sync = NewSync(fx.store, fx.commits, slog.New(slog.NewTextHandler(&fx.logs, nil)))
	fx.sync.now = func() time.Time { fx.clock = fx.clock.Add(time.Second); return fx.clock }
	return fx
}

// build adds a rendered ART FBC build of quay-operator for OCP 4.16, in the
// group of its version's stream, whose members are itself, built from
// opCommit, and a quay image built from quayCommit.
func (fx *fixture) build(version, digest string, quayCommit, opCommit string) FBCBuild {
	n := len(fx.store.fbcs)
	f := FBCBuild{
		Group:       "quay-" + version[:strings.LastIndex(version, ".")],
		NVR:         fmt.Sprintf("quay-operator-fbc-%s-2026101400%04d.ocp4.16", version, n),
		Digest:      digest,
		StartTime:   fmt.Sprintf("2026-10-14T%02d:00:00", n),
		RenderState: "rendered",
	}
	fx.store.fbcs = append(fx.store.fbcs, f)
	quay := "sha256:quay-" + quayCommit
	fx.store.sources[digest] = []Source{{Name: "quay-operator-fbc", SourceRepo: opRepo, Commitish: opCommit}}
	fx.store.sources[quay] = []Source{{Name: "quay-quay", SourceRepo: quayRepo, Commitish: quayCommit}}
	fx.store.members[digest] = []string{quay, digest}
	return f
}

// last returns the build added last, to change it.
func (fx *fixture) last() *FBCBuild { return &fx.store.fbcs[len(fx.store.fbcs)-1] }

func (fx *fixture) run(t *testing.T) {
	t.Helper()
	if err := fx.sync.RunOnce(t.Context()); err != nil {
		t.Fatal(err)
	}
}

// uncertain returns the reasons logged for the FBC build nvr, "" for none.
func (fx *fixture) uncertain(nvr string) string {
	for line := range strings.Lines(fx.logs.String()) {
		if _, reasons, ok := strings.Cut(line, "msg=uncertain fbc="+nvr+" "); ok {
			_, reasons, _ = strings.Cut(reasons, "reasons=")
			return strings.Trim(strings.TrimSpace(reasons), `"`)
		}
	}
	return ""
}

func TestRunOnce(t *testing.T) {
	fx := newFixture(t)
	op := sha(100)
	fx.build("3.18.1", "sha256:f1", sha(1), op)
	f2 := fx.build("3.18.1", "sha256:f2", sha(2), op)
	fx.run(t)

	// The latest FBC is the baseline, reported for nothing.
	want := Baseline{Stream: "3.18", Operator: "quay-operator-fbc", OCP: "4.16", FBCDigest: "sha256:f2"}
	if got := fx.store.baselines[slot{"3.18", "quay-operator-fbc", "4.16"}]; got != want {
		t.Errorf("baseline = %+v; want %+v", got, want)
	}
	if len(fx.store.actions) != 0 || len(fx.commits.calls) != 0 {
		t.Errorf("first pass: actions %+v, fetches %v; want none", fx.store.actions, fx.commits.calls)
	}
	if want := map[[2]string]bool{{f2.NVR, "sha256:f2"}: true}; !reflect.DeepEqual(fx.store.runs, want) {
		t.Errorf("runs = %v; want only the baseline's", fx.store.runs)
	}

	f3 := fx.build("3.18.2", "sha256:f3", sha(3), op)
	fx.commits.ranges[quayRepo+" redhat-3.18 "+sha(2)+" "+sha(3)] = []Commit{
		{SHA: sha(3), Message: "PROJQUAY-1 fix\n\nSee PROJQUAY-1."},
		{SHA: sha(30), Message: "chore: bump (PROJQUAY-2)"},
		{SHA: sha(31), Message: "PROJQUAY-20 fix (cherry-pick)\n\nUpstream: PROJQUAY-21\n(cherry picked from commit " + sha(32) + ")"},
	}
	fx.run(t)
	for _, key := range []string{"PROJQUAY-1", "PROJQUAY-2", "PROJQUAY-20"} {
		a, ok := fx.store.actions[ActionKey{FBCDigest: "sha256:f3", IssueKey: key, Release: f3.NVR}]
		if !ok || a.Status != StatusProposed || a.DesiredTargetVersion != "quay-v3.18.2" || a.Assembly != "3.18.2" || a.FBCNVR != f3.NVR {
			t.Errorf("%s: %+v, %v; want proposed for quay-v3.18.2", key, a, ok)
		}
	}
	ev := fx.store.actions[ActionKey{FBCDigest: "sha256:f3", IssueKey: "PROJQUAY-1", Release: f3.NVR}].EvidenceJSON
	if want := `[{"component":"quay-quay","repo":"https://github.com/quay/quay","sha":"` + sha(3) + `"}]`; ev != want {
		t.Errorf("evidence = %s; want %s", ev, want)
	}
	// A key only in a commit body is not this branch's fix.
	if len(fx.store.actions) != 3 {
		t.Errorf("actions = %+v; want three", fx.store.actions)
	}

	// A respin whose range lists PROJQUAY-1 again inherits it.
	f4 := fx.build("3.18.2", "sha256:f4", sha(4), op)
	fx.commits.ranges[quayRepo+" redhat-3.18 "+sha(3)+" "+sha(4)] = []Commit{{SHA: sha(4), Message: "PROJQUAY-1 follow-up"}}
	fx.run(t)
	a := fx.store.actions[ActionKey{FBCDigest: "sha256:f4", IssueKey: "PROJQUAY-1", Release: f4.NVR}]
	if want := "already included in " + f3.NVR; a.Status != StatusSkipped || a.SkipReason != want || a.DesiredTargetVersion != "" {
		t.Errorf("inherited PROJQUAY-1 = %+v; want skipped %q", a, want)
	}
	if len(fx.store.actions) != 4 {
		t.Errorf("actions = %+v; want four", fx.store.actions)
	}

	// Done runs are not fetched or rewritten again.
	before := maps.Clone(fx.store.actions)
	fx.commits.calls = nil
	fx.run(t)
	if len(fx.commits.calls) != 0 || !reflect.DeepEqual(fx.store.actions, before) {
		t.Errorf("rerun: fetches %v, actions changed %v", fx.commits.calls, !reflect.DeepEqual(fx.store.actions, before))
	}

	// Each proposed action is logged once; the skipped one is not.
	logs := fx.logs.String()
	if n := strings.Count(logs, "msg=proposed "); n != 3 {
		t.Errorf("logged %d proposed actions; want 3:\n%s", n, logs)
	}
	if want := "msg=proposed issue=PROJQUAY-1 fbc=" + f3.NVR + " target_version_add=quay-v3.18.2 target_version_remove=quay-v3.18.z"; !strings.Contains(logs, want) {
		t.Errorf("logs lack %q:\n%s", want, logs)
	}
}

func TestRunOnceUncertain(t *testing.T) {
	fx := newFixture(t)
	fx.build("3.18.1", "sha256:f1", sha(1), sha(100))
	fx.run(t)

	f2 := fx.build("3.18.2", "sha256:f2", sha(2), sha(101))
	fx.store.members["sha256:f2"] = append(fx.store.members["sha256:f2"], "sha256:unknown")
	fx.commits.errs[quayRepo+" redhat-3.18 "+sha(1)+" "+sha(2)] = errors.New("head is not on redhat-3.18")
	fx.commits.ranges[opRepo+" redhat-3.18 "+sha(100)+" "+sha(101)] = []Commit{
		{SHA: sha(101), Message: "PROJQUAY-7: fix"},
		{SHA: sha(102), Message: "PROJQUAY-8: fix"},
		{SHA: sha(103), Message: "Revert \"PROJQUAY-8: fix\""},
	}
	fx.run(t)

	if a := fx.store.actions[ActionKey{FBCDigest: "sha256:f2", IssueKey: "PROJQUAY-7", Release: f2.NVR}]; a.Status != StatusProposed {
		t.Errorf("verified key = %+v; want proposed", a)
	}
	if got, want := fx.uncertain(f2.NVR), "PROJQUAY-8: reverted by "+sha(103)+"; component quay-quay: head is not on redhat-3.18; component sha256:unknown: no ART source"; got != want {
		t.Errorf("uncertain = %q; want %q", got, want)
	}
	if len(fx.store.actions) != 1 {
		t.Errorf("actions = %+v; want one", fx.store.actions)
	}
}

// A build outside a quay-X.Y group or without an X.Y.Z version is skipped;
// one with more than one ART name has no slot.
func TestRunOnceUnslotted(t *testing.T) {
	fx := newFixture(t)
	other := fx.build("3.18.1", "sha256:other", sha(1), sha(100))
	fx.last().Group = "openshift-4.16"
	fx.build("3.18.1", "sha256:nover", sha(1), sha(100))
	fx.last().NVR = "quay-operator-fbc-2026101400001.ocp4.16"
	noVersion := *fx.last()
	two := fx.build("3.18.1", "sha256:two", sha(1), sha(100))
	fx.store.sources["sha256:two"] = append(fx.store.sources["sha256:two"], Source{Name: "quay-operator-bundle", SourceRepo: opRepo, Commitish: sha(100)})
	fx.run(t)

	if len(fx.store.baselines) != 0 || len(fx.store.actions) != 0 {
		t.Fatalf("baselines %+v, actions %+v; want none", fx.store.baselines, fx.store.actions)
	}
	if u := fx.uncertain(two.NVR); !strings.Contains(u, "2 ART names") {
		t.Errorf("uncertain = %q; want 2 ART names", u)
	}
	if want := map[[2]string]bool{{two.NVR, "sha256:two"}: true}; !reflect.DeepEqual(fx.store.runs, want) {
		t.Errorf("runs = %v; want only %s", fx.store.runs, two.NVR)
	}
	for _, f := range []FBCBuild{other, noVersion} {
		if u := fx.uncertain(f.NVR); u != "" {
			t.Errorf("skipped %s logged uncertain %q", f.NVR, u)
		}
	}
}

func TestRunOnceUnchangedMember(t *testing.T) {
	fx := newFixture(t)
	fx.build("3.18.1", "sha256:f1", sha(1), sha(100))
	fx.store.members["sha256:f1"] = append(fx.store.members["sha256:f1"], "sha256:ext1")
	fx.run(t)

	// An external image carried over unchanged is not uncertain.
	f2 := fx.build("3.18.2", "sha256:f2", sha(2), sha(100))
	fx.store.members["sha256:f2"] = append(fx.store.members["sha256:f2"], "sha256:ext1")
	fx.run(t)
	if len(fx.store.actions) != 0 {
		t.Errorf("actions = %+v; want none", fx.store.actions)
	}

	f3 := fx.build("3.18.3", "sha256:f3", sha(3), sha(100))
	fx.store.members["sha256:f3"] = append(fx.store.members["sha256:f3"], "sha256:ext2")
	fx.run(t)
	if u := fx.uncertain(f2.NVR); u != "" {
		t.Errorf("f2 uncertain: %q; want none", u)
	}
	if got, want := fx.uncertain(f3.NVR), "component sha256:ext2: no ART source"; got != want {
		t.Errorf("uncertain = %q; want %q", got, want)
	}
}

func TestRunOnceAwaitsProvenance(t *testing.T) {
	fx := newFixture(t)
	op := sha(100)
	// The FBC's own record is not checked yet: its stream waits.
	fx.build("3.18.1", "sha256:f1", sha(1), op)
	fbcSrc := fx.store.sources["sha256:f1"]
	delete(fx.store.sources, "sha256:f1")
	fx.store.unchecked["sha256:f1"] = true
	fx.run(t)
	if len(fx.store.baselines) != 0 || len(fx.store.actions) != 0 {
		t.Fatalf("unchecked FBC: baselines %+v, actions %+v; want none", fx.store.baselines, fx.store.actions)
	}
	fx.store.sources["sha256:f1"], fx.store.unchecked["sha256:f1"] = fbcSrc, false
	fx.run(t)
	if len(fx.store.baselines) != 1 {
		t.Fatalf("baselines %+v; want f1's", fx.store.baselines)
	}

	// A changed member's record is not checked yet: it and the FBC after it wait.
	f2 := fx.build("3.18.2", "sha256:f2", sha(2), op)
	quay2 := "sha256:quay-" + sha(2)
	quaySrc := fx.store.sources[quay2]
	delete(fx.store.sources, quay2)
	fx.store.unchecked[quay2] = true
	fx.commits.ranges[quayRepo+" redhat-3.18 "+sha(1)+" "+sha(2)] = []Commit{{SHA: sha(2), Message: "PROJQUAY-7 fix"}}
	fx.run(t)
	f3 := fx.build("3.18.3", "sha256:f3", sha(3), op)
	fx.commits.ranges[quayRepo+" redhat-3.18 "+sha(2)+" "+sha(3)] = []Commit{{SHA: sha(3), Message: "PROJQUAY-8 fix"}}
	fx.run(t)
	if len(fx.store.actions) != 0 || len(fx.commits.calls) != 0 {
		t.Fatalf("unchecked member: actions %+v, fetches %v; want none", fx.store.actions, fx.commits.calls)
	}
	fx.store.sources[quay2], fx.store.unchecked[quay2] = quaySrc, false
	fx.run(t)
	for k, want := range map[ActionKey]string{
		{FBCDigest: "sha256:f2", IssueKey: "PROJQUAY-7", Release: f2.NVR}: "quay-v3.18.2",
		{FBCDigest: "sha256:f3", IssueKey: "PROJQUAY-8", Release: f3.NVR}: "quay-v3.18.3",
	} {
		if a := fx.store.actions[k]; a.Status != StatusProposed || a.DesiredTargetVersion != want {
			t.Errorf("%v: %+v; want proposed for %s", k, a, want)
		}
	}
	if len(fx.store.actions) != 2 {
		t.Errorf("actions = %+v; want two", fx.store.actions)
	}

	// A member with no record waits provenanceWait after the build started,
	// a UTC time with no zone.
	f4 := fx.build("3.18.4", "sha256:f4", sha(3), op)
	fx.last().StartTime = fx.clock.Format("2006-01-02T15:04:05")
	fx.store.members["sha256:f4"] = append(fx.store.members["sha256:f4"], "sha256:ext")
	fx.run(t)
	if u := fx.uncertain(f4.NVR); u != "" || fx.store.runs[[2]string{f4.NVR, "sha256:f4"}] {
		t.Fatalf("within the wait: uncertain %q, done %v; want neither", u, fx.store.runs[[2]string{f4.NVR, "sha256:f4"}])
	}
	fx.clock = fx.clock.Add(provenanceWait)
	fx.run(t)
	if got, want := fx.uncertain(f4.NVR), "component sha256:ext: no ART source"; got != want {
		t.Errorf("after the wait: uncertain %q; want %q", got, want)
	}
}

func TestRunOnceHeldStream(t *testing.T) {
	fx := newFixture(t)
	fx.build("3.18.1", "sha256:f1", sha(1), sha(100))
	fx.run(t)
	// f2 has no slot until its record is checked; f3, in the same stream, waits behind it.
	f2 := fx.build("3.18.2", "sha256:f2", sha(2), sha(100))
	fbcSrc := fx.store.sources["sha256:f2"]
	delete(fx.store.sources, "sha256:f2")
	fx.store.unchecked["sha256:f2"] = true
	fx.build("3.18.3", "sha256:f3", sha(3), sha(100))
	fx.commits.ranges[quayRepo+" redhat-3.18 "+sha(1)+" "+sha(2)] = []Commit{{SHA: sha(2), Message: "PROJQUAY-7 fix"}}
	fx.run(t)
	fx.run(t)
	if len(fx.store.actions) != 0 || len(fx.commits.calls) != 0 {
		t.Fatalf("held: actions %+v, fetches %v; want none", fx.store.actions, fx.commits.calls)
	}
	if n := strings.Count(fx.logs.String(), "msg=waiting"); n != 1 {
		t.Errorf("logged the wait %d times; want once:\n%s", n, fx.logs.String())
	}
	fx.store.sources["sha256:f2"], fx.store.unchecked["sha256:f2"] = fbcSrc, false
	fx.run(t)
	if a := fx.store.actions[ActionKey{FBCDigest: "sha256:f2", IssueKey: "PROJQUAY-7", Release: f2.NVR}]; a.Status != StatusProposed || a.DesiredTargetVersion != "quay-v3.18.2" {
		t.Errorf("PROJQUAY-7 = %+v; want proposed on f2", a)
	}
}

func TestRunOnceAwaitsPrevMember(t *testing.T) {
	fx := newFixture(t)
	fx.build("3.18.1", "sha256:f1", sha(1), sha(100))
	fx.run(t)
	// f1's quay image, replaced in f2, has an unchecked record.
	quay1 := "sha256:quay-" + sha(1)
	quaySrc := fx.store.sources[quay1]
	delete(fx.store.sources, quay1)
	fx.store.unchecked[quay1] = true
	f2 := fx.build("3.18.2", "sha256:f2", sha(2), sha(100))
	fx.commits.ranges[quayRepo+" redhat-3.18 "+sha(1)+" "+sha(2)] = []Commit{{SHA: sha(2), Message: "PROJQUAY-7 fix"}}
	fx.run(t)
	if len(fx.store.actions) != 0 || len(fx.commits.calls) != 0 {
		t.Fatalf("unchecked prev member: actions %+v, fetches %v; want none", fx.store.actions, fx.commits.calls)
	}
	fx.store.sources[quay1], fx.store.unchecked[quay1] = quaySrc, false
	fx.run(t)
	if a := fx.store.actions[ActionKey{FBCDigest: "sha256:f2", IssueKey: "PROJQUAY-7", Release: f2.NVR}]; a.Status != StatusProposed {
		t.Errorf("PROJQUAY-7 = %+v; want proposed", a)
	}
}

func TestRunOnceLateFBC(t *testing.T) {
	fx := newFixture(t)
	fx.build("3.18.1", "sha256:f1", sha(1), sha(100))
	fx.run(t)
	f2 := fx.build("3.18.2", "sha256:f2", sha(2), sha(100))
	f3 := fx.build("3.18.3", "sha256:f3", sha(3), sha(100))
	// f2 is listed late, after f3 was reconciled against f1.
	fx.store.fbcs = slices.Delete(fx.store.fbcs, 1, 2)
	fx.run(t)
	fx.store.fbcs = slices.Insert(fx.store.fbcs, 1, f2)
	fx.run(t)
	if got, want := fx.uncertain(f2.NVR), "built before "+f3.NVR+", which was reconciled first"; got != want {
		t.Errorf("late FBC: uncertain %q; want %q", got, want)
	}
}

func TestRunOnceGap(t *testing.T) {
	for name, unslot := range map[string]func(*fixture){
		"no slot": func(fx *fixture) {
			d := fx.last().Digest
			fx.store.sources[d] = append(fx.store.sources[d], Source{Name: "quay-operator-bundle", SourceRepo: opRepo, Commitish: sha(100)})
		},
		"render failed": func(fx *fixture) { fx.last().RenderState = "failed" },
	} {
		t.Run(name, func(t *testing.T) {
			fx := newFixture(t)
			op := sha(100)
			fx.build("3.18.1", "sha256:f1", sha(1), op)
			fx.run(t)
			// f3's slot predecessor is f1, so its diff would take f2's PROJQUAY-2.
			f2 := fx.build("3.18.2", "sha256:f2", sha(2), op)
			unslot(fx)
			f3 := fx.build("3.18.3", "sha256:f3", sha(3), op)
			fx.commits.ranges[quayRepo+" redhat-3.18 "+sha(1)+" "+sha(3)] = []Commit{{SHA: sha(2), Message: "PROJQUAY-2 fix"}}
			fx.run(t)
			if u := fx.uncertain(f2.NVR); u == "" {
				t.Error("f2 not uncertain")
			}
			if got, want := fx.uncertain(f3.NVR), "follows "+f2.NVR+", which has no slot"; got != want {
				t.Errorf("f3 uncertain %q; want %q", got, want)
			}
			if len(fx.store.actions) != 0 || len(fx.commits.calls) != 0 {
				t.Errorf("actions %+v, fetches %v; want none", fx.store.actions, fx.commits.calls)
			}

			// The FBC after f3 is diffed against it.
			f4 := fx.build("3.18.4", "sha256:f4", sha(4), op)
			fx.commits.ranges[quayRepo+" redhat-3.18 "+sha(3)+" "+sha(4)] = []Commit{{SHA: sha(4), Message: "PROJQUAY-4 fix"}}
			fx.run(t)
			if a := fx.store.actions[ActionKey{FBCDigest: "sha256:f4", IssueKey: "PROJQUAY-4", Release: f4.NVR}]; a.Status != StatusProposed || a.DesiredTargetVersion != "quay-v3.18.4" {
				t.Errorf("PROJQUAY-4 = %+v; want proposed for quay-v3.18.4", a)
			}
		})
	}
}

// ART builds an FBC per OCP version: a gap reaches only the next FBC of its
// own operator and OCP.
func TestRunOnceGapSlots(t *testing.T) {
	// The failed 4.16 build is listed before or after its 4.17 sibling.
	for _, failedFirst := range []bool{true, false} {
		fx := newFixture(t)
		op := sha(100)
		add := func(version, digest string, n int, ocp string) FBCBuild {
			fx.build(version, digest, sha(n), op)
			f := fx.last()
			f.NVR = strings.Replace(f.NVR, ".ocp4.16", ".ocp"+ocp, 1)
			return *f
		}
		add("3.18.1", "sha256:a1", 1, "4.16")
		add("3.18.1", "sha256:b1", 1, "4.17")
		fx.run(t)
		var b2 FBCBuild
		if !failedFirst {
			b2 = add("3.18.2", "sha256:b2", 2, "4.17")
		}
		a2 := add("3.18.2", "sha256:a2", 2, "4.16")
		fx.last().RenderState = "failed"
		if failedFirst {
			b2 = add("3.18.2", "sha256:b2", 2, "4.17")
		}
		a3 := add("3.18.3", "sha256:a3", 3, "4.16")
		b3 := add("3.18.3", "sha256:b3", 3, "4.17")
		fx.commits.ranges[quayRepo+" redhat-3.18 "+sha(1)+" "+sha(2)] = []Commit{{SHA: sha(2), Message: "PROJQUAY-2 fix"}}
		fx.commits.ranges[quayRepo+" redhat-3.18 "+sha(1)+" "+sha(3)] = []Commit{{SHA: sha(2), Message: "PROJQUAY-2 fix"}}
		fx.commits.ranges[quayRepo+" redhat-3.18 "+sha(2)+" "+sha(3)] = []Commit{{SHA: sha(3), Message: "PROJQUAY-3 fix"}}
		fx.run(t)
		if got, want := fx.uncertain(a3.NVR), "follows "+a2.NVR+", which has no slot"; got != want {
			t.Errorf("failedFirst=%v: a3 uncertain %q; want %q", failedFirst, got, want)
		}
		for _, f := range []FBCBuild{b2, b3} {
			if u := fx.uncertain(f.NVR); u != "" {
				t.Errorf("failedFirst=%v: %s uncertain %q; want none", failedFirst, f.NVR, u)
			}
		}
		for k, want := range map[ActionKey]string{
			{FBCDigest: "sha256:b2", IssueKey: "PROJQUAY-2", Release: b2.NVR}: "quay-v3.18.2",
			{FBCDigest: "sha256:b3", IssueKey: "PROJQUAY-3", Release: b3.NVR}: "quay-v3.18.3",
		} {
			if a := fx.store.actions[k]; a.Status != StatusProposed || a.DesiredTargetVersion != want {
				t.Errorf("failedFirst=%v: %v: %+v; want proposed for %s", failedFirst, k, a, want)
			}
		}
		if len(fx.store.actions) != 2 {
			t.Errorf("failedFirst=%v: actions = %+v; want b2 and b3 proposed", failedFirst, fx.store.actions)
		}
	}
}

func TestRunOnceUnrendered(t *testing.T) {
	fx := newFixture(t)
	op := sha(100)
	fx.build("3.18.1", "sha256:f1", sha(1), op)
	fx.run(t)
	// f2 is not rendered yet: f3, in the same stream, waits behind it.
	f2 := fx.build("3.18.2", "sha256:f2", sha(2), op)
	fx.last().RenderState = ""
	f3 := fx.build("3.18.3", "sha256:f3", sha(3), op)
	fx.commits.ranges[quayRepo+" redhat-3.18 "+sha(1)+" "+sha(2)] = []Commit{{SHA: sha(2), Message: "PROJQUAY-7 fix"}}
	fx.commits.ranges[quayRepo+" redhat-3.18 "+sha(2)+" "+sha(3)] = []Commit{{SHA: sha(3), Message: "PROJQUAY-8 fix"}}
	fx.run(t)
	if len(fx.store.actions) != 0 || len(fx.commits.calls) != 0 {
		t.Fatalf("unrendered: actions %+v, fetches %v; want none", fx.store.actions, fx.commits.calls)
	}
	fx.store.fbcs[1].RenderState = "rendered"
	fx.run(t)
	for k, want := range map[ActionKey]string{
		{FBCDigest: "sha256:f2", IssueKey: "PROJQUAY-7", Release: f2.NVR}: "quay-v3.18.2",
		{FBCDigest: "sha256:f3", IssueKey: "PROJQUAY-8", Release: f3.NVR}: "quay-v3.18.3",
	} {
		if a := fx.store.actions[k]; a.Status != StatusProposed || a.DesiredTargetVersion != want {
			t.Errorf("%v: %+v; want proposed for %s", k, a, want)
		}
	}
	if len(fx.store.actions) != 2 {
		t.Errorf("actions = %+v; want two", fx.store.actions)
	}
}

// An unrendered build a later one of its version supersedes is skipped
// unless staged, which holds the stream until it renders.
func TestRunOnceSuperseded(t *testing.T) {
	for _, staged := range []bool{false, true} {
		fx := newFixture(t)
		op := sha(100)
		fx.build("3.18.1", "sha256:f1", sha(1), op)
		fx.run(t)
		fx.build("3.18.2", "sha256:f2", sha(2), op)
		fx.last().RenderState, fx.last().Staged = "", staged
		f3 := fx.build("3.18.2", "sha256:f3", sha(3), op)
		fx.commits.ranges[quayRepo+" redhat-3.18 "+sha(1)+" "+sha(3)] = []Commit{{SHA: sha(2), Message: "PROJQUAY-7 fix"}}
		fx.run(t)
		a, ok := fx.store.actions[ActionKey{FBCDigest: "sha256:f3", IssueKey: "PROJQUAY-7", Release: f3.NVR}]
		if staged {
			if len(fx.store.actions) != 0 || len(fx.commits.calls) != 0 {
				t.Errorf("staged: actions %+v, fetches %v; want none", fx.store.actions, fx.commits.calls)
			}
			continue
		}
		if !ok || a.Status != StatusProposed || a.DesiredTargetVersion != "quay-v3.18.2" || len(fx.store.actions) != 1 {
			t.Errorf("superseded: actions %+v; want PROJQUAY-7 proposed on f3 for quay-v3.18.2", fx.store.actions)
		}
	}
}

// A transient GitHub error leaves the FBC to the next pass; a permanent one
// is its uncertainty.
func TestRunOnceGitHubError(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		retry  bool
	}{
		{"unavailable", http.StatusServiceUnavailable, true},
		{"rate limited", http.StatusTooManyRequests, true},
		{"not found", http.StatusNotFound, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx := newFixture(t)
			g := newFake(t)
			fx.sync.commits = g.start(t, "")
			op := sha(100)
			fx.build("3.18.1", "sha256:f1", base, op)
			fx.run(t)
			f2 := fx.build("3.18.2", "sha256:f2", head, op)
			g.fail = func(w http.ResponseWriter) { w.WriteHeader(tc.status) }
			fx.run(t)
			done := fx.store.runs[[2]string{f2.NVR, "sha256:f2"}]
			if len(fx.store.actions) != 0 || done == tc.retry {
				t.Fatalf("GitHub %d: actions %+v, done %v; want none, done %v", tc.status, fx.store.actions, done, !tc.retry)
			}
			if u := fx.uncertain(f2.NVR); (u == "") != tc.retry {
				t.Errorf("GitHub %d: uncertain %q", tc.status, u)
			}

			g.fail = nil
			fx.run(t)
			if !tc.retry {
				if len(fx.store.actions) != 0 {
					t.Errorf("done FBC proposed %+v", fx.store.actions)
				}
				return
			}
			for _, key := range []string{"PROJQUAY-1", "PROJQUAY-2"} {
				if a := fx.store.actions[ActionKey{FBCDigest: "sha256:f2", IssueKey: key, Release: f2.NVR}]; a.Status != StatusProposed || a.DesiredTargetVersion != "quay-v3.18.2" {
					t.Errorf("retry: %s = %+v; want proposed for quay-v3.18.2", key, a)
				}
			}
		})
	}
}

func (f *fakeCommits) Branch(_ context.Context, repo, prefix, commit string) (string, string, error) {
	k := strings.Join([]string{repo, prefix, commit}, " ")
	f.calls = append(f.calls, k)
	if err := f.errs[k]; err != nil {
		return "", "", err
	}
	if b, ok := f.branches[k]; ok {
		return b, "", nil
	}
	return "", "on no branch", nil
}

// A stream no list names baselines on first sight, then reconciles. The
// stream is the group's, as for an EUS build of assembly stream.
func TestRunOnceNewStream(t *testing.T) {
	fx := newFixture(t)
	op := sha(100)
	fx.build("3.10.28", "sha256:f1", sha(1), op)
	fx.run(t)
	if _, ok := fx.store.baselines[slot{"3.10", "quay-operator-fbc", "4.16"}]; !ok || len(fx.store.actions) != 0 || len(fx.commits.calls) != 0 {
		t.Fatalf("first pass: baselines %+v, actions %+v, fetches %v; want a 3.10 baseline only", fx.store.baselines, fx.store.actions, fx.commits.calls)
	}

	f2 := fx.build("3.10.28", "sha256:f2", sha(2), op)
	fx.commits.ranges[quayRepo+" redhat-3.10 "+sha(1)+" "+sha(2)] = []Commit{{SHA: sha(2), Message: "PROJQUAY-1 fix"}}
	fx.run(t)
	if a := fx.store.actions[ActionKey{FBCDigest: "sha256:f2", IssueKey: "PROJQUAY-1", Release: f2.NVR}]; a.Status != StatusProposed || a.DesiredTargetVersion != "quay-v3.10.28" {
		t.Errorf("PROJQUAY-1 = %+v; want proposed for quay-v3.10.28", a)
	}
}

// A slot whose baseline is a build ART staged before builds were the input
// reconciles only the builds after it.
func TestRunOnceExistingBaseline(t *testing.T) {
	fx := newFixture(t)
	op := sha(100)
	fx.store.baselines[slot{"3.18", "quay-operator-fbc", "4.16"}] = Baseline{Stream: "3.18", Operator: "quay-operator-fbc", OCP: "4.16", FBCDigest: "sha256:f2"}
	fx.build("3.18.1", "sha256:f1", sha(1), op)
	fx.build("3.18.1", "sha256:f2", sha(2), op)
	fx.last().Staged = true
	f3 := fx.build("3.18.2", "sha256:f3", sha(3), op)
	fx.commits.ranges[quayRepo+" redhat-3.18 "+sha(1)+" "+sha(2)] = []Commit{{SHA: sha(2), Message: "PROJQUAY-1 fix"}}
	fx.commits.ranges[quayRepo+" redhat-3.18 "+sha(2)+" "+sha(3)] = []Commit{{SHA: sha(3), Message: "PROJQUAY-2 fix"}}
	fx.run(t)
	if a := fx.store.actions[ActionKey{FBCDigest: "sha256:f3", IssueKey: "PROJQUAY-2", Release: f3.NVR}]; a.Status != StatusProposed || a.DesiredTargetVersion != "quay-v3.18.2" || len(fx.store.actions) != 1 {
		t.Errorf("actions = %+v; want only PROJQUAY-2 proposed on f3", fx.store.actions)
	}
	if slices.ContainsFunc(fx.commits.calls, func(c string) bool { return strings.Contains(c, sha(1)) }) {
		t.Errorf("fetches = %v; want none from f1", fx.commits.calls)
	}
	if fx.store.runs[[2]string{fx.store.fbcs[0].NVR, "sha256:f1"}] {
		t.Error("f1, before the baseline, marked run")
	}
}

// buildClair adds an FBC build whose quay image is unchanged and whose
// Clair image is built from clairCommit.
func (fx *fixture) buildClair(digest, clairCommit string) FBCBuild {
	f := fx.build("3.18.1", digest, sha(1), sha(100))
	img := "sha256:clair-" + clairCommit
	fx.store.sources[img] = []Source{{Name: "quay-clair", SourceRepo: clairRepo, Commitish: clairCommit}}
	fx.store.members[digest] = append(fx.store.members[digest], img)
	return f
}

const clairRepo = "https://github.com/quay/clair"

func TestRunOnceClairBranch(t *testing.T) {
	for _, tc := range []struct {
		name      string
		prev, cur string
		reason    string
	}{
		{"same branch", "release-4.9", "release-4.9", ""},
		{"diverged", "release-4.8", "release-4.8-lts", "previous commit " + sha(5) + " is on release-4.8, current " + sha(6) + " on release-4.8-lts"},
		{"unresolved", "release-4.9", "", "component quay-clair: on no branch"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx := newFixture(t)
			fx.commits.branches = map[string]string{clairRepo + " release- " + sha(5): tc.prev}
			if tc.cur != "" {
				fx.commits.branches[clairRepo+" release- "+sha(6)] = tc.cur
			}
			fx.buildClair("sha256:f1", sha(5))
			fx.run(t)
			f2 := fx.buildClair("sha256:f2", sha(6))
			fx.commits.ranges[clairRepo+" release-4.9 "+sha(5)+" "+sha(6)] = []Commit{{SHA: sha(6), Message: "PROJQUAY-9 fix"}}
			fx.run(t)

			_, proposed := fx.store.actions[ActionKey{FBCDigest: "sha256:f2", IssueKey: "PROJQUAY-9", Release: f2.NVR}]
			if proposed != (tc.reason == "") || !strings.Contains(fx.uncertain(f2.NVR), tc.reason) {
				t.Errorf("proposed %v, uncertain %q; want reason %q", proposed, fx.uncertain(f2.NVR), tc.reason)
			}
			if !fx.store.runs[[2]string{f2.NVR, "sha256:f2"}] {
				t.Error("f2 not marked run")
			}
		})
	}
}

// A transient GitHub error resolving a branch makes the FBC wait for the
// next pass; any other makes it uncertain.
func TestRunOnceClairBranchError(t *testing.T) {
	fx := newFixture(t)
	fx.commits.branches = map[string]string{clairRepo + " release- " + sha(5): "release-4.9", clairRepo + " release- " + sha(6): "release-4.9", clairRepo + " release- " + sha(7): "release-4.9"}
	fx.buildClair("sha256:f1", sha(5))
	fx.run(t)
	f2 := fx.buildClair("sha256:f2", sha(6))
	k := clairRepo + " release- " + sha(6)
	fx.commits.errs[k] = fmt.Errorf("%w: 503", errTransient)
	fx.run(t)
	if fx.store.runs[[2]string{f2.NVR, "sha256:f2"}] || fx.uncertain(f2.NVR) != "" {
		t.Fatalf("f2 marked run or uncertain after a transient GitHub error")
	}
	delete(fx.commits.errs, k)
	fx.commits.ranges[clairRepo+" release-4.9 "+sha(5)+" "+sha(6)] = []Commit{{SHA: sha(6), Message: "PROJQUAY-9 fix"}}
	fx.run(t)
	if a := fx.store.actions[ActionKey{FBCDigest: "sha256:f2", IssueKey: "PROJQUAY-9", Release: f2.NVR}]; a.Status != StatusProposed {
		t.Errorf("retry: PROJQUAY-9 = %+v; want proposed", a)
	}

	f3 := fx.buildClair("sha256:f3", sha(7))
	fx.commits.errs[clairRepo+" release- "+sha(7)] = errors.New("GitHub 404 Not Found")
	fx.run(t)
	if !fx.store.runs[[2]string{f3.NVR, "sha256:f3"}] || !strings.Contains(fx.uncertain(f3.NVR), "component quay-clair: GitHub 404 Not Found") {
		t.Errorf("f3 run %v, uncertain %q; want run and uncertain", fx.store.runs[[2]string{f3.NVR, "sha256:f3"}], fx.uncertain(f3.NVR))
	}
}

// A build reconciled before actions were keyed by NVR, under its Konflux
// Release name, is not proposed again.
func TestRunOnceReleaseKeyed(t *testing.T) {
	fx := newFixture(t)
	op := sha(100)
	fx.build("3.18.1", "sha256:f1", sha(1), op)
	fx.run(t)
	f2 := fx.build("3.18.2", "sha256:f2", sha(2), op)
	fx.store.runs[[2]string{"quay-fbc-stage-abc", "sha256:f2"}] = true
	old := ActionKey{FBCDigest: "sha256:f2", IssueKey: "PROJQUAY-1", Release: "quay-fbc-stage-abc"}
	fx.store.actions[old] = Action{Key: old, Assembly: "3.18.2", Status: StatusApplied, DesiredTargetVersion: "quay-v3.18.2"}
	fx.commits.ranges[quayRepo+" redhat-3.18 "+sha(1)+" "+sha(2)] = []Commit{{SHA: sha(2), Message: "PROJQUAY-1 fix"}}
	fx.run(t)

	if a := fx.store.actions[ActionKey{FBCDigest: "sha256:f2", IssueKey: "PROJQUAY-1", Release: f2.NVR}]; a.Status != StatusSkipped {
		t.Errorf("PROJQUAY-1 = %+v; want skipped", a)
	}
}
