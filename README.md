# BuildHive

Open-source, self-hostable remote Docker build acceleration platform — a [Depot](https://depot.dev) alternative you can run on your own hardware.

Instead of building Docker images locally or on slow CI runners, BuildHive routes builds to a dedicated machine with a persistent BuildKit layer cache and native multi-arch support.

## Features

- **Remote builds** — `buildhive build -t myapp .` sends your build to a powerful remote builder
- **Persistent cache** — BuildKit layer cache lives on the builder's disk across all builds
- **Native multi-arch** — build `linux/amd64` + `linux/arm64` without QEMU emulation
- **Shared cache** — every developer and CI pipeline hits the same cache
- **Web dashboard** — monitor builder health, active builds, cache hit rate, and live logs
- **GitHub Action** — drop-in replacement for `docker/build-push-action`
- **Fully self-hostable** — no vendor account, runs on your own machine

## Architecture

```
Developer / GitHub Actions
  buildhive CLI ──────────────────────────────────────────┐
  (docker buildx remote driver)                           │ gRPC
                                              ┌───────────▼──────────┐
                                              │  buildhive-server     │
                                              │  REST API + gRPC Proxy│
                                              │  PostgreSQL           │
                                              └───────────┬──────────┘
                                                          │
                                          ┌───────────────▼───────────────┐
                                          │  Builder Node                 │
                                          │  buildhive-agent              │
                                          │  └─ manages buildkitd         │
                                          │  320 GB persistent cache      │
                                          └───────────────────────────────┘
Web UI (React) ──── REST API ──── buildhive-server
```

Three Go binaries, one module:

| Binary | Role |
|---|---|
| `buildhive-server` | REST API + gRPC proxy + embedded React UI |
| `buildhive-agent` | Manages `buildkitd` on builder node, sends heartbeats |
| `buildhive` | CLI — login, build, projects, builders |

## Quick Start

### 1. Start the server + database

```bash
cp .env.example .env
# Edit .env — set BUILDHIVE_ADMIN_TOKEN to a strong secret

AGENT_PUBLIC_IP=<your-builder-ip> docker compose up -d
```

### 2. Run the agent on your builder node

```bash
BUILDHIVE_SERVER_URL=http://<server-ip>:8080 \
BUILDKIT_PUBLIC_ADDR=<builder-ip>:1234 \
CACHE_ROOT=/var/lib/buildkit \
  ./buildhive-agent
```

### 3. Install the CLI and log in

```bash
# Install
go install github.com/goodwiins/buildhive/cmd/cli@latest

# Log in
buildhive login --server http://<server-ip>:8080 --token <your-admin-token>
```

### 4. Build remotely

```bash
buildhive build -t myregistry/myapp:latest .
```

## GitHub Actions

Drop-in replacement for `docker/build-push-action`:

```yaml
- uses: goodwiins/buildhive/action@main
  with:
    server-url: ${{ secrets.BUILDHIVE_URL }}
    token: ${{ secrets.BUILDHIVE_TOKEN }}
    tags: myregistry/myapp:latest
    push: true
    platforms: linux/amd64,linux/arm64
```

## Configuration

### Server (`buildhive-server`)

| Env var | Required | Default | Description |
|---|---|---|---|
| `DATABASE_URL` | Yes | — | PostgreSQL DSN |
| `BUILDHIVE_ADMIN_TOKEN` | Yes | — | Admin token (strong random string) |
| `PORT` | No | `8080` | HTTP port |
| `GRPC_PORT` | No | `8765` | BuildKit gRPC proxy port |

### Agent (`buildhive-agent`)

| Env var | Required | Default | Description |
|---|---|---|---|
| `BUILDHIVE_SERVER_URL` | Yes | — | Server URL |
| `BUILDKIT_PUBLIC_ADDR` | Yes | — | Public `host:port` of buildkitd on this node |
| `CACHE_ROOT` | No | `/var/lib/buildkit` | BuildKit cache directory |
| `AGENT_NAME` | No | hostname | Builder display name |

## Development

**Requirements:** Go 1.22+, Node 20+, Docker, `sqlc`, `golang-migrate`

```bash
# Start postgres
docker compose up -d postgres

# Run migrations
make migrate-up

# Run all tests
make test

# Build all binaries
make build

# Build web UI
make web
```

## Stack

- **Go** — server, agent, CLI
- **Chi** — HTTP router
- **sqlc** — type-safe SQL queries
- **pgx/v5** — PostgreSQL driver
- **golang-migrate** — schema migrations
- **gRPC** — transparent BuildKit proxy
- **React + Vite + shadcn/ui** — web dashboard
- **testcontainers-go** — integration tests

## Status

Active development. Core server, REST API, and gRPC proxy are complete. Agent, CLI, and web UI are in progress.

See [implementation plan](docs/plans/2026-04-15-buildhive-implementation.md) for full task breakdown.

## License

MIT
