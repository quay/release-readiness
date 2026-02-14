package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/quay/release-readiness/internal/db"
	s3client "github.com/quay/release-readiness/internal/s3"
)

type Server struct {
	db          *db.DB
	s3          *s3client.Client
	http        *http.Server
	logger      *slog.Logger
	jiraBaseURL string
}

func New(database *db.DB, s3c *s3client.Client, addr, jiraBaseURL string, logger *slog.Logger) *Server {
	s := &Server{db: database, s3: s3c, logger: logger, jiraBaseURL: jiraBaseURL}
	mux := http.NewServeMux()
	s.registerRoutes(mux)

	var handler http.Handler = mux
	handler = loggingMiddleware(logger, handler)
	handler = recoveryMiddleware(logger, handler)

	s.http = &http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	return s
}

func (s *Server) Run(ctx context.Context) error {
	go func() {
		s.logger.Info("listening", "addr", s.http.Addr)
		if err := s.http.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Error("server error", "error", err)
		}
	}()

	<-ctx.Done()
	s.logger.Info("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := s.http.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}

	return nil
}
