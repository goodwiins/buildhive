package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/buildhive/buildhive/internal/store"
)

// Config holds server configuration.
type Config struct {
	AdminTokenHash string
}

// Server is the HTTP API server.
type Server struct {
	router chi.Router
	store  *store.Store
	cfg    Config
}

// New creates and configures a new Server.
func New(cfg Config, s *store.Store) *Server {
	srv := &Server{cfg: cfg, store: s}
	srv.router = srv.buildRouter()
	return srv
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(w, r)
}

func (s *Server) buildRouter() chi.Router {
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RealIP)

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	r.Route("/api", func(r chi.Router) {
		r.Post("/auth/login", s.handleLogin)

		// Admin-authenticated routes
		r.Group(func(r chi.Router) {
			r.Use(s.adminAuth)
			r.Route("/projects", func(r chi.Router) {
				r.Get("/", s.listProjects)
				r.Post("/", s.createProject)
				r.Delete("/{id}", s.deleteProject)
				r.Post("/{id}/tokens", s.createToken)
			})
			r.Get("/builders", s.listBuilders)
			r.Get("/builds", s.listBuilds)
			r.Get("/builds/{id}/logs", s.streamBuildLogs)
			r.Get("/metrics", s.getMetrics)
		})

		// Agent registration (no admin auth — uses shared builder secret)
		r.Post("/builders/register", s.registerBuilder)
		r.Post("/builders/heartbeat", s.builderHeartbeat)
		r.Post("/builds/{id}/events", s.buildEvent)

		// Build init — project token auth
		r.With(s.projectTokenAuth).Post("/builds/init", s.initBuild)
	})

	return r
}

// writeJSON encodes v as JSON and writes it with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v) //nolint:errcheck
}

// writeError writes a JSON error response.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
