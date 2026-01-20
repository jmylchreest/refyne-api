# Build stage
FROM golang:latest AS builder

# Build args for version info (passed from CI)
ARG VERSION=0.0.0-dev
ARG COMMIT=unknown
ARG DATE=unknown
ARG DIRTY=false

WORKDIR /app

# Install build dependencies for CGO (required for libsql)
RUN apt-get update && apt-get install -y --no-install-recommends \
    gcc \
    libc6-dev \
    git \
    && rm -rf /var/lib/apt/lists/*

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies (using build secret for private repos)
RUN --mount=type=secret,id=GH_TOKEN \
    if [ -f /run/secrets/GH_TOKEN ]; then \
        git config --global url."https://$(cat /run/secrets/GH_TOKEN)@github.com/".insteadOf "https://github.com/"; \
    fi && \
    GOPRIVATE=github.com/jmylchreest/* go mod download

# Copy source code
COPY . .

# Build the binary with CGO enabled and version info injected
RUN echo "Building version: ${VERSION} (commit: ${COMMIT}, dirty: ${DIRTY})" && \
    CGO_ENABLED=1 GOOS=linux go build \
        -ldflags "-s -w \
            -X github.com/jmylchreest/refyne-api/internal/version.Version=${VERSION} \
            -X github.com/jmylchreest/refyne-api/internal/version.Commit=${COMMIT} \
            -X github.com/jmylchreest/refyne-api/internal/version.Date=${DATE} \
            -X github.com/jmylchreest/refyne-api/internal/version.Dirty=${DIRTY}" \
        -o /app/refyne-api ./cmd/refyne-api

# Create data directory with correct permissions for nonroot user (65532)
RUN mkdir -p /data && chown 65532:65532 /data

# Runtime stage - Chainguard distroless with glibc
FROM cgr.dev/chainguard/glibc-dynamic:latest

WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/refyne-api /app/refyne-api

# Copy data directory with correct ownership
COPY --from=builder --chown=65532:65532 /data /data

# Expose port
EXPOSE 8080

# Set environment defaults
ENV PORT=8080
ENV DATABASE_URL=file:/data/refyne.db

# Health check (exec form works in distroless)
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD ["/app/refyne-api", "-health"]

# Run the binary (runs as nonroot user 65532 by default)
ENTRYPOINT ["/app/refyne-api"]
