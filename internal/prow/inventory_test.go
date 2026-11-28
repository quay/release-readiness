package prow

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestInventoryPinsAllFilesAndRejectsPartial(t *testing.T) {
	sha := strings.Repeat("a", 40)
	broken := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/repos/openshift/release/")
		if p == "commits/main" {
			_ = json.NewEncoder(w).Encode(map[string]string{"sha": sha})
			return
		}
		if r.URL.Query().Get("ref") != sha {
			t.Error("un-pinned inventory read")
		}
		dir := strings.TrimPrefix(p, "contents/")
		if strings.HasSuffix(dir, ".yaml") {
			if broken {
				http.Error(w, "failure", http.StatusServiceUnavailable)
				return
			}
			name := "periodic-ci-" + strings.TrimSuffix(strings.TrimPrefix(dir, dir[:strings.LastIndex(dir, "/")+1]), "-periodics.yaml") + "-aws-test"
			_, _ = fmt.Fprintf(w, "periodics:\n- name: %s\n  labels:\n    job-release: '4.22'\n  spec:\n    containers:\n    - args: ['--target=aws-s3-nightly']\n", name)
			return
		}
		name := "quay-quay-redhat-3.18-periodics.yaml"
		if strings.HasSuffix(dir, "quay-operator") {
			name = "quay-quay-operator-redhat-5.0-periodics.yaml"
		}
		_ = json.NewEncoder(w).Encode([]map[string]string{{"name": name, "path": dir + "/" + name, "type": "file"}, {"name": "quay-quay-master-periodics.yaml", "type": "file"}})
	}))
	defer srv.Close()
	c := NewInventoryClient("")
	c.BaseURL = srv.URL
	c.RawBaseURL = ""
	jobs, rev, err := c.Fetch(t.Context(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 2 || rev != sha {
		t.Fatalf("%+v %s", jobs, rev)
	}
	broken = true
	if jobs, _, err = c.Fetch(t.Context(), ""); err == nil || jobs != nil {
		t.Fatal("published partial inventory")
	}
	if jobs, rev, err = c.Fetch(t.Context(), sha); err != nil || jobs != nil || rev != sha {
		t.Fatalf("unchanged revision: %v", err)
	}
}

func TestInventoryRawFilesArePinnedAndUnauthenticated(t *testing.T) {
	sha := strings.Repeat("a", 40)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/"+sha+"/ci-operator/jobs/test.yaml" || r.Header.Get("Authorization") != "" {
			t.Errorf("unexpected raw request %s auth=%t", r.URL.Path, r.Header.Get("Authorization") != "")
		}
		_, _ = w.Write([]byte("periodics: []"))
	}))
	defer srv.Close()
	c := NewInventoryClient("test-token")
	c.RawBaseURL = srv.URL
	if _, err := c.get(t.Context(), "contents/ci-operator/jobs/test.yaml?ref="+sha, true); err != nil {
		t.Fatal(err)
	}
}
