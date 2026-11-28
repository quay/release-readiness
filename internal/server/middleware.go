package server

import (
	"log/slog"
	"net/http"
	"path"
	"strings"
	"time"
)

func loggingMiddleware(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rw, r)
		logger.Info("http request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rw.status,
			"duration", time.Since(start).Round(time.Millisecond),
		)
	})
}

func recoveryMiddleware(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				logger.Error("panic recovered", "error", err)
				http.Error(w, "internal server error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

// stripBasePath removes BasePath (e.g. /v2) from request paths before
// routing, so the app works behind a path-prefixed route that does not
// rewrite URLs. The bare prefix redirects to its trailing-slash form, which
// relative asset URLs in the served document rely on. Paths without the
// prefix pass through unchanged, so direct pod probes keep working.
func (s *Server) stripBasePath(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		base := s.BasePath
		if base == "" {
			next.ServeHTTP(w, r)
			return
		}
		if r.URL.Path == base {
			target := base + "/"
			if r.URL.RawQuery != "" {
				target += "?" + r.URL.RawQuery
			}
			http.Redirect(w, r, target, http.StatusPermanentRedirect)
			return
		}
		if rest, ok := strings.CutPrefix(r.URL.Path, base+"/"); ok {
			// Clean the remainder so the mux never answers with its own
			// path-cleaning redirect, whose Location would point outside
			// the prefix the route forwards.
			r.URL.Path = path.Clean("/" + rest)
		}
		next.ServeHTTP(w, r)
	})
}
