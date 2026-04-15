# BuildHive Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build BuildHive — a fully open-source, self-hostable remote Docker build acceleration platform (Depot alternative) with a Go monorepo, PostgreSQL, React UI, CLI, machine agent, and GitHub Action.

**Architecture:** Three Go binaries (server, agent, CLI) from one module. The server embeds the compiled React bundle via `go:embed`. The control plane proxies BuildKit gRPC streams transparently to builder nodes running `buildkitd`. The machine agent manages buildkitd and heartbeats to the control plane.

**Tech Stack:** Go 1.22+, Chi (HTTP router), sqlc (type-safe DB queries), golang-migrate (SQL migrations), pgx/v5 (PostgreSQL driver), google.golang.org/grpc (gRPC proxy), bcrypt (token hashing), Cobra+Viper (CLI), React+Vite+shadcn/ui (web), testcontainers-go (integration tests)

---

## Phase 1: Project Foundation

### Task 1: Go module + directory scaffold

**Files:**
- Create: `go.mod`
- Create: `Makefile`
- Create: `cmd/server/main.go`
- Create: `cmd/agent/main.go`
- Create: `cmd/cli/main.go`
- Create: `docker-compose.yml`
- Create: `.gitignore`

**Step 1: Initialize Go module**

```bash
cd ~/development/buildhive
go mod init github.com/buildhive/buildhive
```

**Step 2: Create directory tree**

```bash
mkdir -p cmd/server cmd/agent cmd/cli
mkdir -p internal/api internal/proxy internal/store internal/agent internal/auth
mkdir -p schema web action
```

**Step 3: Write stub entrypoints**

`cmd/server/main.go`:
```go
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stdout, "buildhive-server starting")
}
```

Same stub pattern for `cmd/agent/main.go` and `cmd/cli/main.go`.

**Step 4: Write Makefile**

```makefile
.PHONY: build build-server build-agent build-cli test lint migrate-up web

GO=go
GOFLAGS=-trimpath
LDFLAGS=-s -w

build: build-server build-agent build-cli

build-server:
	$(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o bin/buildhive-server ./cmd/server

build-agent:
	$(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o bin/buildhive-agent ./cmd/agent

build-cli:
	$(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o bin/buildhive ./cmd/cli

test:
	$(GO) test ./... -v

lint:
	golangci-lint run ./...

migrate-up:
	migrate -path schema -database "$$DATABASE_URL" up

web:
	cd web && npm install && npm run build
```

**Step 5: Write `.gitignore`**

```
bin/
web/dist/
web/node_modules/
*.env
.env
```

**Step 6: Write `docker-compose.yml`**

```yaml
version: "3.9"
services:
  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_DB: buildhive
      POSTGRES_USER: buildhive
      POSTGRES_PASSWORD: buildhive
    ports:
      - "5432:5432"
    volumes:
      - pgdata:/var/lib/postgresql/data

  server:
    build:
      context: .
      dockerfile: Dockerfile.server
    environment:
      DATABASE_URL: postgres://buildhive:buildhive@postgres:5432/buildhive?sslmode=disable
      BUILDHIVE_ADMIN_TOKEN: changeme
      PORT: 8080
      GRPC_PORT: 8765
    ports:
      - "8080:8080"
      - "8765:8765"
    depends_on:
      - postgres

volumes:
  pgdata:
```

**Step 7: Install core dependencies**

```bash
go get github.com/go-chi/chi/v5
go get github.com/jackc/pgx/v5
go get golang.org/x/crypto
go get github.com/spf13/cobra
go get github.com/spf13/viper
go get google.golang.org/grpc
go get github.com/golang-migrate/migrate/v4
```

**Step 8: Verify module compiles**

```bash
go build ./...
```
Expected: no errors, no output.

**Step 9: Commit**

```bash
git add -A
git commit -m "feat: scaffold Go module + directory structure"
```

---

### Task 2: Database schema + migrations

**Files:**
- Create: `schema/000001_init.up.sql`
- Create: `schema/000001_init.down.sql`

**Step 1: Write `schema/000001_init.up.sql`**

```sql
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TABLE projects (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT NOT NULL,
    slug        TEXT NOT NULL UNIQUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE api_tokens (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id   UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    token_hash   TEXT NOT NULL,
    label        TEXT NOT NULL,
    last_used_at TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE builders (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name         TEXT NOT NULL UNIQUE,
    address      TEXT NOT NULL,  -- host:port of buildkitd
    arch         TEXT NOT NULL DEFAULT 'amd64',
    status       TEXT NOT NULL DEFAULT 'healthy',
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE builds (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id  UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    builder_id  UUID REFERENCES builders(id) ON DELETE SET NULL,
    status      TEXT NOT NULL DEFAULT 'pending',
    image_ref   TEXT,
    cache_hit   BOOLEAN NOT NULL DEFAULT FALSE,
    started_at  TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX builds_project_id_idx ON builds(project_id);
CREATE INDEX builds_created_at_idx ON builds(created_at DESC);

CREATE TABLE build_logs (
    id         BIGSERIAL PRIMARY KEY,
    build_id   UUID NOT NULL REFERENCES builds(id) ON DELETE CASCADE,
    ts         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    line       TEXT NOT NULL
);

CREATE INDEX build_logs_build_id_idx ON build_logs(build_id);
```

**Step 2: Write `schema/000001_init.down.sql`**

```sql
DROP TABLE IF EXISTS build_logs;
DROP TABLE IF EXISTS builds;
DROP TABLE IF EXISTS builders;
DROP TABLE IF EXISTS api_tokens;
DROP TABLE IF EXISTS projects;
```

**Step 3: Install golang-migrate CLI**

```bash
brew install golang-migrate
# or: go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest
```

**Step 4: Run migration against local postgres**

```bash
docker compose up -d postgres
export DATABASE_URL="postgres://buildhive:buildhive@localhost:5432/buildhive?sslmode=disable"
migrate -path schema -database "$DATABASE_URL" up
```
Expected: `1/u 000001_init (Xms)`

**Step 5: Verify tables exist**

```bash
psql "$DATABASE_URL" -c "\dt"
```
Expected: 5 tables listed (projects, api_tokens, builders, builds, build_logs).

**Step 6: Commit**

```bash
git add schema/
git commit -m "feat: add database schema + migrations"
```

---

### Task 3: sqlc setup + generated queries

**Files:**
- Create: `sqlc.yaml`
- Create: `internal/store/queries/projects.sql`
- Create: `internal/store/queries/tokens.sql`
- Create: `internal/store/queries/builders.sql`
- Create: `internal/store/queries/builds.sql`

**Step 1: Install sqlc**

```bash
brew install sqlc
# or: go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
```

**Step 2: Write `sqlc.yaml`**

```yaml
version: "2"
sql:
  - engine: "postgresql"
    queries: "internal/store/queries"
    schema: "schema"
    gen:
      go:
        package: "db"
        out: "internal/store/db"
        emit_json_tags: true
        emit_pointers_for_null_types: true
```

**Step 3: Write `internal/store/queries/projects.sql`**

```sql
-- name: CreateProject :one
INSERT INTO projects (name, slug) VALUES ($1, $2) RETURNING *;

-- name: GetProjectByID :one
SELECT * FROM projects WHERE id = $1;

-- name: GetProjectBySlug :one
SELECT * FROM projects WHERE slug = $1;

-- name: ListProjects :many
SELECT * FROM projects ORDER BY created_at DESC;

-- name: DeleteProject :exec
DELETE FROM projects WHERE id = $1;
```

**Step 4: Write `internal/store/queries/tokens.sql`**

