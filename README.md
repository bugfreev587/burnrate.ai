# burnrate-ai

A multi-tenant usage tracking and management gateway for Claude Code and other LLM agents. Teams use it to report, visualize, and control their LLM API spending.

## What it does

- **Usage tracking** – Agents call a simple HTTP endpoint to report token consumption and cost after each LLM request.
- **Dashboard** – Owners and team members see total requests, tokens, and costs with a per-request history table.
- **Team management** – Invite members by email, assign roles, suspend or remove users.
- **API key management** – Admins create and revoke agent API keys; secrets are stored hashed and shown only once.
- **Multi-tenant isolation** – Every organization gets its own workspace; data is fully separated.
- **Provider key vault** – Centralized Anthropic/OpenAI key storage *(coming soon)*.

## Architecture

```
burnrate-ai/
├── api-server/   # Go 1.24 backend (Gin + PostgreSQL + Redis)
└── dashboard/    # React 19 + TypeScript frontend (Vite + Clerk)
```

The frontend is hosted on **Vercel**. The backend runs in a **Docker container on Railway** backed by a managed PostgreSQL and Redis instance.

---

## API Server

**Stack:** Go 1.24 · Gin · GORM · PostgreSQL · Redis

### Directory layout

```
api-server/
├── cmd/server/main.go                # Entry point
├── conf/api-server-prod.yaml         # Production config
├── Dockerfile                        # Multi-stage build (Alpine)
└── internal/
    ├── api/
    │   ├── server.go                 # Route registration
    │   ├── auth.go                   # POST /v1/auth/sync
    │   ├── usage.go                  # Usage report & list
    │   ├── apikeys.go                # API key CRUD
    │   ├── users.go                  # User management
    │   ├── provider_keys.go          # Provider key CRUD (TODO)
    │   └── middleware.go             # CORS · logger · rate-limit
    ├── middleware/
    │   ├── auth.go                   # API key validation
    │   └── rbac.go                   # RBAC (user lookup + role gates)
    ├── models/models.go              # Tenant · User · APIKey · ProviderKey · UsageLog
    ├── services/
    │   ├── apikey_service.go         # HMAC-SHA256 + Redis cache
    │   └── usage_service.go          # Usage log CRUD
    ├── db/postgres.go                # GORM init + AutoMigrate
    └── config/config.go              # YAML config + env overrides
```

### Data models

| Model | Key fields |
|---|---|
| `Tenant` | `id`, `name`, `created_at` |
| `User` | `id` (Clerk ID), `tenant_id`, `email`, `name`, `role`, `status` |
| `APIKey` | `key_id`, `tenant_id`, `label`, `hash`, `salt`, `scopes`, `expires_at` |
| `ProviderKey` | `id`, `tenant_id`, `provider`, `encrypted_key` |
| `UsageLog` | `id`, `tenant_id`, `provider`, `model`, `prompt_tokens`, `completion_tokens`, `cost`, `request_id` |

### API endpoints

| Method | Path | Auth | Description |
|---|---|---|---|
| GET | `/v1/health` | — | Health check |
| POST | `/v1/auth/sync` | — | Sync Clerk user → DB (creates tenant for new users) |
| POST | `/v1/agent/usage` | API key | Report LLM usage from an agent |
| GET | `/v1/usage` | Viewer+ | List tenant usage logs |
| GET | `/v1/usage/summary` | Viewer+ | Aggregated stats *(not yet implemented)* |
| GET | `/v1/admin/users` | Admin+ | List tenant members |
| POST | `/v1/admin/users/invite` | Admin+ | Invite user by email |
| PATCH | `/v1/admin/users/:id/role` | Admin+ | Set role (viewer / editor) |
| PATCH | `/v1/admin/users/:id/suspend` | Admin+ | Suspend user |
| PATCH | `/v1/admin/users/:id/unsuspend` | Admin+ | Unsuspend user |
| DELETE | `/v1/admin/users/:id` | Admin+ | Remove user |
| POST | `/v1/admin/api_keys` | Admin+ | Create API key |
| GET | `/v1/admin/api_keys` | Admin+ | List API keys |
| DELETE | `/v1/admin/api_keys/:id` | Admin+ | Revoke API key |
| POST | `/v1/admin/provider_keys` | Admin+ | Add provider key *(TODO)* |
| GET | `/v1/admin/provider_keys` | Admin+ | List provider keys *(TODO)* |
| DELETE | `/v1/admin/provider_keys/:id` | Admin+ | Revoke provider key *(TODO)* |
| POST | `/v1/owner/users/:id/promote-admin` | Owner | Promote user to admin |
| DELETE | `/v1/owner/users/:id/demote-admin` | Owner | Demote admin to editor |
| POST | `/v1/owner/transfer-ownership` | Owner | Transfer workspace ownership |

### RBAC roles

| Role | Level | Permissions |
|---|---|---|
| `owner` | 4 | Everything + promote/demote admins, transfer ownership |
| `admin` | 3 | Invite/suspend/remove users, manage API & provider keys |
| `editor` | 2 | View usage, manage API keys |
| `viewer` | 1 | View usage |

