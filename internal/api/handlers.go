package api

import (
	"net/http"

	"github.com/buildhive/buildhive/internal/auth"
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

func (s *Server) listProjects(w http.ResponseWriter, r *http.Request)     { writeJSON(w, 200, []any{}) }
func (s *Server) createProject(w http.ResponseWriter, r *http.Request)    { writeJSON(w, 201, map[string]any{}) }
func (s *Server) deleteProject(w http.ResponseWriter, r *http.Request)    { w.WriteHeader(204) }
func (s *Server) createToken(w http.ResponseWriter, r *http.Request)      { writeJSON(w, 201, map[string]any{}) }
func (s *Server) listBuilders(w http.ResponseWriter, r *http.Request)     { writeJSON(w, 200, []any{}) }
func (s *Server) listBuilds(w http.ResponseWriter, r *http.Request)       { writeJSON(w, 200, []any{}) }
func (s *Server) streamBuildLogs(w http.ResponseWriter, r *http.Request)  { w.WriteHeader(200) }
func (s *Server) getMetrics(w http.ResponseWriter, r *http.Request)       { writeJSON(w, 200, map[string]any{}) }
func (s *Server) registerBuilder(w http.ResponseWriter, r *http.Request)  { writeJSON(w, 200, map[string]any{}) }
func (s *Server) builderHeartbeat(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }
func (s *Server) buildEvent(w http.ResponseWriter, r *http.Request)       { w.WriteHeader(204) }
func (s *Server) initBuild(w http.ResponseWriter, r *http.Request)        { writeJSON(w, 200, map[string]any{}) }
