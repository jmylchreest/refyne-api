# Build stage
FROM golang:latest AS builder

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

# Copy source code (including .git for version detection)
COPY . .

# Build the binary with CGO enabled and version info injected
RUN set -e && \
    PKG="github.com/jmylchreest/refyne-api/internal/version" && \
    DATE=$(date -u +"%Y-%m-%dT%H:%M:%SZ") && \
    COMMIT=$(git rev-parse --short=7 HEAD 2>/dev/null || echo "unknown") && \
    DIRTY="false" && \
    if [ -n "$(git status --porcelain 2>/dev/null)" ]; then DIRTY="true"; fi && \
    # Compute version from git tags (gitversion style)
    if git describe --tags --always --long 2>/dev/null | grep -q '^v'; then \
        DESCRIBE=$(git describe --tags --always --long 2>/dev/null); \
        if echo "$DESCRIBE" | grep -qE '^v[0-9]+\.[0-9]+\.[0-9]+-[0-9]+-g[a-f0-9]+$'; then \
            MAJOR=$(echo "$DESCRIBE" | sed -E 's/^v([0-9]+)\.([0-9]+)\.([0-9]+)-([0-9]+)-g([a-f0-9]+)$/\1/'); \
            MINOR=$(echo "$DESCRIBE" | sed -E 's/^v([0-9]+)\.([0-9]+)\.([0-9]+)-([0-9]+)-g([a-f0-9]+)$/\2/'); \
            PATCH=$(echo "$DESCRIBE" | sed -E 's/^v([0-9]+)\.([0-9]+)\.([0-9]+)-([0-9]+)-g([a-f0-9]+)$/\3/'); \
            COMMITS=$(echo "$DESCRIBE" | sed -E 's/^v([0-9]+)\.([0-9]+)\.([0-9]+)-([0-9]+)-g([a-f0-9]+)$/\4/'); \
            HASH=$(echo "$DESCRIBE" | sed -E 's/^v([0-9]+)\.([0-9]+)\.([0-9]+)-([0-9]+)-g([a-f0-9]+)$/\5/'); \
            if [ "$COMMITS" = "0" ]; then \
                VERSION="${MAJOR}.${MINOR}.${PATCH}"; \
            else \
                NEXT_PATCH=$((PATCH + 1)); \
                VERSION="${MAJOR}.${MINOR}.${NEXT_PATCH}-dev.${COMMITS}+${HASH}"; \
            fi; \
        else \
            VERSION="0.0.1-dev.0+${COMMIT}"; \
        fi; \
    else \
        COMMIT_COUNT=$(git rev-list --count HEAD 2>/dev/null || echo "0"); \
        VERSION="0.0.1-dev.${COMMIT_COUNT}+${COMMIT}"; \
    fi && \
    echo "Building version: ${VERSION} (commit: ${COMMIT}, dirty: ${DIRTY})" && \
    CGO_ENABLED=1 GOOS=linux go build \
        -ldflags "-s -w -X ${PKG}.Version=${VERSION} -X ${PKG}.Commit=${COMMIT} -X ${PKG}.Date=${DATE} -X ${PKG}.Dirty=${DIRTY}" \
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
