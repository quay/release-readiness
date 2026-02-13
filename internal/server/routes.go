package server

import (
	"io/fs"
	"net/http"

	"github.com/quay/build-dashboard/web"
)

func (s *Server) registerRoutes(mux *http.ServeMux) {
	// Static files — strip the "static" prefix from the embedded FS
	staticSub, _ := fs.Sub(web.StaticFS, "static")
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(staticSub)))

	// Health
	mux.HandleFunc("GET /api/v1/health", s.handleHealth)

	// Components API
	mux.HandleFunc("GET /api/v1/components", s.handleListComponents)
	mux.HandleFunc("POST /api/v1/components", s.handleCreateComponent)
	mux.HandleFunc("POST /api/v1/components/{name}/suites", s.handleMapSuite)

	// Suites API
	mux.HandleFunc("GET /api/v1/suites", s.handleListSuites)
	mux.HandleFunc("POST /api/v1/suites", s.handleCreateSuite)

	// Builds API
	mux.HandleFunc("POST /api/v1/builds", s.handleCreateBuild)
	mux.HandleFunc("GET /api/v1/builds", s.handleListBuilds)
	mux.HandleFunc("GET /api/v1/builds/latest", s.handleLatestBuild)
	mux.HandleFunc("GET /api/v1/builds/{id}", s.handleGetBuild)

	// Test Results API
	mux.HandleFunc("POST /api/v1/builds/{id}/results", s.handleSubmitResults)
	mux.HandleFunc("GET /api/v1/builds/{id}/results", s.handleGetResults)
	mux.HandleFunc("GET /api/v1/test-runs/{id}", s.handleGetTestRun)

	// Readiness API
	mux.HandleFunc("GET /api/v1/readiness/{version}", s.handleReadiness)

	// UI routes
	mux.HandleFunc("GET /{$}", s.handleDashboardPage)
	mux.HandleFunc("GET /builds/{id}", s.handleBuildDetailPage)
	mux.HandleFunc("GET /builds", s.handleBuildsListPage)
	mux.HandleFunc("GET /readiness", s.handleReadinessListPage)
	mux.HandleFunc("GET /readiness/{version}", s.handleReadinessPage)
	mux.HandleFunc("GET /config", s.handleConfigPage)
	mux.HandleFunc("POST /config", s.handleConfigUpdate)
}
