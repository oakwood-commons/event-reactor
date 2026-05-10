# syntax=docker/dockerfile:1

# ── Build stage ──────────────────────────────────────────────────────
FROM golang:1.26-alpine AS builder

WORKDIR /src

# Cache module downloads
COPY go.mod go.sum ./
RUN go mod download

# Build
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-s -w \
      -X github.com/oakwood-commons/event-reactor/pkg/params/version.BuildTime=$(date -Iseconds) \
      -X github.com/oakwood-commons/event-reactor/pkg/params/version.BuildVersion=$(cat VERSION 2>/dev/null || echo dev) \
      -X github.com/oakwood-commons/event-reactor/pkg/params/version.Commit=$(git rev-parse HEAD 2>/dev/null || echo unknown)" \
    -o /er ./cmd/er

# ── Runtime stage ────────────────────────────────────────────────────
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /er /er

EXPOSE 8080 9090

ENTRYPOINT ["/er"]
CMD ["run", "server", "-c", "/config/server.yaml"]
