package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/buildhive/buildhive/internal/auth"
)

func (s *Server) adminAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := bearerToken(r)
		if token == "" || !auth.VerifyToken(token, s.cfg.AdminTokenHash) {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) projectTokenAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// TODO(task-7): implement project token validation
		// Token validation is performed inside initBuild so the handler
		// has access to the store and can return the build record in one
		// round-trip. Move validation here if additional routes need it.
		next.ServeHTTP(w, r)
	})
}

func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	after, ok := strings.CutPrefix(h, "Bearer ")
	if !ok {
		return ""
	}
	return after
}

func decodeJSON(r *http.Request, v any) error {
	return json.NewDecoder(r.Body).Decode(v)
}
