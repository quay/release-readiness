package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// These cases assert status only: CI's Go job builds an empty web/dist stub,
// so the SPA fallback's body cannot be checked here.
func TestStripBasePath(t *testing.T) {
	srv := setupTestServer(t)
	srv.BasePath = "/v2"

	cases := []struct {
		name       string
		path       string
		wantStatus int
	}{
		{"api under prefix", "/v2/api/v1/health", http.StatusOK},
		{"bare prefix redirects", "/v2", http.StatusPermanentRedirect},
		{"unprefixed passes through", "/api/v1/health", http.StatusOK},
		{"unknown prefixed path serves SPA", "/v2/releases/quay-v5.0.0", http.StatusOK},
		{"duplicated slash is cleaned, not redirected outside the prefix", "/v2/api/v1//health", http.StatusOK},
		{"dot segments are cleaned inside the prefix", "/v2/../api/v1/health", http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			srv.http.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.path, nil))
			if rec.Code != tc.wantStatus {
				t.Fatalf("GET %s = %d, want %d", tc.path, rec.Code, tc.wantStatus)
			}
		})
	}

	redirect := httptest.NewRecorder()
	srv.http.Handler.ServeHTTP(redirect, httptest.NewRequest(http.MethodGet, "/v2?tab=ci", nil))
	if got := redirect.Header().Get("Location"); got != "/v2/?tab=ci" {
		t.Fatalf("redirect Location = %q, want /v2/?tab=ci", got)
	}
}

func TestStripBasePathEmptyServesAtRoot(t *testing.T) {
	srv := setupTestServer(t)
	for _, path := range []string{"/api/v1/health", "/v2/api/v1/health"} {
		rec := httptest.NewRecorder()
		srv.http.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		// Without a configured base path the app serves at the root: real
		// routes answer, and the /v2 path falls through to the SPA fallback.
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s = %d, want %d", path, rec.Code, http.StatusOK)
		}
	}
}
