# Captcha Server

A FlareSolverr-compatible CAPTCHA solving service with anti-bot bypass capabilities.

## Features

- FlareSolverr API compatibility
- Cloudflare challenge bypass
- CAPTCHA solving via 2Captcha/CapSolver
- Session management for persistent browser instances
- Browser pool with automatic recycling
- OpenAPI documentation

## Quick Start

```bash
# Run in standalone mode (no auth, FlareSolverr-compatible)
task dev    # with hot reload
task run    # without hot reload

# Run with authentication
REFYNE_API_SECRET=your-secret task run-auth
```

## API Documentation

OpenAPI documentation is available at:
- **Swagger UI**: `http://localhost:8191/docs`
- **OpenAPI JSON**: `http://localhost:8191/openapi.json`
- **OpenAPI YAML**: `http://localhost:8191/openapi.yaml`

## API Endpoints

### Health Check

```
GET /health
```

Returns server health status and browser pool statistics.

### Solve Challenge

```
POST /v1
Content-Type: application/json
```

FlareSolverr-compatible endpoint for all operations.

#### Commands

| Command | Description |
|---------|-------------|
| `request.get` | Navigate to URL and return page content |
| `request.post` | POST to URL and return page content |
| `sessions.create` | Create a persistent browser session |
| `sessions.list` | List all active sessions |
| `sessions.destroy` | Destroy a session |

#### Example: Simple GET Request

```json
{
  "cmd": "request.get",
  "url": "https://example.com",
  "maxTimeout": 60000
}
```

#### Example: Using Sessions

Sessions provide persistent browser instances with preserved cookies/state.

**Important**: Sessions are single-threaded. Only one request can use a session at a time. Concurrent requests to the same session will receive "session is currently in use" errors. For concurrent requests, either:
- Use multiple sessions (one per concurrent request)
- Don't specify a session (uses browser pool with automatic management)

```json
// Create session (optional: specify your own session name)
{
  "cmd": "sessions.create",
  "session": "my-session"
}
// Response: {"status": "ok", "session": "my-session", ...}

// Create session (auto-generated ID)
{
  "cmd": "sessions.create"
}
// Response: {"status": "ok", "session": "01KFDP23B10YRBC9H9H3Q05MKW", ...}

// Use session (sequential requests only!)
{
  "cmd": "request.get",
  "url": "https://example.com",
  "session": "my-session"
}

// Destroy session
{
  "cmd": "sessions.destroy",
  "session": "my-session"
}
```

#### Example: With Cookies

```json
{
  "cmd": "request.get",
  "url": "https://example.com",
  "cookies": [
    {"name": "session", "value": "abc123", "domain": "example.com"}
  ]
}
```

#### Response Format

```json
{
  "status": "ok",
  "message": "Challenge solved successfully",
  "solution": {
    "url": "https://example.com",
    "status": 200,
    "cookies": [...],
    "userAgent": "Mozilla/5.0...",
    "response": "<html>...</html>"
  },
  "challenged": true,
  "solved": true,
  "method": "wait",
  "startTimestamp": 1234567890,
  "endTimestamp": 1234567891,
  "version": "1.0.0"
}
```

#### Challenge Tracking Fields

| Field | Type | Description |
|-------|------|-------------|
| `challenged` | bool | Whether a challenge was detected |
| `solved` | bool | Whether the challenge was solved |
| `method` | string | How the request was resolved |

**Method values:**

| Method | Description |
|--------|-------------|
| `cached` | No challenge - session cookies were reused |
| `wait` | Challenge auto-resolved (Cloudflare JS challenge) |
| `2captcha` | Solved by 2Captcha service |
| `capsolver` | Solved by CapSolver service |
| (empty) | No challenge and no session cookies |

## Configuration

| Environment Variable | Description | Default |
|---------------------|-------------|---------|
| `PORT` | Server port | `8191` |
| `LOG_LEVEL` | Log level (debug/info/warn/error) | `info` |
| `LOG_FORMAT` | Log format (text/json) | `json` |
| `ALLOW_UNAUTHENTICATED` | Disable auth (standalone mode) | `false` |
| `REFYNE_API_SECRET` | HMAC secret for signed headers | - |
| `CLERK_ISSUER` | Clerk issuer URL for JWT validation | - |
| `REQUIRED_FEATURE` | Required feature for access | `captcha` |
| `BROWSER_POOL_SIZE` | Max concurrent browsers | `5` |
| `BROWSER_IDLE_TIMEOUT` | Browser idle timeout | `5m` |
| `BROWSER_MAX_REQUESTS` | Requests before browser recycle | `100` |
| `BROWSER_MAX_AGE` | Max browser age before recycle | `30m` |
| `CHROME_PATH` | Custom Chrome/Chromium path | auto-download |
| `CHALLENGE_TIMEOUT` | Max time to solve challenge | `60s` |
| `TWOCAPTCHA_API_KEY` | 2Captcha API key | - |
| `CAPSOLVER_API_KEY` | CapSolver API key | - |
| `PROXY_ENABLED` | Enable default proxy | `false` |
| `PROXY_URL` | Default proxy URL | - |
| `SESSION_MAX_IDLE` | Session idle timeout | `10m` |
| `DISABLE_STEALTH` | Disable stealth mode (for testing CAPTCHA solving) | `false` |

## Authentication

### Standalone Mode

Set `ALLOW_UNAUTHENTICATED=true` for local development or FlareSolverr drop-in replacement.

### With Refyne API

The service accepts signed headers from refyne-api:
- `X-Refyne-Signature`: HMAC-SHA256 signature
- `X-Refyne-Timestamp`: Unix timestamp
- `X-Refyne-User-ID`: User ID
- `X-Refyne-Tier`: User tier
- `X-Refyne-Features`: Comma-separated features

### Direct JWT

Supports Clerk JWT validation when `CLERK_ISSUER` is set.

## Development

```bash
# Install dependencies
task deps

# Run tests
task test

# Run with coverage
task test-cover

# Build
task build

# Lint
task lint
```

## Docker

```bash
# Build image
task docker:build

# Run container (standalone mode)
task docker:run
```