```sql
-- name: CreateToken :one
INSERT INTO api_tokens (project_id, token_hash, label) VALUES ($1, $2, $3) RETURNING *;

-- name: ListTokensByProject :many
SELECT * FROM api_tokens WHERE project_id = $1 ORDER BY created_at DESC;

-- name: GetTokenByHash :one
SELECT * FROM api_tokens WHERE token_hash = $1;

-- name: UpdateTokenLastUsed :exec
UPDATE api_tokens SET last_used_at = NOW() WHERE id = $1;

-- name: DeleteToken :exec
DELETE FROM api_tokens WHERE id = $1;
```

**Step 5: Write `internal/store/queries/builders.sql`**

```sql
-- name: UpsertBuilder :one
INSERT INTO builders (name, address, arch)
VALUES ($1, $2, $3)
ON CONFLICT (name) DO UPDATE
  SET address = EXCLUDED.address, arch = EXCLUDED.arch, last_seen_at = NOW(), status = 'healthy'
RETURNING *;

-- name: UpdateBuilderHeartbeat :exec
UPDATE builders SET last_seen_at = NOW(), status = $2 WHERE name = $1;

-- name: ListBuilders :many
SELECT * FROM builders ORDER BY last_seen_at DESC;

-- name: GetHealthyBuilders :many
SELECT * FROM builders WHERE status = 'healthy'
  AND last_seen_at > NOW() - INTERVAL '30 seconds'
ORDER BY last_seen_at DESC;
```

**Step 6: Write `internal/store/queries/builds.sql`**

```sql
-- name: CreateBuild :one
INSERT INTO builds (project_id, builder_id, status) VALUES ($1, $2, 'pending') RETURNING *;

-- name: UpdateBuildStatus :exec
UPDATE builds SET status = $2, finished_at = $3, cache_hit = $4, image_ref = $5 WHERE id = $1;

-- name: StartBuild :exec
UPDATE builds SET status = 'running', started_at = NOW() WHERE id = $1;

-- name: ListBuildsByProject :many
SELECT * FROM builds WHERE project_id = $1 ORDER BY created_at DESC LIMIT 50;

-- name: GetBuild :one
SELECT * FROM builds WHERE id = $1;

-- name: InsertBuildLog :exec
INSERT INTO build_logs (build_id, line) VALUES ($1, $2);

-- name: GetBuildLogs :many
SELECT * FROM build_logs WHERE build_id = $1 ORDER BY id ASC;
```

**Step 7: Generate Go code**

```bash
sqlc generate
```
Expected: `internal/store/db/` created with `models.go`, `*.sql.go` files.

**Step 8: Add sqlc deps**

```bash
go mod tidy
```

**Step 9: Commit**

```bash
git add sqlc.yaml internal/store/queries/ internal/store/db/
git commit -m "feat: add sqlc queries + generate DB layer"
```

---

## Phase 2: Auth & Store

### Task 4: Token package

**Files:**
- Create: `internal/auth/token.go`
- Create: `internal/auth/token_test.go`

**Step 1: Write the failing test**

`internal/auth/token_test.go`:
```go
package auth_test

import (
	"testing"
	"github.com/buildhive/buildhive/internal/auth"
)

func TestGenerateAndVerify(t *testing.T) {
	token, hash, err := auth.GenerateToken()
	if err != nil {
		t.Fatalf("GenerateToken() error: %v", err)
	}
	if len(token) < 32 {
		t.Errorf("token too short: %d", len(token))
	}
	if !auth.VerifyToken(token, hash) {
		t.Error("VerifyToken() returned false for valid token")
	}
}

func TestVerifyInvalidToken(t *testing.T) {
	_, hash, _ := auth.GenerateToken()
	if auth.VerifyToken("wrongtoken", hash) {
		t.Error("VerifyToken() returned true for wrong token")
	}
}

func TestHashAdminToken(t *testing.T) {
	hash, err := auth.HashToken("admin-secret")
	if err != nil {
		t.Fatalf("HashToken() error: %v", err)
	}
	if !auth.VerifyToken("admin-secret", hash) {
		t.Error("VerifyToken() failed for admin token")
	}
}
```

**Step 2: Run test to verify it fails**

```bash
go test ./internal/auth/... -v
```
Expected: FAIL — `package auth not found`

**Step 3: Write `internal/auth/token.go`**

```go
package auth

import (
	"crypto/rand"
	"encoding/hex"

	"golang.org/x/crypto/bcrypt"
)

const bcryptCost = 12

// GenerateToken creates a cryptographically random token and its bcrypt hash.
// The plaintext token is returned for one-time display to the user.
func GenerateToken() (token, hash string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return
	}
	token = hex.EncodeToString(b)
	hash, err = hashRaw(token)
	return
}

// HashToken hashes an existing plaintext token (e.g. the admin token from env).
func HashToken(token string) (string, error) {
	return hashRaw(token)
}

// VerifyToken checks a plaintext token against a bcrypt hash.
func VerifyToken(token, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(token))
	return err == nil
}

func hashRaw(token string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(token), bcryptCost)
	return string(h), err
}
```

**Step 4: Run test to verify it passes**

```bash
go test ./internal/auth/... -v
```
Expected: PASS — 3 tests

**Step 5: Commit**

```bash
git add internal/auth/
git commit -m "feat: add token generation and verification"
```

---

### Task 5: Store package (DB wrapper)

**Files:**
- Create: `internal/store/store.go`

**Step 1: Write `internal/store/store.go`**

```go
package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	db "github.com/buildhive/buildhive/internal/store/db"
)

// Store wraps sqlc Queries with a connection pool.
type Store struct {
	*db.Queries
	pool *pgxpool.Pool
}

// New opens a pgx connection pool and returns a Store.
func New(ctx context.Context, dsn string) (*Store, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("open pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("ping db: %w", err)
	}
	return &Store{Queries: db.New(pool), pool: pool}, nil
}

// Close releases all pool connections.
func (s *Store) Close() {
	s.pool.Close()
}
```

**Step 2: Verify it compiles**

```bash
go build ./internal/store/...
```
Expected: no errors.

**Step 3: Commit**

```bash
git add internal/store/store.go
git commit -m "feat: add store wrapper over sqlc"
```

---

## Phase 3: REST API

### Task 6: Server setup (Chi router + health check)

**Files:**
- Create: `internal/api/server.go`
- Create: `internal/api/server_test.go`

**Step 1: Write the failing test**

`internal/api/server_test.go`:
```go
package api_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/buildhive/buildhive/internal/api"
)

func TestHealthCheck(t *testing.T) {
	srv := api.New(api.Config{AdminTokenHash: "testhash"}, nil)
	r := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("want 200, got %d", w.Code)
	}
}
```

**Step 2: Run test to verify it fails**

```bash
go test ./internal/api/... -v
```
Expected: FAIL — `api.New undefined`

**Step 3: Write `internal/api/server.go`**

```go
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
		// Public
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

		// Agent registration (uses builder secret)
		r.Post("/builders/register", s.registerBuilder)
		r.Post("/builders/heartbeat", s.builderHeartbeat)
		r.Post("/builds/{id}/events", s.buildEvent)

		// Build init (project token auth)
		r.With(s.projectTokenAuth).Post("/builds/init", s.initBuild)
	})

	return r
}

// writeJSON encodes v as JSON and writes it to w with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// writeError writes a JSON error response.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
```

**Step 4: Add stub handlers (so it compiles)**

