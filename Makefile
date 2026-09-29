.PHONY: dev-backend dev-frontend frontend build test fmt vet docker-build clean

GOCACHE_DIR ?= $(CURDIR)/.cache/go-build
VERSION ?= dev

dev-backend:
	GOCACHE=$(GOCACHE_DIR) go run ./cmd/server

dev-frontend:
	npm --prefix frontend run dev

frontend:
	npm --prefix frontend run build
	mkdir -p internal/web/dist
	find internal/web/dist -mindepth 1 -type f ! -name placeholder.txt -delete
	cp -R frontend/dist/. internal/web/dist/

build: frontend
	mkdir -p bin
	GOCACHE=$(GOCACHE_DIR) go build -trimpath -ldflags="-s -w -X main.version=$(VERSION)" -o bin/claude-relay ./cmd/server

test:
	GOCACHE=$(GOCACHE_DIR) go test ./...
	npm --prefix frontend run typecheck

fmt:
	gofmt -w cmd internal

vet:
	GOCACHE=$(GOCACHE_DIR) go vet ./...

docker-build:
	docker build --build-arg VERSION=$(VERSION) -t claude-relay:$(VERSION) .

clean:
	find internal/web/dist -mindepth 1 -type f ! -name placeholder.txt -delete
	find internal/web/dist -mindepth 1 -type d -empty -delete
	rm -f bin/claude-relay
