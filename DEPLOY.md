# Deployment Guide

This guide covers deploying refyne-api (Go backend) and refyne-web (Next.js frontend) to Fly.io and Cloudflare Pages.

## Architecture

```
                    ┌─────────────────────────────────────────────────┐
                    │              GitHub Actions                      │
                    │  ┌─────────────────┬─────────────────────────┐  │
                    │  │   main branch   │      v* tags            │  │
                    │  │   (staging)     │     (production)        │  │
                    │  └────────┬────────┴──────────┬──────────────┘  │
                    └───────────┼───────────────────┼──────────────────┘
                                │                   │
            ┌───────────────────┼───────────────────┼───────────────────┐
            │                   ▼                   ▼                   │
            │  ┌─────────────────────┐  ┌─────────────────────┐        │
            │  │ refyne-api-staging  │  │    refyne-api       │  Fly.io│
            │  └─────────────────────┘  └─────────────────────┘        │
            └──────────────────────────────────────────────────────────┘
            ┌──────────────────────────────────────────────────────────┐
            │  ┌─────────────────────┐  ┌─────────────────────┐        │
            │  │ refyne-web-staging  │  │    refyne-web       │   CF   │
            │  └─────────────────────┘  └─────────────────────┘  Pages │
            └──────────────────────────────────────────────────────────┘
```

## Prerequisites

1. **Fly.io account**: https://fly.io
2. **Cloudflare account**: https://cloudflare.com
3. **Clerk account**: https://clerk.com (authentication)

Install CLIs:
```bash
# Fly.io
curl -L https://fly.io/install.sh | sh
fly auth login

# Cloudflare Wrangler
npm install -g wrangler
wrangler login
```

## Initial Setup

### 1. Create Fly.io Apps

```bash
cd api

# Create staging app
fly apps create refyne-api-staging
fly storage create --app refyne-api-staging  # Creates Tigris bucket
fly volumes create refyne_data --size 1 --region sjc --app refyne-api-staging

# Create production app
fly apps create refyne-api
fly storage create --app refyne-api
fly volumes create refyne_data --size 3 --region sjc --app refyne-api
```

### 2. Create Cloudflare Pages Projects

```bash
cd web

# Create via Cloudflare Dashboard, or:
wrangler pages project create refyne-web-staging
wrangler pages project create refyne-web
```

### 3. Set Fly.io Secrets

**Staging:**
```bash
fly secrets set --app refyne-api-staging \
  CLERK_ISSUER_URL="https://your-staging-clerk.clerk.accounts.dev" \
  CLERK_SECRET_KEY="sk_test_xxx" \
  JWT_SECRET="staging-jwt-secret-min-32-chars" \
  ENCRYPTION_KEY="staging-encryption-key-32-bytes"

# LLM providers (at least one required)
fly secrets set --app refyne-api-staging \
  SERVICE_OPENROUTER_KEY="sk-or-xxx"

# Optional: Stripe
fly secrets set --app refyne-api-staging \
  STRIPE_SECRET_KEY="sk_test_xxx" \
  STRIPE_WEBHOOK_SECRET="whsec_xxx"

# Optional: Turso (if not using local SQLite)
fly secrets set --app refyne-api-staging \
  TURSO_URL="libsql://staging-db.turso.io" \
  TURSO_AUTH_TOKEN="xxx"
```

**Production:**
```bash
fly secrets set --app refyne-api \
  CLERK_ISSUER_URL="https://your-clerk.clerk.accounts.dev" \
  CLERK_SECRET_KEY="sk_live_xxx" \
  JWT_SECRET="production-jwt-secret-min-32-chars" \
  ENCRYPTION_KEY="production-encryption-key-32-bytes"

fly secrets set --app refyne-api \
  SERVICE_OPENROUTER_KEY="sk-or-xxx" \
  SERVICE_ANTHROPIC_KEY="sk-ant-xxx" \
  SERVICE_OPENAI_KEY="sk-xxx"

fly secrets set --app refyne-api \
  STRIPE_SECRET_KEY="sk_live_xxx" \
  STRIPE_WEBHOOK_SECRET="whsec_xxx"
```

### 4. Configure GitHub Secrets

Go to GitHub repo → Settings → Secrets and variables → Actions

**Repository Secrets:**

| Secret | Description |
|--------|-------------|
| `FLY_API_TOKEN` | `fly tokens create deploy -x 999999h` |
| `CLOUDFLARE_API_TOKEN` | Cloudflare API token with Pages edit permission |
| `CLOUDFLARE_ACCOUNT_ID` | Your Cloudflare account ID |