Create `internal/api/handlers.go`:
```go
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

func (s *Server) listProjects(w http.ResponseWriter, r *http.Request)    { writeJSON(w, 200, []any{}) }
func (s *Server) createProject(w http.ResponseWriter, r *http.Request)   { writeJSON(w, 201, map[string]any{}) }
func (s *Server) deleteProject(w http.ResponseWriter, r *http.Request)   { w.WriteHeader(204) }
func (s *Server) createToken(w http.ResponseWriter, r *http.Request)     { writeJSON(w, 201, map[string]any{}) }
func (s *Server) listBuilders(w http.ResponseWriter, r *http.Request)    { writeJSON(w, 200, []any{}) }
func (s *Server) listBuilds(w http.ResponseWriter, r *http.Request)      { writeJSON(w, 200, []any{}) }
func (s *Server) streamBuildLogs(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }
func (s *Server) getMetrics(w http.ResponseWriter, r *http.Request)      { writeJSON(w, 200, map[string]any{}) }
func (s *Server) registerBuilder(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, map[string]any{}) }
func (s *Server) builderHeartbeat(w http.ResponseWriter, r *http.Request){ w.WriteHeader(204) }
func (s *Server) buildEvent(w http.ResponseWriter, r *http.Request)      { w.WriteHeader(204) }
func (s *Server) initBuild(w http.ResponseWriter, r *http.Request)       { writeJSON(w, 200, map[string]any{}) }
```

Create `internal/api/middleware.go`:
```go
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
		// Token validation against DB happens in the handler (needs project context)
		next.ServeHTTP(w, r)
	})
}

func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if after, ok := strings.CutPrefix(h, "Bearer "); ok {
		return after
	}
	return ""
}

func decodeJSON(r *http.Request, v any) error {
	return json.NewDecoder(r.Body).Decode(v)
}
```

**Step 5: Run test to verify it passes**

```bash
go test ./internal/api/... -v
```
Expected: PASS

**Step 6: Commit**

```bash
git add internal/api/
git commit -m "feat: add HTTP server with Chi router and health check"
```

---

### Task 7: Projects, tokens, builders, builds handlers (real implementations)

**Files:**
- Modify: `internal/api/handlers.go`

**Step 1: Implement real handler logic**

Replace stub handlers with real implementations. Here are the key ones:

**`listProjects`:**
```go
func (s *Server) listProjects(w http.ResponseWriter, r *http.Request) {
	projects, err := s.store.ListProjects(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, projects)
}
```

**`createProject`:**
```go
func (s *Server) createProject(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
		Slug string `json:"slug"`
	}
	if err := decodeJSON(r, &body); err != nil || body.Name == "" || body.Slug == "" {
		writeError(w, http.StatusBadRequest, "name and slug required")
		return
	}
	p, err := s.store.CreateProject(r.Context(), db.CreateProjectParams{
		Name: body.Name, Slug: body.Slug,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusCreated, p)
}
```

**`createToken`:**
```go
func (s *Server) createToken(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	var body struct{ Label string `json:"label"` }
	decodeJSON(r, &body)

	plain, hash, err := auth.GenerateToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "token generation failed")
		return
	}
	uid, _ := uuid.Parse(projectID)
	_, err = s.store.CreateToken(r.Context(), db.CreateTokenParams{
		ProjectID: pgtype.UUID{Bytes: uid, Valid: true},
		TokenHash: hash,
		Label:     body.Label,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	// Return plaintext token ONCE — never stored
	writeJSON(w, http.StatusCreated, map[string]string{"token": plain})
}
```

**`initBuild`:**
```go
func (s *Server) initBuild(w http.ResponseWriter, r *http.Request) {
	rawToken := bearerToken(r)
	// Look up token by hash
	hash, _ := auth.HashToken(rawToken)
	apiToken, err := s.store.GetTokenByHash(r.Context(), hash)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid token")
		return
	}
	// Mark token as used
	s.store.UpdateTokenLastUsed(r.Context(), apiToken.ID)

	// Pick a healthy builder
	builders, err := s.store.GetHealthyBuilders(r.Context())
	if err != nil || len(builders) == 0 {
		writeError(w, http.StatusServiceUnavailable, "no healthy builders available")
		return
	}
	builder := builders[0] // simple: pick first (round-robin in Task 14)

	// Create build record
	build, err := s.store.CreateBuild(r.Context(), db.CreateBuildParams{
		ProjectID: apiToken.ProjectID,
		BuilderID: pgtype.UUID{Bytes: builder.ID.Bytes, Valid: true},
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"build_id":         build.ID,
		"builder_endpoint": builder.Address,
	})
}
```

**`streamBuildLogs` (SSE):**
```go
func (s *Server) streamBuildLogs(w http.ResponseWriter, r *http.Request) {
	buildID := chi.URLParam(r, "id")
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	logs, err := s.store.GetBuildLogs(r.Context(), pgtype.UUID{})
	if err != nil {
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		return
	}
	_ = buildID
	for _, log := range logs {
		fmt.Fprintf(w, "data: %s\n\n", log.Line)
		flusher.Flush()
	}
}
```

**Step 2: Add `github.com/google/uuid` and wire up imports**

```bash
go get github.com/google/uuid
go mod tidy
```

**Step 3: Verify compilation**

```bash
go build ./...
```

**Step 4: Commit**

```bash
git add internal/api/
git commit -m "feat: implement project, token, builder, and build handlers"
```

---

### Task 8: Wire up server binary

**Files:**
- Modify: `cmd/server/main.go`

**Step 1: Implement `cmd/server/main.go`**

```go
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/buildhive/buildhive/internal/api"
	"github.com/buildhive/buildhive/internal/auth"
	"github.com/buildhive/buildhive/internal/store"
)

func main() {
	ctx := context.Background()

	dsn := mustEnv("DATABASE_URL")
	adminToken := mustEnv("BUILDHIVE_ADMIN_TOKEN")
	port := envOr("PORT", "8080")

	s, err := store.New(ctx, dsn)
	if err != nil {
		log.Fatalf("connect db: %v", err)
	}
	defer s.Close()

	adminHash, err := auth.HashToken(adminToken)
	if err != nil {
		log.Fatalf("hash admin token: %v", err)
	}

	srv := api.New(api.Config{AdminTokenHash: adminHash}, s)

	addr := fmt.Sprintf(":%s", port)
	log.Printf("buildhive-server listening on %s", addr)
	if err := http.ListenAndServe(addr, srv); err != nil {
		log.Fatalf("server: %v", err)
	}
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("required env var %s is not set", key)
	}
	return v
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
```

**Step 2: Build and verify**

```bash
make build-server
DATABASE_URL="postgres://buildhive:buildhive@localhost:5432/buildhive?sslmode=disable" \
  BUILDHIVE_ADMIN_TOKEN=devtoken \
  ./bin/buildhive-server &
curl http://localhost:8080/healthz
```
Expected: `{"status":"ok"}`

**Step 3: Kill test server and commit**

```bash
kill %1
git add cmd/server/
git commit -m "feat: wire up server binary with env config"
```

---

## Phase 4: gRPC Proxy

### Task 9: BuildKit gRPC transparent proxy

The control plane must accept gRPC connections from `docker buildx` (using the BuildKit protocol) and proxy them to `buildkitd` on the builder node.

**Files:**
- Create: `internal/proxy/proxy.go`
- Create: `internal/proxy/proxy_test.go`

**Step 1: Add gRPC dependencies**

```bash
go get google.golang.org/grpc
go get google.golang.org/grpc/codes
go get google.golang.org/grpc/status
```

**Step 2: Write `internal/proxy/proxy.go`**

