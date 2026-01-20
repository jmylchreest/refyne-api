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

# Copy source code
COPY . .

# Build the binary with CGO enabled (required for libsql)
RUN CGO_ENABLED=1 GOOS=linux go build -o /app/refyne-api ./cmd/refyne-api

# Runtime stage - use Ubuntu 24.04 for glibc 2.39 compatibility
FROM ubuntu:24.04

WORKDIR /app

# Install runtime dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    tzdata \
    && rm -rf /var/lib/apt/lists/*

# Copy binary from builder
COPY --from=builder /app/refyne-api /app/refyne-api

# Create non-root user
RUN useradd -r -u 1000 appuser && \
    mkdir -p /data && \
    chown appuser:appuser /data

USER appuser

# Expose port
EXPOSE 8080

# Set environment defaults
ENV PORT=8080
ENV DATABASE_URL=file:/data/refyne.db

# Health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD ["/app/refyne-api", "-health"] || exit 1

# Run the binary
CMD ["/app/refyne-api"]
