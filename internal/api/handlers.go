package api

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/buildhive/buildhive/internal/auth"
	"github.com/buildhive/buildhive/internal/store/db"
)

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Token string `json:"token"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if !auth.VerifyToken(body.Token, s.cfg.AdminTokenHash) {
		writeError(w, http.StatusUnauthorized, "invalid token")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// listProjects returns all projects as a JSON array.
func (s *Server) listProjects(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		writeError(w, http.StatusServiceUnavailable, "store not available")
		return
	}
	projects, err := s.store.ListProjects(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("list projects: %v", err))
		return
	}
	if projects == nil {
		projects = []db.Project{}
	}
	writeJSON(w, http.StatusOK, projects)
}

// createProject decodes {"name": "...", "slug": "..."} and creates a project.
func (s *Server) createProject(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		writeError(w, http.StatusServiceUnavailable, "store not available")
		return
	}
	var body struct {
		Name string `json:"name"`
		Slug string `json:"slug"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(body.Name) == "" || strings.TrimSpace(body.Slug) == "" {
		writeError(w, http.StatusBadRequest, "name and slug are required")
		return
	}
	project, err := s.store.CreateProject(r.Context(), db.CreateProjectParams{
		Name: body.Name,
		Slug: body.Slug,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("create project: %v", err))
		return
	}
	writeJSON(w, http.StatusCreated, project)
}

