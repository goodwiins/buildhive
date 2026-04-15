# BuildHive — Design Document

**Date:** 2026-04-15  
**Status:** Approved  
**Scope:** Full open-source, self-hostable remote Docker build acceleration platform (Depot alternative)

---

## Overview

BuildHive is a self-hosted remote Docker build platform. Instead of building container images locally or on slow CI runners, builds run on a dedicated builder node with a persistent BuildKit layer cache, native multi-arch support, and a shared cache across all developers and CI pipelines.

**Core value:** Drop-in replacement for `docker build` / `docker/build-push-action` that is fully open-source, self-hostable, and requires no vendor account.

---

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│  Developer Machine / GitHub Actions                             │
│   buildhive CLI  ──────────────────────────────────────────┐   │
│   (docker buildx driver: remote)                            │   │
└─────────────────────────────────────────────────────────────┼───┘
                                                              │ gRPC (BuildKit protocol)
                                    ┌─────────────────────────▼───┐
                                    │   Control Plane (Go)         │
                                    │   REST API  + gRPC Proxy     │
                                    │   Auth / Projects / Routing  │
                                    │   PostgreSQL                 │
                                    └──────────────┬──────────────┘
                                                   │ forwards gRPC stream
                              ┌────────────────────▼────────────────────┐
                              │  Builder Node (AMD 8vCPU/16GB/320GB)    │
                              │  Machine Agent (Go)                     │
                              │  ├─ manages buildkitd                   │
                              │  ├─ heartbeat to control plane          │
                              │  └─ exposes :1234 (BuildKit gRPC)       │
                              │                                         │
                              │  320 GB disk → persistent layer cache   │
                              └─────────────────────────────────────────┘

  Web UI (React/Vite)  ──── REST API ──── Control Plane
  GitHub Action        ──── CLI ──────────────────────
