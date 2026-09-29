# syntax=docker/dockerfile:1.7

FROM node:22-alpine AS frontend-builder
WORKDIR /app/frontend
ARG NPM_REGISTRY=https://registry.npmjs.org
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci --registry=${NPM_REGISTRY} --ignore-scripts
COPY frontend/ ./
RUN npm run build

FROM golang:1.26-alpine AS backend-builder
WORKDIR /src
ARG GOPROXY=https://proxy.golang.org,direct
ARG VERSION=dev
RUN apk add --no-cache ca-certificates
COPY go.mod go.sum ./
RUN GOPROXY=${GOPROXY} go mod download
COPY . .
COPY --from=frontend-builder /app/frontend/dist/ ./internal/web/dist/
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w -X main.version=${VERSION}" \
    -o /out/claude-relay ./cmd/server

FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata && \
    addgroup -S relay && adduser -S -G relay -H -s /sbin/nologin relay && \
    mkdir -p /data && chown relay:relay /data
COPY --from=backend-builder /out/claude-relay /usr/local/bin/claude-relay

USER relay
WORKDIR /app
ENV LISTEN_ADDR=:8080 \
    DATA_DIR=/data \
    COOKIE_SECURE=false
EXPOSE 8080
VOLUME ["/data"]
HEALTHCHECK --interval=30s --timeout=5s --start-period=8s --retries=3 \
  CMD wget -q -O - http://127.0.0.1:8080/healthz | grep -q '"status":"ok"' || exit 1
ENTRYPOINT ["/usr/local/bin/claude-relay"]