// deleteProject parses the {id} URL param and deletes the project.
func (s *Server) deleteProject(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		writeError(w, http.StatusServiceUnavailable, "store not available")
		return
	}
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid project id")
		return
	}
	if err := s.store.DeleteProject(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("delete project: %v", err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// createToken generates a project API token, stores only the hash, and returns the plaintext once.
func (s *Server) createToken(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		writeError(w, http.StatusServiceUnavailable, "store not available")
		return
	}
	idStr := chi.URLParam(r, "id")
	projectID, err := uuid.Parse(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid project id")
		return
	}
	var body struct {
		Label string `json:"label"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	plaintext, hash, err := auth.GenerateToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate token")
		return
	}
	_, err = s.store.CreateToken(r.Context(), db.CreateTokenParams{
		ProjectID: projectID,
		TokenHash: hash,
		Label:     body.Label,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("create token: %v", err))
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"token": plaintext})
}

// listBuilders returns all registered builders.
func (s *Server) listBuilders(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		writeError(w, http.StatusServiceUnavailable, "store not available")
		return
	}
	builders, err := s.store.ListBuilders(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("list builders: %v", err))
		return
	}
	if builders == nil {
		builders = []db.Builder{}
	}
	writeJSON(w, http.StatusOK, builders)
}

// registerBuilder upserts a builder by name.
func (s *Server) registerBuilder(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		writeError(w, http.StatusServiceUnavailable, "store not available")
		return
	}
	var body struct {
		Name    string `json:"name"`
		Address string `json:"address"`
		Arch    string `json:"arch"`
		Status  string `json:"status"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	builder, err := s.store.UpsertBuilder(r.Context(), db.UpsertBuilderParams{
		Name:    body.Name,
		Address: body.Address,
		Arch:    body.Arch,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("register builder: %v", err))
		return
	}
	writeJSON(w, http.StatusOK, builder)
}

// builderHeartbeat updates the last_seen_at timestamp for a builder.
func (s *Server) builderHeartbeat(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		writeError(w, http.StatusServiceUnavailable, "store not available")
		return
	}
	var body struct {
		Name   string `json:"name"`
		Status string `json:"status"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := s.store.UpdateBuilderHeartbeat(r.Context(), db.UpdateBuilderHeartbeatParams{
		Name:   body.Name,
		Status: body.Status,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("heartbeat: %v", err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// listBuilds returns builds, optionally filtered by project slug via ?project=<slug>.
func (s *Server) listBuilds(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		writeError(w, http.StatusServiceUnavailable, "store not available")
		return
	}
	ctx := r.Context()
	slug := r.URL.Query().Get("project")
	if slug != "" {
		project, err := s.store.GetProjectBySlug(ctx, slug)
		if err != nil {
			writeError(w, http.StatusNotFound, "project not found")
			return
		}
		builds, err := s.store.ListBuildsByProject(ctx, project.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("list builds: %v", err))
			return
		}
		if builds == nil {
			builds = []db.Build{}
		}
		writeJSON(w, http.StatusOK, builds)
		return
	}
	// No project filter: return empty for now (no cross-project query defined).
	writeJSON(w, http.StatusOK, []db.Build{})
}

// initBuild validates a project token, selects a healthy builder, and creates a build record.
func (s *Server) initBuild(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		writeError(w, http.StatusServiceUnavailable, "store not available")
		return
	}
	ctx := r.Context()

	rawToken := bearerToken(r)
	if rawToken == "" {
		writeError(w, http.StatusUnauthorized, "missing bearer token")
		return
	}

	lookupHash := auth.HashToken(rawToken)

	token, err := s.store.GetTokenByHash(ctx, lookupHash)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid token")
		return
	}

	if err := s.store.UpdateTokenLastUsed(ctx, token.ID); err != nil {
		// best-effort — log if available, but don't fail the request
		_ = err
	}

	builders, err := s.store.GetHealthyBuilders(ctx)
	if err != nil || len(builders) == 0 {
		writeError(w, http.StatusServiceUnavailable, "no healthy builders available")
		return
	}

	// Pick the first healthy builder (most recently seen, per query ordering).
	builder := builders[0]

	build, err := s.store.CreateBuild(ctx, db.CreateBuildParams{
		ProjectID: token.ProjectID,
		BuilderID: uuid.NullUUID{UUID: builder.ID, Valid: true},
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("create build: %v", err))
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{
		"build_id":         build.ID.String(),
		"builder_endpoint": builder.Address,
	})
}

// streamBuildLogs streams build logs as Server-Sent Events.
func (s *Server) streamBuildLogs(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		writeError(w, http.StatusServiceUnavailable, "store not available")
		return
	}
	idStr := chi.URLParam(r, "id")
	buildID, err := uuid.Parse(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid build id")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	flusher, ok := w.(http.Flusher)
	if !ok {
		return
	}

	logs, err := s.store.GetBuildLogs(r.Context(), buildID)
	if err != nil {
		fmt.Fprintf(w, "data: error fetching logs\n\n")
		flusher.Flush()
		return
	}

	for _, entry := range logs {
		fmt.Fprintf(w, "data: %s\n\n", entry.Line)
		flusher.Flush()
	}
}

// getMetrics returns basic platform statistics.
func (s *Server) getMetrics(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		writeError(w, http.StatusServiceUnavailable, "store not available")
		return
	}
	builders, err := s.store.ListBuilders(r.Context())
	builderCount := 0
	if err == nil {
		builderCount = len(builders)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"builds_24h":     0,
		"cache_hit_rate": 0,
		"builder_count":  builderCount,
	})
}

// buildEvent accepts build status events from builder agents.
func (s *Server) buildEvent(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		writeError(w, http.StatusServiceUnavailable, "store not available")
		return
	}
	idStr := chi.URLParam(r, "id")
	buildID, err := uuid.Parse(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid build id")
		return
	}
	var body struct {
		Status   string `json:"status"`
		CacheHit bool   `json:"cache_hit"`
		ImageRef string `json:"image_ref"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	params := db.UpdateBuildStatusParams{
		ID:       buildID,
		Status:   body.Status,
		CacheHit: body.CacheHit,
		ImageRef: sql.NullString{String: body.ImageRef, Valid: body.ImageRef != ""},
	}
	if body.Status == "success" || body.Status == "failed" || body.Status == "cancelled" {
		params.FinishedAt = sql.NullTime{Time: time.Now(), Valid: true}
	}
	if err := s.store.UpdateBuildStatus(r.Context(), params); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("update build status: %v", err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