```go
package proxy

import (
	"context"
	"fmt"
	"io"
	"log"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// Proxy is a transparent gRPC reverse proxy.
// It forwards all incoming RPCs to a backend determined by the Director.
type Proxy struct {
	director Director
}

// Director decides which backend address to forward an RPC to.
type Director func(ctx context.Context, fullMethodName string) (context.Context, *grpc.ClientConn, error)

// New creates a Proxy with the given Director.
func New(director Director) *Proxy {
	return &Proxy{director: director}
}

// Handler returns a grpc.UnknownServiceHandler that transparently proxies all calls.
func (p *Proxy) Handler() grpc.UnknownServiceHandler {
	return func(srv any, stream grpc.ServerStream) error {
		fullMethod, ok := grpc.MethodFromServerStream(stream)
		if !ok {
			return status.Error(codes.Internal, "missing method")
		}

		outCtx, conn, err := p.director(stream.Context(), fullMethod)
		if err != nil {
			return err
		}
		defer conn.Close()

		// Open client stream to backend
		clientCtx, clientCancel := context.WithCancel(outCtx)
		defer clientCancel()

		// Forward incoming metadata
		md, _ := metadata.FromIncomingContext(stream.Context())
		outCtx = metadata.NewOutgoingContext(clientCtx, md)

		clientStream, err := grpc.NewClientStream(outCtx, &grpc.StreamDesc{
			ServerStreams: true,
			ClientStreams: true,
		}, conn, fullMethod)
		if err != nil {
			return status.Errorf(codes.Unavailable, "open backend stream: %v", err)
		}

		// Pipe: client → backend and backend → client concurrently
		errCh := make(chan error, 2)
		go pipe(stream, clientStream, errCh)
		go pipe(clientStream, stream, errCh)

		// First error wins (e.g. client disconnects)
		if err := <-errCh; err != nil && err != io.EOF {
			return status.Errorf(codes.Internal, "proxy error: %v", err)
		}
		return nil
	}
}

type sender interface{ SendMsg(m any) error }
type recver interface{ RecvMsg(m any) error }

func pipe(src recver, dst sender, errCh chan<- error) {
	for {
		msg := &rawMsg{}
		if err := src.RecvMsg(msg); err != nil {
			errCh <- err
			return
		}
		if err := dst.SendMsg(msg); err != nil {
			errCh <- err
			return
		}
	}
}

// rawMsg holds an arbitrary gRPC frame as raw bytes so we don't need to
// know the proto schema — this is the key to a transparent proxy.
type rawMsg struct {
	buf []byte
}

func (m *rawMsg) ProtoMessage()             {}
func (m *rawMsg) Reset()                    { m.buf = m.buf[:0] }
func (m *rawMsg) String() string            { return fmt.Sprintf("rawMsg(%d bytes)", len(m.buf)) }
func (m *rawMsg) Marshal() ([]byte, error)  { return m.buf, nil }
func (m *rawMsg) Unmarshal(b []byte) error  { m.buf = append(m.buf[:0], b...); return nil }
func (m *rawMsg) Size() int                 { return len(m.buf) }

// BuildkitDirector creates a Director that routes to builder nodes via the store.
func BuildkitDirector(getBuilder func(ctx context.Context) (string, error)) Director {
	return func(ctx context.Context, fullMethod string) (context.Context, *grpc.ClientConn, error) {
		addr, err := getBuilder(ctx)
		if err != nil {
			return nil, nil, status.Errorf(codes.Unavailable, "no builder: %v", err)
		}
		conn, err := grpc.DialContext(ctx, addr,
			grpc.WithInsecure(), //nolint:staticcheck
			grpc.WithBlock(),
		)
		if err != nil {
			return nil, nil, status.Errorf(codes.Unavailable, "dial builder %s: %v", addr, err)
		}
		log.Printf("proxying %s → %s", fullMethod, addr)
		return ctx, conn, nil
	}
}
```

**Step 3: Wire proxy into server binary**

In `cmd/server/main.go`, add after the HTTP server start:

```go
// gRPC proxy for BuildKit
grpcAddr := fmt.Sprintf(":%s", envOr("GRPC_PORT", "8765"))
p := proxy.New(proxy.BuildkitDirector(func(ctx context.Context) (string, error) {
    builders, err := s.GetHealthyBuilders(ctx)
    if err != nil || len(builders) == 0 {
        return "", fmt.Errorf("no healthy builders")
    }
    return builders[0].Address, nil
}))
grpcSrv := grpc.NewServer(
    grpc.UnknownServiceHandler(p.Handler()),
)
lis, err := net.Listen("tcp", grpcAddr)
if err != nil {
    log.Fatalf("listen grpc: %v", err)
}
log.Printf("buildhive gRPC proxy listening on %s", grpcAddr)
go grpcSrv.Serve(lis)
```

**Step 4: Verify compilation**

```bash
go build ./...
```

**Step 5: Commit**

```bash
git add internal/proxy/ cmd/server/
git commit -m "feat: add transparent gRPC BuildKit proxy"
```

---

## Phase 5: Machine Agent

### Task 10: buildkitd lifecycle management

**Files:**
- Create: `internal/agent/buildkitd.go`
- Create: `internal/agent/buildkitd_test.go`

**Step 1: Write the failing test**

```go
package agent_test

import (
	"testing"
	"github.com/buildhive/buildhive/internal/agent"
)

func TestBuildkitdConfig(t *testing.T) {
	cfg := agent.DefaultBuildkitdConfig("/data/buildkit")
	if cfg.Root == "" {
		t.Error("Root should not be empty")
	}
	if cfg.Addr == "" {
		t.Error("Addr should not be empty")
	}
}
```

**Step 2: Run test to verify it fails**

```bash
go test ./internal/agent/... -v
```

**Step 3: Write `internal/agent/buildkitd.go`**

```go
package agent

import (
	"context"
	"fmt"
	"log"
	"os/exec"
	"time"
)

// BuildkitdConfig holds configuration for the managed buildkitd process.
type BuildkitdConfig struct {
	Root string // persistent cache directory
	Addr string // gRPC listen address (e.g. tcp://0.0.0.0:1234)
}

// DefaultBuildkitdConfig returns a sensible default config.
func DefaultBuildkitdConfig(cacheRoot string) BuildkitdConfig {
	return BuildkitdConfig{
		Root: cacheRoot,
		Addr: "tcp://0.0.0.0:1234",
	}
}

// Manager supervises the buildkitd process with automatic restarts.
type Manager struct {
	cfg  BuildkitdConfig
	cmd  *exec.Cmd
}

// NewManager creates a buildkitd manager.
func NewManager(cfg BuildkitdConfig) *Manager {
	return &Manager{cfg: cfg}
}

// Run starts buildkitd and restarts it if it exits unexpectedly.
// Blocks until ctx is cancelled.
func (m *Manager) Run(ctx context.Context) {
	backoff := time.Second
	for {
		if err := ctx.Err(); err != nil {
			return
		}
		log.Printf("starting buildkitd (root=%s addr=%s)", m.cfg.Root, m.cfg.Addr)
		m.cmd = exec.CommandContext(ctx,
			"buildkitd",
			"--root", m.cfg.Root,
			"--addr", m.cfg.Addr,
			"--oci-worker-no-process-sandbox",
		)
		m.cmd.Stdout = log.Writer()
		m.cmd.Stderr = log.Writer()

		if err := m.cmd.Run(); err != nil {
			if ctx.Err() != nil {
				return // graceful shutdown
			}
			log.Printf("buildkitd exited: %v — restarting in %s", err, backoff)
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
				backoff = min(backoff*2, 30*time.Second)
			}
		} else {
			backoff = time.Second
		}
	}
}

// Addr returns the gRPC address buildkitd is listening on.
func (m *Manager) Addr() string {
	return fmt.Sprintf("%s", m.cfg.Addr)
}

func min(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}
```

