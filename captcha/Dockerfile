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
FROM alpine:3.21

# Install Chrome/Chromium and dependencies
RUN apk add --no-cache \
    chromium \
    chromium-chromedriver \
    nss \
    freetype \
    freetype-dev \
    harfbuzz \
    ca-certificates \
    ttf-freefont \
    font-noto \
    font-noto-cjk \
    font-noto-emoji \
    dumb-init \
    && rm -rf /var/cache/apk/*

# Create non-root user
RUN addgroup -g 1000 captcha && \
    adduser -u 1000 -G captcha -s /bin/sh -D captcha

# Set Chrome path for go-rod
ENV CHROME_PATH=/usr/bin/chromium-browser

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
