# Build stage
FROM golang:1.23-alpine AS builder

RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /captcha-server ./cmd/captcha-server

# Runtime stage
FROM alpine:latest

# Install Chromium and dependencies
# Using Alpine's chromium package for reliability
RUN apk add --no-cache \
    # Chromium browser
    chromium \
    # Required for Chrome to run
    nss \
    freetype \
    freetype-dev \
    harfbuzz \
    ca-certificates \
    # Fonts for proper text rendering
    ttf-freefont \
    font-noto \
    font-noto-cjk \
    font-noto-emoji \
    # Process management
    dumb-init \
    && rm -rf /var/cache/apk/*

# Create non-root user
RUN addgroup -g 1000 captcha && \
    adduser -u 1000 -G captcha -s /bin/sh -D captcha

# Set Chrome path for go-rod to use system Chromium
ENV CHROME_PATH=/usr/bin/chromium-browser
# Set HOME to /tmp for Chrome profile (no persistent storage needed)
ENV HOME=/tmp

# Copy binary from builder
COPY --from=builder /captcha-server /usr/local/bin/captcha-server
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Create data directories
RUN mkdir -p /data /tmp/chrome && \
    chown -R captcha:captcha /data /tmp/chrome

# Switch to non-root user
USER captcha

WORKDIR /data

# Expose port
EXPOSE 8191

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=10s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8191/health || exit 1

# Use dumb-init to handle signals properly
ENTRYPOINT ["/usr/bin/dumb-init", "--"]
CMD ["/usr/local/bin/captcha-server"]