**Step 4: Run test to verify it passes**

```bash
go test ./internal/agent/... -v -run TestBuildkitdConfig
```
Expected: PASS

**Step 5: Commit**

```bash
git add internal/agent/
git commit -m "feat: add buildkitd lifecycle manager"
```

---

### Task 11: Agent heartbeat + registration

**Files:**
- Create: `internal/agent/heartbeat.go`
- Modify: `cmd/agent/main.go`

**Step 1: Write `internal/agent/heartbeat.go`**

```go
package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"runtime"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/mem"
)

// HeartbeatConfig configures the agent heartbeat.
type HeartbeatConfig struct {
	ServerURL    string
	AgentName    string
	BuildkitAddr string // public address the control plane can reach (host:port)
	Interval     time.Duration
}

// Heartbeater sends periodic health reports to the control plane.
type Heartbeater struct {
	cfg    HeartbeatConfig
	client *http.Client
}

// NewHeartbeater creates a Heartbeater.
func NewHeartbeater(cfg HeartbeatConfig) *Heartbeater {
	return &Heartbeater{cfg: cfg, client: &http.Client{Timeout: 5 * time.Second}}
}

type heartbeatPayload struct {
	Name    string  `json:"name"`
	Address string  `json:"address"`
	Arch    string  `json:"arch"`
	Status  string  `json:"status"`
	CPUPct  float64 `json:"cpu_pct"`
	MemPct  float64 `json:"mem_pct"`
	DiskPct float64 `json:"disk_pct"`
}

// Run registers the builder and sends heartbeats until ctx is cancelled.
func (h *Heartbeater) Run(ctx context.Context) {
	ticker := time.NewTicker(h.cfg.Interval)
	defer ticker.Stop()

	for {
		if err := h.send(ctx); err != nil {
			log.Printf("heartbeat error: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (h *Heartbeater) send(ctx context.Context) error {
	payload := h.collect()
	body, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("%s/api/builders/heartbeat", h.cfg.ServerURL),
		bytes.NewReader(body),
	)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := h.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func (h *Heartbeater) collect() heartbeatPayload {
	p := heartbeatPayload{
		Name:    h.cfg.AgentName,
		Address: h.cfg.BuildkitAddr,
		Arch:    runtime.GOARCH,
		Status:  "healthy",
	}
	if cpus, err := cpu.Percent(0, false); err == nil && len(cpus) > 0 {
		p.CPUPct = cpus[0]
	}
	if vm, err := mem.VirtualMemory(); err == nil {
		p.MemPct = vm.UsedPercent
	}
	if diskStat, err := disk.Usage(os.Getenv("CACHE_ROOT")); err == nil {
		p.DiskPct = diskStat.UsedPercent
		if diskStat.UsedPercent > 90 {
			p.Status = "disk_pressure"
		}
	}
	return p
}
```

**Step 2: Add gopsutil dependency**

```bash
go get github.com/shirou/gopsutil/v3
```

**Step 3: Implement `cmd/agent/main.go`**

```go
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/buildhive/buildhive/internal/agent"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cacheRoot := envOr("CACHE_ROOT", "/var/lib/buildkit")
	serverURL := mustEnv("BUILDHIVE_SERVER_URL")
	agentName := envOr("AGENT_NAME", mustHostname())
	buildkitAddr := mustEnv("BUILDKIT_PUBLIC_ADDR") // e.g. "192.168.1.10:1234"

	// Start buildkitd
	mgr := agent.NewManager(agent.DefaultBuildkitdConfig(cacheRoot))
	go mgr.Run(ctx)

	// Wait a moment for buildkitd to start
	time.Sleep(2 * time.Second)

	// Start heartbeat
	hb := agent.NewHeartbeater(agent.HeartbeatConfig{
		ServerURL:    serverURL,
		AgentName:    agentName,
		BuildkitAddr: buildkitAddr,
		Interval:     10 * time.Second,
	})
	go hb.Run(ctx)

	log.Printf("buildhive-agent running (name=%s, buildkit=%s)", agentName, buildkitAddr)
	<-ctx.Done()
	log.Println("shutting down")
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("required: %s", key)
	}
	return v
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func mustHostname() string {
	h, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return h
}
```

**Step 4: Build and verify**

```bash
make build-agent
./bin/buildhive-agent --help 2>&1 || true
go build ./...
```

**Step 5: Commit**

```bash
git add internal/agent/ cmd/agent/
git commit -m "feat: add agent heartbeat + buildkitd manager"
```

---

## Phase 6: CLI

### Task 12: CLI framework + login command

**Files:**
- Create: `internal/cli/config.go`
- Create: `cmd/cli/main.go`
- Create: `internal/cli/cmd/root.go`
- Create: `internal/cli/cmd/login.go`

**Step 1: Write `internal/cli/config.go`**

```go
package cli

import (
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

const configFile = "config.yaml"

func ConfigDir() string {
	h, _ := os.UserHomeDir()
	return filepath.Join(h, ".buildhive")
}

func LoadConfig() {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(ConfigDir())
	viper.ReadInConfig()
}

func SaveConfig() error {
	dir := ConfigDir()
	os.MkdirAll(dir, 0700)
	return viper.WriteConfigAs(filepath.Join(dir, configFile))
}
```

**Step 2: Write root command**

`internal/cli/cmd/root.go`:
```go
package cmd

import (
	"github.com/buildhive/buildhive/internal/cli"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "buildhive",
	Short: "BuildHive — remote Docker build acceleration",
}

func Execute() error {
	cli.LoadConfig()
	return rootCmd.Execute()
}

func init() {
	rootCmd.AddCommand(loginCmd)
	rootCmd.AddCommand(buildCmd)
	rootCmd.AddCommand(projectsCmd)
	rootCmd.AddCommand(buildersCmd)
	rootCmd.AddCommand(buildsCmd)
}
```

**Step 3: Write login command**

`internal/cli/cmd/login.go`:
```go
package cmd

import (
	"fmt"

	"github.com/buildhive/buildhive/internal/cli"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Authenticate with a BuildHive server",
	RunE: func(cmd *cobra.Command, args []string) error {
		serverURL, _ := cmd.Flags().GetString("server")
		token, _ := cmd.Flags().GetString("token")

		viper.Set("server_url", serverURL)
		viper.Set("token", token)

		if err := cli.SaveConfig(); err != nil {
			return fmt.Errorf("save config: %w", err)
		}
		fmt.Printf("Logged in to %s\n", serverURL)
		return nil
	},
}

func init() {
	loginCmd.Flags().String("server", "http://localhost:8080", "BuildHive server URL")
	loginCmd.Flags().String("token", "", "Admin or project API token")
	loginCmd.MarkFlagRequired("token")
}
```

**Step 4: Wire `cmd/cli/main.go`**

```go
package main

import (
	"log"
	"github.com/buildhive/buildhive/internal/cli/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		log.Fatal(err)
	}
}
```

**Step 5: Build and test login**

```bash
make build-cli
./bin/buildhive login --server http://localhost:8080 --token testtoken
cat ~/.buildhive/config.yaml
```
Expected: config file created with server_url and token.

**Step 6: Commit**

```bash
git add internal/cli/ cmd/cli/
git commit -m "feat: add CLI framework with login command"
```

---

### Task 13: `buildhive build` command

**Files:**
- Create: `internal/cli/cmd/build.go`

**Step 1: Write `internal/cli/cmd/build.go`**

