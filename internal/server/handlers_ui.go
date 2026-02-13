package server

import (
	"html/template"
	"log"
	"net/http"
	"strconv"

	"github.com/quay/build-dashboard/internal/model"
	"github.com/quay/build-dashboard/web"
)

var templates map[string]*template.Template

func init() {
	funcMap := template.FuncMap{
		"isMapped": func(comp model.Component, suite model.Suite) bool {
			for _, s := range comp.Suites {
				if s.ID == suite.ID {
					return true
				}
			}
			return false
		},
		"isRequired": func(comp model.Component, suite model.Suite) bool {
			for _, s := range comp.Suites {
				if s.ID == suite.ID {
					return s.Required
				}
			}
			return false
		},
	}

	layout := template.Must(template.New("layout").Funcs(funcMap).ParseFS(web.TemplateFS, "templates/layout.html"))

	pages := []string{
		"templates/dashboard.html",
		"templates/build_detail.html",
		"templates/builds_list.html",
		"templates/readiness_list.html",
		"templates/readiness.html",
		"templates/component_config.html",
	}

	templates = make(map[string]*template.Template)
	for _, p := range pages {
		t := template.Must(template.Must(layout.Clone()).ParseFS(web.TemplateFS, p))
		templates[p] = t
	}
}

type pageData struct {
	Versions         []string
	VersionSummaries []model.VersionSummary
	Builds           []model.Build
	Build            *model.Build
	Components       []model.Component
	Suites           []model.Suite
	Matrix           *model.ReadinessMatrix
	Filter           filterParams
	HasNext          bool
	NextOffset       int
	PrevOffset       int
}

type filterParams struct {
	Component string
	Version   string
	Status    string
	Offset    int
}

func (s *Server) basePageData() pageData {
	versions, _ := s.db.ActiveVersions()
	return pageData{Versions: versions}
}

func (s *Server) handleDashboardPage(w http.ResponseWriter, r *http.Request) {
	data := s.basePageData()

	version := r.URL.Query().Get("version")
	if version == "" && len(data.Versions) > 0 {
		version = data.Versions[0]
	}

	if version != "" {
		builds, err := s.db.LatestBuildsPerComponent(version)
		if err != nil {
			log.Printf("dashboard: %v", err)
		}
		data.Builds = builds
	}

	renderPage(w, "templates/dashboard.html", data)
}

func (s *Server) handleBuildDetailPage(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid build id", http.StatusBadRequest)
		return
	}

	build, err := s.db.GetBuild(id)
	if err != nil {
		http.Error(w, "build not found", http.StatusNotFound)
		return
	}

	data := s.basePageData()
	data.Build = build
	renderPage(w, "templates/build_detail.html", data)
}

func (s *Server) handleBuildsListPage(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	offset, _ := strconv.Atoi(q.Get("offset"))
	limit := 50

	data := s.basePageData()
	data.Filter = filterParams{
		Component: q.Get("component"),
		Version:   q.Get("version"),
		Status:    q.Get("status"),
		Offset:    offset,
	}

	builds, err := s.db.ListBuilds(data.Filter.Component, data.Filter.Version, data.Filter.Status, limit+1, offset)
	if err != nil {
		log.Printf("builds list: %v", err)
	}

	if len(builds) > limit {
		data.HasNext = true
		data.NextOffset = offset + limit
		builds = builds[:limit]
	}
	if offset > 0 {
		data.PrevOffset = offset - limit
		if data.PrevOffset < 0 {
			data.PrevOffset = 0
		}
	}
	data.Builds = builds

	components, _ := s.db.ListComponents()
	data.Components = components

	renderPage(w, "templates/builds_list.html", data)
}

func (s *Server) handleReadinessListPage(w http.ResponseWriter, r *http.Request) {
	data := s.basePageData()
	summaries, err := s.db.VersionSummaries()
	if err != nil {
		log.Printf("readiness list: %v", err)
	}
	data.VersionSummaries = summaries
	renderPage(w, "templates/readiness_list.html", data)
}

func (s *Server) handleReadinessPage(w http.ResponseWriter, r *http.Request) {
	version := r.PathValue("version")
	matrix, err := s.db.GetReadiness(version)
	if err != nil {
		log.Printf("readiness: %v", err)
		http.Error(w, "error loading readiness", http.StatusInternalServerError)
		return
	}

	data := s.basePageData()
	data.Matrix = matrix
	renderPage(w, "templates/readiness.html", data)
}

func (s *Server) handleConfigPage(w http.ResponseWriter, r *http.Request) {
	data := s.basePageData()
	components, _ := s.db.ListComponents()
	suites, _ := s.db.ListSuites()
	data.Components = components
	data.Suites = suites
	renderPage(w, "templates/component_config.html", data)
}

func (s *Server) handleConfigUpdate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form data", http.StatusBadRequest)
		return
	}

	components, _ := s.db.ListComponents()
	suites, _ := s.db.ListSuites()

	for _, comp := range components {
		for _, suite := range suites {
			mapKey := "map_" + comp.Name + "_" + suite.Name
			reqKey := "req_" + comp.Name + "_" + suite.Name
			mapped := r.FormValue(mapKey) == "1"
			required := r.FormValue(reqKey) == "1"

			if mapped {
				s.db.MapSuiteToComponent(comp.Name, suite.Name, required)
			} else {
				s.db.UnmapSuiteFromComponent(comp.Name, suite.Name)
			}
		}
	}

	http.Redirect(w, r, "/config", http.StatusSeeOther)
}

func renderPage(w http.ResponseWriter, page string, data interface{}) {
	t, ok := templates[page]
	if !ok {
		log.Printf("template %q not found", page)
		http.Error(w, "template not found", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.ExecuteTemplate(w, "layout", data); err != nil {
		log.Printf("template error: %v", err)
	}
}