**Repository Variables:**

| Variable | Staging | Production |
|----------|---------|------------|
| `CLERK_PUBLISHABLE_KEY` | `pk_test_xxx` | `pk_live_xxx` |

Note: Use GitHub Environments for environment-specific variables:
- Create "staging" and "production" environments
- Add `CLERK_PUBLISHABLE_KEY` to each environment

### 5. Enable SQLite Volume (if needed)

If using local SQLite instead of Turso, uncomment the mounts in the fly.toml files:

```toml
[[mounts]]
  source = "refyne_data"
  destination = "/data"
```

## Deployment

### Automatic (GitHub Actions)

| Trigger | Environment | API App | Web Project |
|---------|-------------|---------|-------------|
| Push to `main` | staging | refyne-api-staging | refyne-web-staging |
| Push `v*` tag | production | refyne-api | refyne-web |
| Manual dispatch | choice | based on selection | based on selection |

**Create a release:**
```bash
git tag v1.0.0
git push origin v1.0.0
```

### Manual Deployment

**API:**
```bash
cd api

# Staging
fly deploy --config fly.staging.toml --app refyne-api-staging

# Production
fly deploy --config fly.production.toml --app refyne-api
```

**Web:**
```bash
cd web
npm ci
npm run build

# Staging
NEXT_PUBLIC_API_URL=https://refyne-api-staging.fly.dev \
NEXT_PUBLIC_CLERK_PUBLISHABLE_KEY=pk_test_xxx \
npm run build

wrangler pages deploy .next --project-name=refyne-web-staging

# Production
NEXT_PUBLIC_API_URL=https://refyne-api.fly.dev \
NEXT_PUBLIC_CLERK_PUBLISHABLE_KEY=pk_live_xxx \
npm run build

wrangler pages deploy .next --project-name=refyne-web
```

## Environment Reference

### API Environment Variables

| Variable | Required | Description |
|----------|----------|-------------|
| `PORT` | Yes | HTTP port (default: 8080) |
| `LOG_LEVEL` | No | debug, info, warn, error (default: info) |
| `CLERK_ISSUER_URL` | Yes | Clerk issuer URL |
| `CLERK_SECRET_KEY` | Yes | Clerk secret key |
| `JWT_SECRET` | Yes | JWT signing secret (min 32 chars) |
| `ENCRYPTION_KEY` | Yes | Encryption key for sensitive data (32 bytes) |
| `DATABASE_URL` | No | SQLite path (default: /data/refyne.db) |
| `TURSO_URL` | No | Turso database URL |
| `TURSO_AUTH_TOKEN` | No | Turso auth token |
| `SERVICE_OPENROUTER_KEY` | Cond. | OpenRouter API key |
| `SERVICE_ANTHROPIC_KEY` | Cond. | Anthropic API key |
| `SERVICE_OPENAI_KEY` | Cond. | OpenAI API key |
| `STRIPE_SECRET_KEY` | No | Stripe secret key |
| `STRIPE_WEBHOOK_SECRET` | No | Stripe webhook secret |

### Web Environment Variables

| Variable | Required | Description |
|----------|----------|-------------|
| `NEXT_PUBLIC_API_URL` | Yes | API base URL |
| `NEXT_PUBLIC_CLERK_PUBLISHABLE_KEY` | Yes | Clerk publishable key |
| `CLERK_SECRET_KEY` | Yes | Clerk secret key (server-side) |

## Monitoring

**Fly.io:**
```bash
# Logs
fly logs --app refyne-api-staging
fly logs --app refyne-api

# Status
fly status --app refyne-api

# SSH into machine
fly ssh console --app refyne-api
```

**Cloudflare Pages:**
- View deployments in Cloudflare Dashboard → Pages

## Troubleshooting

### Health check failures
```bash
# Check if app is running
fly status --app refyne-api

# Check logs
fly logs --app refyne-api --region sjc

# Verify health endpoint
curl https://refyne-api.fly.dev/health
```

### Database issues
```bash
# SSH and check SQLite
fly ssh console --app refyne-api
ls -la /data/
sqlite3 /data/refyne.db ".tables"
```

### Volume not mounted
```bash
# Verify volume exists
fly volumes list --app refyne-api

# Check mount in fly.toml
# Ensure [[mounts]] is uncommented
```