### Auth

- **Dashboard → API:** `X-User-ID: <clerk_user_id>` header. The RBAC middleware looks up the user in PostgreSQL, checks their status (`active` / `suspended`) and role.
- **Agent → API:** `Authorization: ApiKey <key_id>:<secret>`. Validated via HMAC-SHA256; results are cached in Redis for 5 minutes.

### Configuration

Production config is loaded from `conf/api-server-prod.yaml`. Sensitive values are overridden by environment variables at runtime:

| Env var | Purpose |
|---|---|
| `POSTGRES_DB_URL` | PostgreSQL connection string |
| `REDIS_URL` | Redis connection string |
| `API_KEY_PEPPER` | Secret pepper for API key hashing |
| `CORS_ORIGINS` | Comma-separated allowed origins (or `*`) |

---

## Dashboard

**Stack:** React 19 · TypeScript · Vite · Clerk · Recharts · React Router

### Directory layout

```
dashboard/
├── src/
│   ├── main.tsx                 # Clerk provider setup
│   ├── App.tsx                  # Routes + auth guards
│   ├── pages/
│   │   ├── HomePage.tsx         # Landing page
│   │   ├── SignInPage.tsx       # Clerk sign-in embed
│   │   ├── SignUpPage.tsx       # Clerk sign-up embed
│   │   ├── Dashboard.tsx        # Usage summary + log table
│   │   ├── ProfilePage.tsx      # Clerk profile embed
│   │   └── ManagementPage.tsx   # Team & API key management
│   ├── components/
│   │   ├── Navbar.tsx           # Top nav + user menu
│   │   └── APIKeyModal.tsx      # One-time secret display
│   └── hooks/
│       ├── useUserSync.ts       # Clerk ↔ backend sync
│       └── useUsageData.ts      # Usage log fetcher
├── vercel.json                  # SPA rewrite (all routes → index.html)
└── .env.example
```

### Pages

- **Dashboard** – Summary cards (requests / tokens / cost) and a paginated usage table.
- **ManagementPage** – Team members table (invite, change role, suspend, remove), Gateway API Keys table (create, revoke, one-time secret display), Provider Keys section (placeholder).

### Key hooks

- **`useUserSync`** – Runs once after Clerk sign-in. Calls `POST /v1/auth/sync`, stores `userId`, `tenantId`, `role`, and `status` in state and `localStorage`. Handles three cases: existing user, pending email invitation, and brand-new user (creates tenant).
- **`useUsageData`** – Calls `GET /v1/usage`, returns logs + refresh function.

### Environment variables

```
VITE_CLERK_PUBLISHABLE_KEY=pk_live_...
VITE_API_SERVER_URL=https://burnrateai-production.up.railway.app
```

---

## Deployment

### Backend (Railway)

The API server is deployed as a Docker container on Railway.

```bash
# Dockerfile builds a minimal Alpine image
docker build -t burnrate-ai-api ./api-server
```

Required Railway environment variables:

```
POSTGRES_DB_URL=postgresql://...
REDIS_URL=redis://...
API_KEY_PEPPER=<random-secret>
CORS_ORIGINS=https://burnrate-ai-weld.vercel.app
```

### Frontend (Vercel)

The dashboard is deployed to Vercel from the `dashboard/` root directory.

```bash
vercel --prod
```

Required Vercel environment variables:

```
VITE_CLERK_PUBLISHABLE_KEY=pk_live_...
VITE_API_SERVER_URL=https://burnrateai-production.up.railway.app
```

---

## User flows

### First signup (becomes owner)

1. User signs up via Clerk.
2. Frontend calls `POST /v1/auth/sync`.
3. Backend creates a new `Tenant` and sets the user as `owner`.
4. User lands on the Dashboard (empty) and can create API keys or invite teammates.

### Invited team member

1. Admin creates a pending invite via the Management page.
2. Invitee signs up with the same email address.
3. `auth/sync` detects the pending record, activates the account with the invited role.

### Agent reporting usage

```bash
curl -X POST https://burnrateai-production.up.railway.app/v1/agent/usage \
  -H "Authorization: ApiKey <key_id>:<secret>" \
  -H "Content-Type: application/json" \
  -d '{
    "provider": "anthropic",
    "model": "claude-sonnet-4-6",
    "prompt_tokens": 1200,
    "completion_tokens": 400,
    "cost": 0.0048,
    "request_id": "req_abc123"
  }'
```

---

## Status

| Component | Status |
|---|---|
| Auth sync (Clerk → DB) | ✅ Live |
| Multi-tenant RBAC | ✅ Live |
| Usage reporting (`/v1/agent/usage`) | ✅ Live |
| Usage dashboard | ✅ Live |
| Team management (invite / suspend / remove) | ✅ Live |
| API key management | ✅ Live |
| Usage summary / aggregation | 🚧 Not implemented |
| Provider key vault (Anthropic / OpenAI) | 🚧 Not implemented |