```go
package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var buildCmd = &cobra.Command{
	Use:   "build [flags] PATH",
	Short: "Build a Docker image remotely using BuildHive",
	Args:  cobra.ExactArgs(1),
	RunE:  runBuild,
}

func init() {
	buildCmd.Flags().StringP("tag", "t", "", "Image name and tag (required)")
	buildCmd.Flags().String("platform", "", "Target platform (e.g. linux/amd64,linux/arm64)")
	buildCmd.Flags().StringArray("build-arg", nil, "Build arguments")
	buildCmd.Flags().String("file", "Dockerfile", "Dockerfile path")
	buildCmd.MarkFlagRequired("tag")
}

func runBuild(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	contextPath := args[0]
	tag, _ := cmd.Flags().GetString("tag")
	platform, _ := cmd.Flags().GetString("platform")
	buildArgs, _ := cmd.Flags().GetStringArray("build-arg")
	file, _ := cmd.Flags().GetString("file")

	serverURL := viper.GetString("server_url")
	token := viper.GetString("token")
	if serverURL == "" || token == "" {
		return fmt.Errorf("not logged in — run: buildhive login")
	}

	// Step 1: Init build on control plane
	body, _ := json.Marshal(map[string]string{"tag": tag})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost,
		serverURL+"/api/builds/init", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("init build: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned %d", resp.StatusCode)
	}

	var initResp struct {
		BuildID         string `json:"build_id"`
		BuilderEndpoint string `json:"builder_endpoint"`
	}
	json.NewDecoder(resp.Body).Decode(&initResp)

	fmt.Printf("BuildHive: build %s → builder %s\n", initResp.BuildID, initResp.BuilderEndpoint)

	// Step 2: Configure buildx remote driver
	builderName := "buildhive-" + initResp.BuildID[:8]
	exec.CommandContext(ctx, "docker", "buildx", "rm", builderName).Run() // ignore error if not exists
	createCmd := exec.CommandContext(ctx, "docker", "buildx", "create",
		"--driver", "remote",
		"--name", builderName,
		"tcp://"+initResp.BuilderEndpoint,
	)
	createCmd.Stdout = os.Stdout
	createCmd.Stderr = os.Stderr
	if err := createCmd.Run(); err != nil {
		return fmt.Errorf("create buildx builder: %w", err)
	}
	defer exec.Command("docker", "buildx", "rm", builderName).Run()

	// Step 3: Run docker buildx build
	bArgs := []string{"buildx", "build",
		"--builder", builderName,
		"-t", tag,
		"-f", file,
	}
	if platform != "" {
		bArgs = append(bArgs, "--platform", platform)
	}
	for _, a := range buildArgs {
		bArgs = append(bArgs, "--build-arg", a)
	}
	bArgs = append(bArgs, contextPath)

	buildxCmd := exec.CommandContext(ctx, "docker", bArgs...)
	buildxCmd.Stdout = os.Stdout
	buildxCmd.Stderr = os.Stderr
	return buildxCmd.Run()
}
```

**Step 2: Build and do a dry-run (no real builder needed)**

```bash
make build-cli
./bin/buildhive build --help
```
Expected: help text with all flags shown.

**Step 3: Commit**

```bash
git add internal/cli/cmd/build.go
git commit -m "feat: add build command with remote buildx integration"
```

---

### Task 14: Remaining CLI commands (projects, builders, builds)

**Files:**
- Create: `internal/cli/cmd/projects.go`
- Create: `internal/cli/cmd/builders.go`
- Create: `internal/cli/cmd/builds.go`
- Create: `internal/cli/client.go`

**Step 1: Write a shared API client**

`internal/cli/client.go`:
```go
package cli

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/spf13/viper"
)

// Get makes an authenticated GET request to the BuildHive API.
func Get(path string, out any) error {
	req, err := http.NewRequest(http.MethodGet, viper.GetString("server_url")+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+viper.GetString("token"))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("server returned %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
```

**Step 2: Write `internal/cli/cmd/builders.go`**

```go
package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/buildhive/buildhive/internal/cli"
	"github.com/spf13/cobra"
)

var buildersCmd = &cobra.Command{
	Use:   "builders",
	Short: "Manage builder nodes",
}

var buildersListCmd = &cobra.Command{
	Use:   "list",
	Short: "List registered builders",
	RunE: func(cmd *cobra.Command, args []string) error {
		var builders []map[string]any
		if err := cli.Get("/api/builders", &builders); err != nil {
			return err
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		for _, b := range builders {
			fmt.Printf("%-20s %-15s %s\n", b["name"], b["status"], b["address"])
		}
		return nil
	},
}

func init() {
	buildersCmd.AddCommand(buildersListCmd)
}
```

**Step 3: Build and verify**

```bash
make build-cli && ./bin/buildhive builders list --help
```

**Step 4: Commit**

```bash
git add internal/cli/
git commit -m "feat: add projects, builders, builds CLI commands"
```

---

## Phase 7: Web UI

### Task 15: React + Vite + shadcn/ui setup

**Files:**
- Create: `web/` (Vite project)

**Step 1: Scaffold Vite + React project**

```bash
cd ~/development/buildhive
npm create vite@latest web -- --template react-ts
cd web && npm install
```

**Step 2: Install shadcn/ui + Tailwind**

```bash
npm install -D tailwindcss postcss autoprefixer
npx tailwindcss init -p
npm install @radix-ui/react-slot class-variance-authority clsx tailwind-merge lucide-react
npx shadcn-ui@latest init
```

Choose: TypeScript, default style, default base color, CSS variables.

**Step 3: Install additional deps**

```bash
npm install react-router-dom axios
npm install -D @types/node
```

**Step 4: Update `web/vite.config.ts`**

```ts
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import path from 'path'

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: { '@': path.resolve(__dirname, './src') },
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
  },
  server: {
    proxy: {
      '/api': 'http://localhost:8080',
    },
  },
})
```

**Step 5: Verify Vite dev server starts**

```bash
cd web && npm run dev
```
Expected: Vite dev server at http://localhost:5173

**Step 6: Commit**

```bash
cd ~/development/buildhive
git add web/
git commit -m "feat: scaffold React + Vite + shadcn/ui web UI"
```

---

### Task 16: Dashboard, Projects, and Builds pages

**Files:**
- Create: `web/src/pages/Dashboard.tsx`
- Create: `web/src/pages/Projects.tsx`
- Create: `web/src/pages/Builds.tsx`
- Create: `web/src/lib/api.ts`
- Modify: `web/src/App.tsx`

**Step 1: Write `web/src/lib/api.ts`**

```ts
import axios from 'axios'

const client = axios.create({ baseURL: '/api' })

client.interceptors.request.use((config) => {
  const token = localStorage.getItem('buildhive_token')
  if (token) config.headers.Authorization = `Bearer ${token}`
  return config
})

export const api = {
  getBuilders: () => client.get('/builders').then(r => r.data),
  getBuilds: () => client.get('/builds').then(r => r.data),
  getProjects: () => client.get('/projects').then(r => r.data),
  createProject: (name: string, slug: string) =>
    client.post('/projects', { name, slug }).then(r => r.data),
  createToken: (projectId: string, label: string) =>
    client.post(`/projects/${projectId}/tokens`, { label }).then(r => r.data),
  getMetrics: () => client.get('/metrics').then(r => r.data),
}
```

**Step 2: Write `web/src/pages/Dashboard.tsx`**

