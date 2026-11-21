package shipment

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestAuthenticatedRedirectCannotLeaveGitLabOrigin(t *testing.T) {
	var contacted atomic.Bool
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contacted.Store(true)
		if r.Header.Get("PRIVATE-TOKEN") != "" {
			t.Error("credential forwarded to redirect destination")
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	defer destination.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, destination.URL, http.StatusFound) }))
	defer origin.Close()
	c := NewClient(origin.URL, origin.URL, 1, "test-only-token", origin.Client())
	if _, err := c.get(t.Context(), origin.URL+"/api/v4/projects/1", true); err == nil {
		t.Error("accepted cross-origin authenticated redirect")
	}
	if contacted.Load() {
		t.Error("followed cross-origin redirect")
	}
}

func TestAuthenticatedSameOriginRedirect(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/start" {
			http.Redirect(w, r, "/end", http.StatusFound)
			return
		}
		if r.Header.Get("PRIVATE-TOKEN") != "test-only-token" {
			t.Error("same-origin token lost")
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	c := NewClient(srv.URL, srv.URL, 1, "test-only-token", srv.Client())
	if _, err := c.get(t.Context(), srv.URL+"/start", true); err != nil {
		t.Fatal(err)
	}
}

func TestShipmentResponseSizeBounded(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		block := make([]byte, 1<<20)
		for range 17 {
			if _, err := w.Write(block); err != nil {
				return
			}
		}
	}))
	defer srv.Close()
	c := NewClient(srv.URL, srv.URL, 1, "", srv.Client())
	if _, err := c.get(t.Context(), srv.URL, false); err == nil {
		t.Fatal("oversized body accepted")
	}
}
