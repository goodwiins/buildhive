.PHONY: build build-server build-agent build-cli test lint migrate-up web test-integration

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
	$(GO) test -race ./... -v

lint:
	golangci-lint run ./...

migrate-up:
	migrate -path schema -database "$$DATABASE_URL" up

web:
	cd web && npm install && npm run build

test-integration:
	$(GO) test -tags integration ./tests/integration/... -v