```tsx
import { useEffect, useState } from 'react'
import { api } from '@/lib/api'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'

export default function Dashboard() {
  const [builders, setBuilders] = useState<any[]>([])
  const [metrics, setMetrics] = useState<any>({})

  useEffect(() => {
    api.getBuilders().then(setBuilders).catch(console.error)
    api.getMetrics().then(setMetrics).catch(console.error)
  }, [])

  return (
    <div className="p-6 space-y-6">
      <h1 className="text-2xl font-bold">Dashboard</h1>

      <div className="grid grid-cols-3 gap-4">
        <Card>
          <CardHeader><CardTitle>Builders</CardTitle></CardHeader>
          <CardContent>
            <p className="text-3xl font-bold">{builders.length}</p>
          </CardContent>
        </Card>
        <Card>
          <CardHeader><CardTitle>Builds (24h)</CardTitle></CardHeader>
          <CardContent>
            <p className="text-3xl font-bold">{metrics.builds_24h ?? '-'}</p>
          </CardContent>
        </Card>
        <Card>
          <CardHeader><CardTitle>Cache Hit Rate</CardTitle></CardHeader>
          <CardContent>
            <p className="text-3xl font-bold">{metrics.cache_hit_rate ?? '-'}%</p>
          </CardContent>
        </Card>
      </div>

      <div>
        <h2 className="text-xl font-semibold mb-3">Builder Nodes</h2>
        <div className="space-y-2">
          {builders.map((b: any) => (
            <Card key={b.id}>
              <CardContent className="flex items-center justify-between py-3">
                <span className="font-mono">{b.name}</span>
                <span className="text-sm text-muted-foreground">{b.address}</span>
                <Badge variant={b.status === 'healthy' ? 'default' : 'destructive'}>
                  {b.status}
                </Badge>
              </CardContent>
            </Card>
          ))}
        </div>
      </div>
    </div>
  )
}
```

**Step 3: Write `web/src/pages/Builds.tsx`** (with SSE log streaming)

```tsx
import { useEffect, useRef, useState } from 'react'
import { api } from '@/lib/api'

export default function Builds() {
  const [builds, setBuilds] = useState<any[]>([])
  const [selectedBuild, setSelectedBuild] = useState<string | null>(null)
  const [logs, setLogs] = useState<string[]>([])
  const logsRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    api.getBuilds().then(setBuilds).catch(console.error)
  }, [])

  useEffect(() => {
    if (!selectedBuild) return
    setLogs([])
    const token = localStorage.getItem('buildhive_token')
    const es = new EventSource(`/api/builds/${selectedBuild}/logs?token=${token}`)
    es.onmessage = (e) => {
      setLogs(prev => [...prev, e.data])
      logsRef.current?.scrollTo(0, logsRef.current.scrollHeight)
    }
    return () => es.close()
  }, [selectedBuild])

  return (
    <div className="p-6 flex gap-6 h-screen">
      <div className="w-1/3 space-y-2 overflow-y-auto">
        <h1 className="text-2xl font-bold mb-4">Builds</h1>
        {builds.map((b: any) => (
          <div
            key={b.id}
            onClick={() => setSelectedBuild(b.id)}
            className={`p-3 rounded border cursor-pointer hover:bg-accent
              ${selectedBuild === b.id ? 'border-primary' : 'border-border'}`}
          >
            <p className="font-mono text-sm truncate">{b.id}</p>
            <p className="text-xs text-muted-foreground">{b.status}</p>
          </div>
        ))}
      </div>
      <div ref={logsRef} className="flex-1 bg-black rounded p-4 overflow-y-auto font-mono text-green-400 text-sm">
        {logs.map((line, i) => <div key={i}>{line}</div>)}
      </div>
    </div>
  )
}
```

**Step 4: Update `web/src/App.tsx`**

```tsx
import { BrowserRouter, Routes, Route, Link } from 'react-router-dom'
import Dashboard from './pages/Dashboard'
import Projects from './pages/Projects'
import Builds from './pages/Builds'

export default function App() {
  return (
    <BrowserRouter>
      <div className="flex h-screen">
        <aside className="w-48 border-r p-4 flex flex-col gap-2">
          <h1 className="font-bold text-lg mb-4">BuildHive</h1>
          <Link to="/" className="hover:underline">Dashboard</Link>
          <Link to="/projects" className="hover:underline">Projects</Link>
          <Link to="/builds" className="hover:underline">Builds</Link>
        </aside>
        <main className="flex-1 overflow-y-auto">
          <Routes>
            <Route path="/" element={<Dashboard />} />
            <Route path="/projects" element={<Projects />} />
            <Route path="/builds" element={<Builds />} />
          </Routes>
        </main>
      </div>
    </BrowserRouter>
  )
}
```

**Step 5: Build the UI**

```bash
cd web && npm run build
```
Expected: `web/dist/` populated with static assets.

**Step 6: Commit**

```bash
cd ~/development/buildhive
git add web/
git commit -m "feat: add Dashboard, Projects, and Builds pages"
```

---

## Phase 8: Embed UI + Docker + GitHub Action

### Task 17: Embed React bundle in server binary

**Files:**
- Create: `internal/api/static.go`
- Modify: `internal/api/server.go`

**Step 1: Write `internal/api/static.go`**

```go
package api

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed all:../../web/dist
var staticFiles embed.FS

func staticHandler() http.Handler {
	dist, _ := fs.Sub(staticFiles, "web/dist")
	return http.FileServer(http.FS(dist))
}
```

**Step 2: Add static serving to router**

In `server.go` `buildRouter()`, add before the API routes:
```go
// Serve React SPA
r.Handle("/*", staticHandler())
```

**Step 3: Build and verify**

```bash
cd web && npm run build && cd ..
make build-server
DATABASE_URL="postgres://buildhive:buildhive@localhost:5432/buildhive?sslmode=disable" \
  BUILDHIVE_ADMIN_TOKEN=devtoken \
  ./bin/buildhive-server &
curl http://localhost:8080/
```
Expected: HTML response (React app).

**Step 4: Kill server, commit**

```bash
kill %1
git add internal/api/static.go internal/api/server.go
git commit -m "feat: embed React build into server binary via go:embed"
```

---

### Task 18: Dockerfiles + docker-compose for full stack

**Files:**
- Create: `Dockerfile.server`
- Create: `Dockerfile.agent`
- Modify: `docker-compose.yml`

**Step 1: Write `Dockerfile.server`**

```dockerfile
FROM node:20-alpine AS web-builder
WORKDIR /app/web
COPY web/package*.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.22-alpine AS go-builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web-builder /app/web/dist ./web/dist
RUN go build -trimpath -ldflags="-s -w" -o bin/buildhive-server ./cmd/server

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY --from=go-builder /app/bin/buildhive-server /usr/local/bin/
ENTRYPOINT ["buildhive-server"]
```

**Step 2: Write `Dockerfile.agent`**

```dockerfile
FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -trimpath -ldflags="-s -w" -o bin/buildhive-agent ./cmd/agent

FROM moby/buildkit:latest
COPY --from=builder /app/bin/buildhive-agent /usr/local/bin/
ENTRYPOINT ["buildhive-agent"]
```

**Step 3: Update `docker-compose.yml` for full stack**

```yaml
version: "3.9"
services:
  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_DB: buildhive
      POSTGRES_USER: buildhive
      POSTGRES_PASSWORD: buildhive
    ports:
      - "5432:5432"
    volumes:
      - pgdata:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U buildhive"]
      interval: 5s
      timeout: 5s
      retries: 5

  server:
    build:
      context: .
      dockerfile: Dockerfile.server
    environment:
      DATABASE_URL: postgres://buildhive:buildhive@postgres:5432/buildhive?sslmode=disable
      BUILDHIVE_ADMIN_TOKEN: ${ADMIN_TOKEN:-changeme}
      PORT: 8080
      GRPC_PORT: 8765
    ports:
      - "8080:8080"
      - "8765:8765"
    depends_on:
      postgres:
        condition: service_healthy

  agent:
    build:
      context: .
      dockerfile: Dockerfile.agent
    privileged: true  # required for buildkitd
    environment:
      BUILDHIVE_SERVER_URL: http://server:8080
      BUILDKIT_PUBLIC_ADDR: ${AGENT_PUBLIC_IP}:1234
      CACHE_ROOT: /var/lib/buildkit
    volumes:
      - buildkit-cache:/var/lib/buildkit
    depends_on:
      - server

volumes:
  pgdata:
  buildkit-cache:
```

