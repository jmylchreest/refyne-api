# Deployment Guide

This guide covers deploying refyne-api to production using Fly.io (backend), Cloudflare Pages (frontend), and Turso (database).

## Architecture

```
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│  Cloudflare     │     │    Fly.io       │     │     Turso       │
│  Pages          │────▶│    (Go API)     │────▶│   (Database)    │
│  (Next.js)      │     │                 │     │                 │
└─────────────────┘     └─────────────────┘     └─────────────────┘
```

## Prerequisites

- [Fly.io CLI](https://fly.io/docs/hands-on/install-flyctl/)
- [Turso CLI](https://docs.turso.tech/cli/installation)
- [Wrangler CLI](https://developers.cloudflare.com/workers/wrangler/install-and-update/)
- GitHub repository with Actions enabled

## 1. Database Setup (Turso)

```bash
# Login to Turso
turso auth login

# Create production database
turso db create refyne-prod --region sjc  # Use region close to your Fly.io app

# Get connection details
turso db show refyne-prod --url           # Save as TURSO_URL
turso db tokens create refyne-prod        # Save as TURSO_AUTH_TOKEN
```

## 2. Backend Setup (Fly.io)

```bash
cd api

# Login to Fly.io
fly auth login

# Create app (first time only)
fly launch --no-deploy
# Follow prompts:
# - App name: refyne-api (or your preferred name)
# - Region: sjc (or region close to Turso)
# - No PostgreSQL
# - No Redis

# Set secrets
fly secrets set \
  TURSO_URL="libsql://refyne-prod-xxx.turso.io" \
  TURSO_AUTH_TOKEN="eyJhbGciOiJFZERTQSIsInR5cCI6IkpXVCJ9..." \
  CLERK_ISSUER_URL="https://your-clerk-instance.clerk.accounts.dev" \
  CLERK_SECRET_KEY="sk_live_xxx" \
  ENCRYPTION_KEY="your-32-byte-encryption-key"

# Deploy
fly deploy
```

### Fly.io Configuration

The `fly.toml` file configures:
- **Auto-scaling**: Scales to 0 when idle, starts on request
- **Health checks**: `/health` endpoint monitored
- **Resources**: 512MB RAM, shared CPU (adjustable)

To use persistent SQLite (instead of Turso):
```bash
# Create volume
fly volumes create refyne_data --size 1 --region sjc

# Uncomment mounts section in fly.toml
```

## 3. Frontend Setup (Cloudflare Pages)

### Option A: GitHub Integration (Recommended)

1. Go to [Cloudflare Dashboard](https://dash.cloudflare.com/) → Pages
2. Create project → Connect to Git
3. Select your repository
4. Configure build:
   - Build command: `npm run build`
   - Build output: `.next`
   - Root directory: `web`
5. Add environment variables:
   - `NEXT_PUBLIC_API_URL`: `https://refyne-api.fly.dev`
   - `NEXT_PUBLIC_CLERK_PUBLISHABLE_KEY`: Your Clerk key

### Option B: Wrangler CLI

```bash
cd web

# Login to Cloudflare
wrangler login

# Build
npm run build

# Deploy
wrangler pages deploy .next --project-name=refyne-web
```

## 4. GitHub Actions Setup

### Required Secrets

Add these in GitHub → Settings → Secrets and variables → Actions:

| Secret | Description | How to get |
|--------|-------------|------------|
| `FLY_API_TOKEN` | Fly.io deploy token | `fly tokens create deploy` |
| `CLOUDFLARE_API_TOKEN` | Cloudflare API token | Dashboard → API Tokens → Create |
| `CLOUDFLARE_ACCOUNT_ID` | Cloudflare account ID | Dashboard → Overview (right sidebar) |

### Required Variables

Add these in GitHub → Settings → Secrets and variables → Actions → Variables:

| Variable | Description | Example |
|----------|-------------|---------|
| `API_URL` | Backend URL | `https://refyne-api.fly.dev` |
| `CLERK_PUBLISHABLE_KEY` | Clerk public key | `pk_live_xxx` |

## 5. DNS Configuration (Optional)

If using custom domain with Cloudflare:

```bash
# Add custom domain to Fly.io
fly certs create api.yourdomain.com

# In Cloudflare DNS, add:
# Type: CNAME
# Name: api
# Target: refyne-api.fly.dev
# Proxy: Off (DNS only) - required for Fly.io TLS
```

For the frontend, Cloudflare Pages handles this automatically.

## Workflows

### CI (`ci.yml`)
- Runs on: Pull requests and pushes to `main`
- Jobs:
  - `api-test`: Go tests and build
  - `web-test`: Next.js lint and build

### Deploy (`deploy.yml`)
- Runs on: Push to `main`
- Jobs:
  - `deploy-api`: Deploys to Fly.io
  - `deploy-web`: Deploys to Cloudflare Pages

## Monitoring

### Fly.io
```bash
fly logs              # View logs
fly status            # App status
fly ssh console       # SSH into machine
```

### Turso
```bash
turso db shell refyne-prod    # SQL shell
turso db inspect refyne-prod  # Database stats
```

### Cloudflare
- Pages dashboard shows deployment logs and analytics

## Troubleshooting

### "database is locked" errors
- Ensure WAL mode is enabled (automatic in current code)
- Check connection pool settings

### Build fails with CGO errors
- The Dockerfile uses `golang:latest` with Debian for CGO support
- libsql requires CGO enabled

### Fly.io machine not starting
```bash
fly logs --app refyne-api
fly machine list --app refyne-api
```

## Cost Estimate

| Service | Free Tier | Typical Small App |
|---------|-----------|-------------------|
| Fly.io | Invoices <$5 waived | ~$3-5/month |
| Turso | 5GB, 500M reads/month | Free |
| Cloudflare Pages | Unlimited sites | Free |
| **Total** | | **~$0-5/month** |
