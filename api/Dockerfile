# Multi-stage Dockerfile for AeternisLog API

# Stage 1: Build
# Go 1.25 is required by the Fabric Gateway client (github.com/hyperledger/fabric-gateway v1.11).
FROM golang:1.25-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git make ca-certificates tzdata

# Set working directory
WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Install the Swagger generator (pinned; cached independently of the source)
RUN go install github.com/swaggo/swag/cmd/swag@v1.16.4

# Copy source code
COPY . .

# Regenerate the Swagger docs so the image always serves the current API surface.
# docs/ is a gitignored build artifact, so generating it here also lets a clean
# checkout build. --parseDependency is omitted on purpose: it makes swag parse
# external modules (e.g. mongo-driver, whose untyped const overflows uint64) and
# aborts generation. Every annotated response type is internal.
RUN swag init -g cmd/api/main.go -o ./docs --parseInternal

# Build the application
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s -X main.Version=$(git describe --tags --always --dirty) -X main.BuildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    -a -installsuffix cgo \
    -o /build/aeternislog-api \
    ./cmd/api

# Build the schema migrator (run as a Job / before the API; never at API boot)
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s" \
    -a -installsuffix cgo \
    -o /build/aeternislog-migrate \
    ./cmd/migrate

# Stage 2: Runtime
FROM alpine:latest

# Install runtime dependencies
RUN apk --no-cache add ca-certificates tzdata curl docker-cli

# Create non-root user
RUN addgroup -g 1000 appuser && \
    adduser -D -u 1000 -G appuser appuser

# Set working directory
WORKDIR /app

# Copy binaries from builder
COPY --from=builder /build/aeternislog-api /app/aeternislog-api
COPY --from=builder /build/aeternislog-migrate /app/aeternislog-migrate

# Copy configuration file and entrypoint
COPY config.yaml /app/config.yaml
COPY entrypoint.sh /app/entrypoint.sh
RUN chmod +x /app/entrypoint.sh

# Create WAL directory
RUN mkdir -p /var/log/aeternislog-wal && \
    chown -R appuser:appuser /var/log/aeternislog-wal

# Change ownership
RUN chown -R appuser:appuser /app

# Switch to non-root user
USER appuser

# Expose ports
EXPOSE 5001 9090

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:5001/health || exit 1

# Run the application (entrypoint optionally applies migrations first; see
# RUN_MIGRATIONS in entrypoint.sh)
ENTRYPOINT ["/app/entrypoint.sh"]