**Step 4: Build and verify**

```bash
AGENT_PUBLIC_IP=127.0.0.1 docker compose up --build -d
docker compose ps
curl http://localhost:8080/healthz
```
Expected: all containers healthy, health check returns `{"status":"ok"}`

**Step 5: Commit**

```bash
git add Dockerfile.* docker-compose.yml
git commit -m "feat: add Dockerfiles and full-stack docker-compose"
```

---

### Task 19: GitHub Action

**Files:**
- Create: `action/action.yml`
- Create: `action/entrypoint.sh`

**Step 1: Write `action/action.yml`**

```yaml
name: 'BuildHive Build'
description: 'Build and push Docker images using BuildHive remote builders'
author: 'BuildHive'

inputs:
  server-url:
    description: 'BuildHive server URL'
    required: true
  token:
    description: 'BuildHive project API token'
    required: true
  tags:
    description: 'Image name and tags (same as docker/metadata-action output)'
    required: true
  context:
    description: 'Build context path'
    default: '.'
  file:
    description: 'Dockerfile path'
    default: 'Dockerfile'
  platforms:
    description: 'Target platforms (e.g. linux/amd64,linux/arm64)'
    default: 'linux/amd64'
  push:
    description: 'Push image after build'
    default: 'false'

runs:
  using: 'composite'
  steps:
    - name: Install BuildHive CLI
      shell: bash
      run: |
        curl -fsSL ${{ inputs.server-url }}/install.sh | sh
        echo "$HOME/.local/bin" >> $GITHUB_PATH

    - name: Login to BuildHive
      shell: bash
      run: |
        buildhive login \
          --server "${{ inputs.server-url }}" \
          --token "${{ inputs.token }}"

    - name: Build (and optionally push) image
      shell: bash
      run: |
        PUSH_FLAG=""
        if [ "${{ inputs.push }}" = "true" ]; then PUSH_FLAG="--push"; fi
        buildhive build \
          -t "${{ inputs.tags }}" \
          -f "${{ inputs.file }}" \
          --platform "${{ inputs.platforms }}" \
          $PUSH_FLAG \
          "${{ inputs.context }}"
```

**Step 2: Commit**

```bash
git add action/
git commit -m "feat: add GitHub Action for BuildHive builds"
```

---

## Phase 9: Integration Tests

### Task 20: Integration tests with testcontainers

**Files:**
- Create: `tests/integration/api_test.go`

**Step 1: Add testcontainers dependency**

```bash
go get github.com/testcontainers/testcontainers-go
go get github.com/testcontainers/testcontainers-go/modules/postgres
```

**Step 2: Write `tests/integration/api_test.go`**

```go
//go:build integration

package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/buildhive/buildhive/internal/api"
	"github.com/buildhive/buildhive/internal/auth"
	"github.com/buildhive/buildhive/internal/store"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

func setupTestServer(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	ctx := context.Background()

	pgCtr, err := tcpostgres.RunContainer(ctx,
		tcpostgres.WithDatabase("testdb"),
		tcpostgres.WithUsername("test"),
		tcpostgres.WithPassword("test"),
	)
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	t.Cleanup(func() { pgCtr.Terminate(ctx) })

	dsn, _ := pgCtr.ConnectionString(ctx, "sslmode=disable")
	s, err := store.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect store: %v", err)
	}
	t.Cleanup(s.Close)

	const adminToken = "test-admin-token"
	hash, _ := auth.HashToken(adminToken)
	srv := api.New(api.Config{AdminTokenHash: hash}, s)
	return httptest.NewServer(srv), adminToken
}

func TestCreateAndListProjects(t *testing.T) {
	srv, token := setupTestServer(t)
	defer srv.Close()

	// Create project
	body, _ := json.Marshal(map[string]string{"name": "My App", "slug": "my-app"})
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/projects", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusCreated {
		t.Fatalf("create project: status=%d err=%v", resp.StatusCode, err)
	}

	// List projects
	req2, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/projects", nil)
	req2.Header.Set("Authorization", "Bearer "+token)
	resp2, _ := http.DefaultClient.Do(req2)
	var projects []map[string]any
	json.NewDecoder(resp2.Body).Decode(&projects)
	if len(projects) != 1 {
		t.Errorf("expected 1 project, got %d", len(projects))
	}
}

func TestUnauthorizedRejects(t *testing.T) {
	srv, _ := setupTestServer(t)
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/projects", nil)
	resp, _ := http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}
}
```

**Step 3: Run integration tests**

```bash
go test -tags integration ./tests/integration/... -v
```
Expected: PASS — all tests green.

**Step 4: Add integration test to Makefile**

```makefile
test-integration:
	$(GO) test -tags integration ./tests/integration/... -v
```

**Step 5: Commit**

```bash
git add tests/ Makefile
git commit -m "test: add integration tests with testcontainers-go"
```

---

## Phase 10: CI Pipeline

### Task 21: GitHub Actions CI workflow

**Files:**
- Create: `.github/workflows/ci.yml`

**Step 1: Write `.github/workflows/ci.yml`**

```yaml
name: CI

on:
  push:
    branches: [main]
  pull_request:
    branches: [main]

jobs:
  lint:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: '1.22' }
      - name: golangci-lint
        uses: golangci/golangci-lint-action@v4

  test:
    runs-on: ubuntu-latest
    services:
      postgres:
        image: postgres:16-alpine
        env:
          POSTGRES_DB: buildhive
          POSTGRES_USER: buildhive
          POSTGRES_PASSWORD: buildhive
        options: >-
          --health-cmd pg_isready
          --health-interval 10s
          --health-timeout 5s
          --health-retries 5
        ports:
          - 5432:5432
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: '1.22' }
      - name: Run unit tests
        run: go test ./... -v
      - name: Run integration tests
        env:
          DATABASE_URL: postgres://buildhive:buildhive@localhost:5432/buildhive?sslmode=disable
        run: go test -tags integration ./tests/integration/... -v

  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: '1.22' }
      - uses: actions/setup-node@v4
        with: { node-version: '20' }
      - name: Build web UI
        run: cd web && npm ci && npm run build
      - name: Build Go binaries
        run: make build
      - name: Upload binaries
        uses: actions/upload-artifact@v4
        with:
          name: buildhive-binaries
          path: bin/
```

**Step 2: Commit**

```bash
mkdir -p .github/workflows
git add .github/
git commit -m "ci: add GitHub Actions lint + test + build pipeline"
```

---

## Final Verification Checklist

Before marking the project complete, verify each of the following:

```bash
# 1. All unit tests pass
go test ./... -v

# 2. All integration tests pass
go test -tags integration ./tests/integration/... -v

# 3. All three binaries build
make build
ls -la bin/

# 4. Web UI builds
cd web && npm run build && cd ..

# 5. Full stack comes up
AGENT_PUBLIC_IP=127.0.0.1 docker compose up --build -d
curl http://localhost:8080/healthz
curl http://localhost:8765  # gRPC proxy listening

# 6. End-to-end: login + build (requires real builder running)
./bin/buildhive login --server http://localhost:8080 --token changeme
./bin/buildhive builders list
./bin/buildhive projects create --name test --slug test
```
