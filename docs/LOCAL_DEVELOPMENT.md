# Local Development Guide

This guide covers setting up and running refyne-api locally for development.

## Prerequisites

- **Go 1.21+** with CGO enabled (required for libsql)
- **Node.js 18+** (for web frontend)
- **Task** - Task runner (`go install github.com/go-task/task/v3/cmd/task@latest`)
- **GCC/Clang** - C compiler for CGO

### macOS
```bash
xcode-select --install  # Installs clang
brew install go node task
```

### Linux (Debian/Ubuntu)
```bash
sudo apt install build-essential golang nodejs npm
go install github.com/go-task/task/v3/cmd/task@latest
```

## Quick Start

```bash
# 1. Clone and enter the project
cd refyne-api

# 2. Copy environment template
cp api/.env.example api/.env

# 3. Configure required environment variables (see below)
# Edit api/.env with your Clerk issuer URL

# 4. Install dependencies
task deps

# 5. Start the API (terminal 1)
task api:dev

# 6. Start the web frontend (terminal 2)
task web:dev
```

## Required Configuration

### Clerk Authentication

Clerk is required for user authentication.

1. Create a free account at [clerk.com](https://clerk.com)
2. Create a new application
3. Go to **API Keys** → **Advanced** to find your issuer URL
4. Add to `api/.env`:

```env
CLERK_ISSUER_URL=https://your-app.clerk.accounts.dev
```

**Testing Authentication:**

In development instances, Clerk provides a test mode:
- Use email with `+clerk_test` suffix: `you+clerk_test@example.com`
- Verification code is always: `424242`

### Database (libsql/Turso)

The API uses libsql (Turso's SQLite fork). For local development, you have options:

**Option 1: Local File (Simplest)**
```env
DATABASE_URL=file:refyne.db
```
This creates a local SQLite file. No additional setup needed.

**Option 2: Local libsql Server**
```bash
# Install Turso CLI
curl -sSfL https://get.tur.so/install.sh | bash

# Start local server with persistent file
turso dev --db-file local.db
```

Then configure:
```env
DATABASE_URL=http://127.0.0.1:8080
```

**Option 3: Turso Cloud (with local replica)**

Create a free database at [turso.tech](https://turso.tech):
```bash
turso db create refyne-dev
turso db tokens create refyne-dev
```

Configure:
```env
DATABASE_URL=file:local-replica.db
TURSO_URL=libsql://refyne-dev-yourorg.turso.io
TURSO_AUTH_TOKEN=your-auth-token
```

This syncs your local database with Turso cloud.

### LLM Provider

At least one LLM provider is needed for extractions:

**OpenRouter (Recommended - has free tier)**
```env
SERVICE_OPENROUTER_KEY=sk-or-v1-xxxxx
```

**Or use your own keys:**
```env
SERVICE_ANTHROPIC_KEY=sk-ant-xxxxx
SERVICE_OPENAI_KEY=sk-xxxxx
```

## Environment Variables Reference

Create `api/.env` with these variables:

```env
# Server
PORT=8080
BASE_URL=http://localhost:8080

# Database
DATABASE_URL=file:refyne.db

# Turso Cloud (optional)
# TURSO_URL=libsql://your-db.turso.io
# TURSO_AUTH_TOKEN=your-token

# Clerk (required)
CLERK_ISSUER_URL=https://your-app.clerk.accounts.dev

# LLM Providers (at least one required for extractions)
SERVICE_OPENROUTER_KEY=
SERVICE_ANTHROPIC_KEY=
SERVICE_OPENAI_KEY=

# Frontend
CORS_ORIGINS=http://localhost:3000

# Optional
ADMIN_ENABLED=true
ENCRYPTION_KEY=  # Auto-generated if not set
```

## Task Commands

| Command | Description |
|---------|-------------|
| `task deps` | Install all dependencies (Go + npm) |
| `task api:dev` | Run API in development mode |
| `task api:build` | Build API binary to `bin/` |
| `task api:test` | Run API tests |
| `task api:lint` | Lint API code |
| `task web:dev` | Run frontend dev server |
| `task web:build` | Build frontend for production |
| `task build` | Build everything |
| `task clean` | Remove build artifacts |

## Testing the API

### 1. Health Check

```bash
curl http://localhost:8080/api/v1/health
```

Expected response:
```json
{"status":"healthy","version":"1.0.0"}
```

### 2. Get a Clerk JWT Token

**Option A: From the Web Frontend**

1. Start the web frontend: `task web:dev`
2. Sign in at `http://localhost:3000`
3. Open browser DevTools → Application → Cookies
4. Copy the `__session` cookie value

**Option B: Using Clerk's Backend API**

```bash
# Get a session token for testing (requires Clerk secret key)
curl -X POST https://api.clerk.com/v1/sessions \
  -H "Authorization: Bearer sk_test_xxxxx" \
  -H "Content-Type: application/json" \
  -d '{"user_id": "user_xxxxx"}'
```

### 3. Test Extraction

```bash
curl -X POST http://localhost:8080/api/v1/extract \
  -H "Authorization: Bearer YOUR_CLERK_JWT" \
  -H "Content-Type: application/json" \
  -d '{
    "url": "https://example.com",
    "schema": {
      "type": "object",
      "properties": {
        "title": {"type": "string"},
        "description": {"type": "string"}
      }
    }
  }'
```

### 4. Create an API Key

Once authenticated, you can create API keys for programmatic access:

```bash
curl -X POST http://localhost:8080/api/v1/keys \
  -H "Authorization: Bearer YOUR_CLERK_JWT" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "dev-key"
  }'
```

Response includes the API key (starts with `rf_`). Use it for subsequent requests:

```bash
curl http://localhost:8080/api/v1/usage \
  -H "Authorization: rf_your_api_key"
```

## Troubleshooting

### "CLERK_ISSUER_URL is required"
Add your Clerk issuer URL to `api/.env`. See [Clerk Authentication](#clerk-authentication).

### CGO errors / "sqlite driver not found"
Ensure CGO is enabled:
```bash
export CGO_ENABLED=1
go build ./cmd/refyne-api
```

On Linux, install build tools: `sudo apt install build-essential`

### "missing authorization header"
All endpoints except `/api/v1/health` require authentication. Get a JWT from Clerk.

### Database locked errors
The libsql driver handles this automatically, but if using a file database, ensure only one process accesses it.

### LLM extraction returns empty
Check that at least one `SERVICE_*_KEY` is configured in your `.env` file.

## Project Structure

```
refyne-api/
├── api/                    # Go API backend
│   ├── cmd/refyne-api/     # Entry point
│   ├── internal/           # Internal packages
│   │   ├── auth/           # Clerk JWT verification
│   │   ├── config/         # Configuration
│   │   ├── database/       # Database connection
│   │   ├── http/           # HTTP handlers & middleware
│   │   ├── llm/            # LLM error handling
│   │   ├── repository/     # Data access layer
│   │   ├── service/        # Business logic
│   │   └── worker/         # Background job processor
│   ├── docs/               # Documentation
│   ├── .env                # Local environment (git-ignored)
│   ├── .env.example        # Environment template
│   └── go.mod
├── web/                    # Next.js frontend
│   ├── src/
│   └── package.json
├── Taskfile.yml            # Task runner config
└── docs/                   # Project-wide docs
```

## API Endpoints

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| GET | `/api/v1/health` | No | Health check |
| POST | `/api/v1/extract` | Yes | Single-page extraction |
| POST | `/api/v1/crawl` | Yes | Multi-page crawl job |
| GET | `/api/v1/jobs` | Yes | List jobs |
| GET | `/api/v1/jobs/{id}` | Yes | Get job status |
| GET | `/api/v1/jobs/{id}/stream` | Yes | SSE stream for job results |
| GET | `/api/v1/usage` | Yes | Get usage statistics |
| GET | `/api/v1/llm-config` | Yes | Get LLM configuration |
| GET | `/api/v1/keys` | Yes | List API keys |
| POST | `/api/v1/keys` | Yes | Create API key |

## Next Steps

- Set up Clerk webhooks for user sync (optional)
- Configure Stripe for billing (production)
- See `docs/ANTIBOT_PLAN.md` for anti-bot feature roadmap