```

### Deployable Components

| Component | Language | Artifact | Runs on |
|---|---|---|---|
| `buildhive-server` | Go | single binary | Any Linux VPS / same node |
| `buildhive-agent` | Go | single binary | Builder node |
| `buildhive` CLI | Go | single binary | Dev machine / CI |
| Web UI | React + Vite | embedded in server binary | Browser |

The server binary embeds the compiled React bundle via `go:embed` — one process, one port, no separate static file server.

---

## Components

### Control Plane (`buildhive-server`)

**REST API:**
```
POST   /api/auth/login
GET    /api/projects
POST   /api/projects
DELETE /api/projects/:id
POST   /api/projects/:id/tokens
GET    /api/builders
GET    /api/builds
GET    /api/builds/:id/logs     (SSE streaming)
GET    /api/metrics
```

**gRPC proxy:**
- Listens on `:8765` (BuildKit wire protocol)
- Authenticates via ephemeral token in gRPC metadata
- Routes to least-loaded healthy builder
- Pure bidirectional stream passthrough — no build logic in control plane

### Machine Agent (`buildhive-agent`)

- Starts and supervises `buildkitd` (persistent cache on 320 GB disk)
- Heartbeats every 10s to control plane with: CPU %, mem %, disk %, active build count
- Reports build lifecycle events (start, finish, cache hit/miss) via REST
- Outbound-only connections — no inbound ports required on the agent

### CLI (`buildhive`)

```bash
buildhive login                         # saves token + server URL → ~/.buildhive/config.yaml
buildhive projects list|create|delete
buildhive build -t registry/myapp:v1 .  # wraps docker buildx build with remote endpoint
buildhive builders list
buildhive builds list --project myapp
```

Build flow:
1. `POST /api/builds/init` → receives `{ builder_endpoint, build_id, ephemeral_token }`
2. Configures `docker buildx` remote driver with that endpoint
3. Delegates to `docker buildx build` — BuildKit handles all build logic

### Web UI (React + Vite + shadcn/ui)

| Page | Content |
|---|---|
| Dashboard | Builder health, active builds, cache hit rate, 24h build count |
| Projects | CRUD projects, generate/revoke API tokens |
| Builds | History table; click row → live log stream via SSE |

---

## Data Model (PostgreSQL)

```sql
projects    (id, name, slug, created_at)
api_tokens  (id, project_id, token_hash, label, last_used_at)
builders    (id, name, address, arch, last_seen_at, status)
builds      (id, project_id, builder_id, status, started_at, finished_at, cache_hit, image_ref)
build_logs  (id, build_id, timestamp, line)
```

---

## Auth Model

| Token type | Scope | Lifetime |
|---|---|---|
| Admin token | Web UI login, project CRUD | Static, set via `BUILDHIVE_ADMIN_TOKEN` env var |
| Project API token | Trigger builds for one project | Long-lived, revocable |
| Ephemeral build token | Authenticate one build's gRPC stream | 1 hour |

Tokens stored as bcrypt hashes — never in plaintext.

---

## Build Data Flow

```
1. CLI: POST /api/builds/init (project token) → { builder_endpoint, build_id, ephemeral_token }
2. CLI: docker buildx create --driver remote --name buildhive <builder_endpoint>
3. CLI: docker buildx build → gRPC stream → control plane proxy
4. Proxy: validates ephemeral token → forwards stream to buildkitd on builder node
5. buildkitd: executes build, hits persistent cache on 320 GB disk
6. Agent: POSTs build completion event → control plane updates builds table + build_logs
```

---

## Error Handling

| Scenario | Behavior |
|---|---|
| Builder offline | 3 missed heartbeats → `unhealthy`; new builds queued |
| Ephemeral token expired | Proxy returns `UNAUTHENTICATED`; CLI shows clear error |
| Disk pressure (>90%) | Agent flags `disk_pressure`; control plane stops routing; UI alerts |
| buildkitd crash | Agent restarts with exponential backoff; reports `restarting` status |
| Control plane unreachable | CLI fails fast: "cannot reach BuildHive server at <url>" |

---

## Repository Layout

```
buildhive/
├── cmd/
│   ├── server/          # buildhive-server entrypoint
│   ├── agent/           # buildhive-agent entrypoint
│   └── cli/             # buildhive CLI entrypoint
├── internal/
│   ├── api/             # REST handlers (Chi router)
│   ├── proxy/           # gRPC BuildKit proxy
│   ├── store/           # PostgreSQL queries (sqlc generated)
│   ├── agent/           # agent logic (heartbeat, buildkitd mgmt)
│   └── auth/            # token hashing, validation
├── web/                 # React + Vite source
│   └── dist/            # compiled, embedded into server binary
├── schema/              # SQL migrations (golang-migrate)
├── action/              # GitHub Action YAML + entrypoint script
├── docker-compose.yml   # local dev: server + postgres
├── Makefile
└── go.mod
```

---

## Testing Strategy

| Layer | What | Tool |
|---|---|---|
| Unit | Token auth, routing logic, builder selection | `go test` |
| Integration | REST API against real PostgreSQL | `testcontainers-go` |
| E2E | Full build: CLI → proxy → real buildkitd | Docker-in-Docker in CI |
| Frontend | Dashboard rendering, SSE log stream | Vitest + React Testing Library |

CI pipeline: lint → unit → integration → build binaries → build Docker image.

---

## Key Technical Decisions

- **No Redis** — PostgreSQL is sufficient; SSE handles log streaming without pub/sub
- **Single Go module** — three binaries from one `go build`, easy cross-compilation
- **go:embed** — React bundle embedded in server binary, zero runtime dependencies
- **sqlc** — type-safe DB queries generated from SQL, no ORM
- **Chi** — lightweight HTTP router, idiomatic Go
- **golang-migrate** — SQL migrations tracked in version control
- **Horizontal scaling** — add more builder nodes by deploying more agents; control plane routes automatically
