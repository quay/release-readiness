package reconcile

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

const (
	repo   = "https://github.com/quay/quay"
	branch = "redhat-3.18"
	base   = "1111111111111111111111111111111111111111"
	head   = "4444444444444444444444444444444444444444"
)

// fakeGitHub serves the testdata compare pages for base...head and
// branchStatus for head...branch.
type fakeGitHub struct {
	pages        []map[string]any
	branchStatus string
	// fail, when set, answers every request.
	fail     func(http.ResponseWriter)
	requests int
	auth     []string
}

func newFake(t *testing.T) *fakeGitHub {
	t.Helper()
	g := &fakeGitHub{branchStatus: "ahead"}
	for _, name := range []string{"compare-page1.json", "compare-page2.json"} {
		data, err := os.ReadFile("testdata/" + name)
		if err != nil {
			t.Fatal(err)
		}
		var page map[string]any
		if err := json.Unmarshal(data, &page); err != nil {
			t.Fatal(err)
		}
		g.pages = append(g.pages, page)
	}
	return g
}

func (g *fakeGitHub) start(t *testing.T, token string) *GitHubFetcher {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		g.requests++
		g.auth = append(g.auth, r.Header.Get("Authorization"))
		if g.fail != nil {
			g.fail(w)
			return
		}
		switch r.URL.Path {
		case "/repos/quay/quay/compare/" + base + "..." + head:
			var page int
			_, _ = fmt.Sscan(r.URL.Query().Get("page"), &page)
			if page < 1 || page > len(g.pages) {
				http.NotFound(w, r)
				return
			}
			if page < len(g.pages) {
				next := fmt.Sprintf("http://%s%s?per_page=100&page=%d", r.Host, r.URL.Path, page+1)
				w.Header().Set("Link", fmt.Sprintf(`<%s>; rel="next", <%s>; rel="last"`, next, next))
			}
			_ = json.NewEncoder(w).Encode(g.pages[page-1])
		case "/repos/quay/quay/compare/" + head + "..." + branch:
			_ = json.NewEncoder(w).Encode(map[string]any{"status": g.branchStatus})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	f := NewGitHubFetcher(srv.Client(), token)
	f.baseURL = srv.URL
	return f
}

func TestRangeTwoPages(t *testing.T) {
	g := newFake(t)
	f := g.start(t, "secret")
	got, err := f.Range(context.Background(), repo, branch, base, head)
	if err != nil {
		t.Fatal(err)
	}
	want := []Commit{
		{"2222222222222222222222222222222222222222", "PROJQUAY-1: fix the thing (#1)\n\nBody."},
		{"3333333333333333333333333333333333333333", "chore: bump deps (#2)"},
		{head, "PROJQUAY-2: another fix (#3)"},
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("Range = %q, want %q", got, want)
	}
	if g.requests != 3 {
		t.Errorf("%d requests, want 2 pages and 1 branch check", g.requests)
	}
	for _, a := range g.auth {
		if a != "Bearer secret" {
			t.Errorf("Authorization %q, want Bearer secret", a)
		}
	}
}

func TestRangeCached(t *testing.T) {
	g := newFake(t)
	f := g.start(t, "")
	for range 2 {
		if _, err := f.Range(context.Background(), repo, branch, base, head); err != nil {
			t.Fatal(err)
		}
	}
	if g.requests != 3 {
		t.Errorf("%d requests, want 3 from the first Range only", g.requests)
	}
}

func TestRangeUnauthenticated(t *testing.T) {
	g := newFake(t)
	f := g.start(t, "")
	if _, err := f.Range(context.Background(), repo, branch, base, head); err != nil {
		t.Fatal(err)
	}
	for _, a := range g.auth {
		if a != "" {
			t.Errorf("Authorization %q, want none", a)
		}
	}
}

func TestRangeFailsClosed(t *testing.T) {
	status := func(code int, header ...string) func(*fakeGitHub) {
		return func(g *fakeGitHub) {
			g.fail = func(w http.ResponseWriter) {
				for i := 0; i < len(header); i += 2 {
					w.Header().Set(header[i], header[i+1])
				}
				w.WriteHeader(code)
			}
		}
	}
	for _, tc := range []struct {
		name      string
		setup     func(*fakeGitHub)
		err       string
		transient bool
	}{
		{"total mismatch", func(g *fakeGitHub) { g.pages[1]["total_commits"] = 4 }, "total_commits is 4", false},
		{"diverged", func(g *fakeGitHub) { g.pages[0]["behind_by"] = 1 }, "diverged", false},
		{"merge base mismatch", func(g *fakeGitHub) {
			g.pages[0]["merge_base_commit"] = map[string]any{"sha": strings.Repeat("9", 40)}
		}, "merge base is 9999", false},
		{"last commit not head", func(g *fakeGitHub) {
			g.pages[1]["commits"].([]any)[0].(map[string]any)["sha"] = strings.Repeat("5", 40)
		}, "not head", false},
		{"head not on branch", func(g *fakeGitHub) { g.branchStatus = "diverged" }, "not on branch", false},
		{"not found", status(http.StatusNotFound), "404", false},
		{"forbidden", status(http.StatusForbidden), "403", false},
		{"rate limited", status(http.StatusTooManyRequests), "429", true},
		{"rate limit exhausted", status(http.StatusForbidden, "X-RateLimit-Remaining", "0"), "403", true},
		{"secondary rate limit", status(http.StatusForbidden, "Retry-After", "60"), "403", true},
		{"unavailable", status(http.StatusServiceUnavailable), "503", true},
		{"connection dropped", func(g *fakeGitHub) {
			g.fail = func(http.ResponseWriter) { panic(http.ErrAbortHandler) }
		}, "EOF", true},
		{"connection dropped mid-body", func(g *fakeGitHub) {
			g.fail = func(w http.ResponseWriter) {
				_, _ = w.Write([]byte(`{"status": "ahead", "commits": [`))
				w.(http.Flusher).Flush()
				panic(http.ErrAbortHandler)
			}
		}, "decode", true},
		{"malformed body", func(g *fakeGitHub) {
			g.fail = func(w http.ResponseWriter) { _, _ = w.Write([]byte("<html>proxy</html>")) }
		}, "invalid character", false},
	} {
		g := newFake(t)
		tc.setup(g)
		f := g.start(t, "")
		got, err := f.Range(context.Background(), repo, branch, base, head)
		if err == nil || !strings.Contains(err.Error(), tc.err) {
			t.Errorf("%s: error %v, want %q", tc.name, err, tc.err)
		}
		if errors.Is(err, errTransient) != tc.transient {
			t.Errorf("%s: transient %v, want %v", tc.name, !tc.transient, tc.transient)
		}
		if got != nil {
			t.Errorf("%s: partial result %v", tc.name, got)
		}
	}
}

func TestRangeNoRequest(t *testing.T) {
	for _, tc := range []struct {
		name, repo, base, head, err string
	}{
		{"unsupported host", "https://gitlab.com/quay/quay", base, head, "unsupported host"},
		{"not a repo", "https://github.com/quay/quay/tree/master", base, head, "unsupported host"},
		{"short sha", repo, "1111111", head, "full commit SHAs"},
		{"base is head", repo, head, head, ""},
	} {
		g := newFake(t)
		f := g.start(t, "")
		got, err := f.Range(context.Background(), tc.repo, branch, tc.base, tc.head)
		if tc.err == "" {
			if err != nil || got == nil || len(got) != 0 {
				t.Errorf("%s: Range = %v, %v; want empty, nil", tc.name, got, err)
			}
		} else if err == nil || !strings.Contains(err.Error(), tc.err) {
			t.Errorf("%s: error %v, want %q", tc.name, err, tc.err)
		}
		if g.requests != 0 {
			t.Errorf("%s: %d requests, want none", tc.name, g.requests)
		}
	}
}

func TestBranch(t *testing.T) {
	const clair = "https://github.com/quay/clair"
	commit := sha(7)
	for _, tc := range []struct {
		name   string
		heads  []string
		status map[string]string
		branch string
		reason string
	}{
		{"exact head", []string{"master", "release-4.9"}, nil, "release-4.9", ""},
		{"two heads", []string{"release-4.8", "release-4.8-lts"}, nil, "", "on 2 release-* branches [release-4.8 release-4.8-lts]"},
		{"moved head", nil, map[string]string{"release-4.8": "diverged", "release-4.9": "ahead"}, "release-4.9", ""},
		{"two containing", nil, map[string]string{"release-4.8": "ahead", "release-4.9": "ahead"}, "", "on 2 release-* branches"},
		{"none containing", nil, map[string]string{"release-4.8": "behind", "release-4.9": "diverged"}, "", "on 0 release-* branches"},
	} {
		requests := 0
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requests++
			api := "/repos/quay/clair/"
			switch {
			case r.URL.Path == api+"commits/"+commit+"/branches-where-head":
				heads := []map[string]any{}
				for _, h := range tc.heads {
					heads = append(heads, map[string]any{"name": h})
				}
				_ = json.NewEncoder(w).Encode(heads)
			case r.URL.Path == api+"git/matching-refs/heads/release-":
				_ = json.NewEncoder(w).Encode([]map[string]any{{"ref": "refs/heads/release-4.8"}, {"ref": "refs/heads/release-4.9"}})
			case strings.HasPrefix(r.URL.Path, api+"compare/"+commit+"..."):
				_ = json.NewEncoder(w).Encode(map[string]any{"status": tc.status[strings.TrimPrefix(r.URL.Path, api+"compare/"+commit+"...")]})
			default:
				http.NotFound(w, r)
			}
		}))
		f := NewGitHubFetcher(srv.Client(), "")
		f.baseURL = srv.URL
		for range 2 {
			branch, reason, err := f.Branch(context.Background(), clair, "release-", commit)
			if err != nil || branch != tc.branch || !strings.Contains(reason, tc.reason) || (reason == "") != (tc.reason == "") {
				t.Errorf("%s: Branch = %q, %q, %v; want %q, %q", tc.name, branch, reason, err, tc.branch, tc.reason)
			}
		}
		if want := map[bool]int{true: 1, false: 4}[tc.status == nil]; requests != want {
			t.Errorf("%s: %d requests, want %d from the first Branch only", tc.name, requests, want)
		}
		srv.Close()
	}
}

func TestBranchError(t *testing.T) {
	for _, tc := range []struct {
		status    int
		transient bool
	}{
		{http.StatusServiceUnavailable, true},
		{http.StatusTooManyRequests, true},
		{http.StatusNotFound, false},
	} {
		g := newFake(t)
		g.fail = func(w http.ResponseWriter) { w.WriteHeader(tc.status) }
		f := g.start(t, "")
		_, _, err := f.Branch(context.Background(), "https://github.com/quay/clair", "release-", sha(7))
		if err == nil || errors.Is(err, errTransient) != tc.transient {
			t.Errorf("GitHub %d: error %v, want transient %v", tc.status, err, tc.transient)
		}
		// An error is not cached: the next pass asks again.
		g.fail = nil
		before := g.requests
		_, _, _ = f.Branch(context.Background(), "https://github.com/quay/clair", "release-", sha(7))
		if g.requests == before {
			t.Errorf("GitHub %d: error was cached", tc.status)
		}
	}
}
