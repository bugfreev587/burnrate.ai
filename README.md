# burnrate-ai

A multi-tenant usage tracking and management gateway for Claude Code and other LLM agents. Teams use it to report, visualize, and control their LLM API spending.

## What it does

- **Usage tracking** – Agents call a simple HTTP endpoint to report token consumption after each LLM request.
- **Server-side cost computation** – Cost is computed authoritatively from token counts using versioned, per-provider pricing. Client-provided costs are ignored.
- **Cost ledger** – Every priced request is written to an immutable ledger for financial auditing and forecasting.
- **Spend forecasting** – Daily-average extrapolation gives a projected monthly spend based on actual usage so far.
- **Budget enforcement** – Per-tenant spend limits (monthly / weekly / daily) can alert or block requests when exceeded.
- **Markup / monetization** – Admins configure percentage markups per provider, model, or globally to bill tenants above cost.
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

**Stack:** Go 1.24 · Gin · GORM · PostgreSQL · Redis · shopspring/decimal

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
    │   ├── pricing.go                # Pricing admin + cost ledger + forecast
    │   ├── apikeys.go                # API key CRUD
    │   ├── users.go                  # User management
    │   ├── provider_keys.go          # Provider key CRUD
    │   └── middleware.go             # CORS · logger · rate-limit
    ├── middleware/
    │   ├── auth.go                   # API key validation
    │   └── rbac.go                   # RBAC (user lookup + role gates)
    ├── models/
    │   ├── models.go                 # Tenant · User · APIKey · ProviderKey · UsageLog
    │   └── pricing.go                # Provider · ModelDef · ModelPricing · ContractPricing
    │                                 # PricingMarkup · CostLedger · BudgetLimit
    ├── pricing/
    │   ├── types.go                  # UsageEvent · PricingResult · errors
    │   ├── resolver.go               # Contract override → standard pricing (Redis-cached)
    │   ├── calculator.go             # Decimal token math + markup application
    │   ├── seeder.go                 # Seed providers, models, and pricing on startup
    │   └── engine.go                 # Pipeline orchestrator
    ├── services/
    │   ├── apikey_service.go         # HMAC-SHA256 + Redis cache
    │   └── usage_service.go          # Usage log CRUD
    ├── db/postgres.go                # GORM init + AutoMigrate + seed
    └── config/config.go              # YAML config + env overrides
```

### Data models

| Model | Key fields |
|---|---|
| `Tenant` | `id`, `name` |
| `User` | `id` (Clerk ID), `tenant_id`, `email`, `role`, `status` |
| `APIKey` | `key_id`, `tenant_id`, `label`, `hash`, `salt`, `scopes`, `expires_at` |
| `ProviderKey` | `id`, `tenant_id`, `provider`, `encrypted_key` |
| `UsageLog` | `id`, `tenant_id`, `provider`, `model`, `prompt_tokens`, `completion_tokens`, `cache_creation_tokens`, `cache_read_tokens`, `reasoning_tokens`, `cost` (decimal), `request_id` |
| `Provider` | `id`, `name`, `display_name`, `currency` |
| `ModelDef` | `id`, `provider_id`, `model_name`, `billing_unit_type` |
| `ModelPricing` | `id`, `model_id`, `price_type`, `price_per_unit` (decimal/1M tokens), `effective_from`, `effective_to` |
| `ContractPricing` | `id`, `tenant_id`, `model_id`, `price_type`, `price_override` (decimal), `effective_from`, `effective_to` |
| `PricingMarkup` | `id`, `tenant_id`, `provider_id?`, `model_id?`, `percentage` (decimal), `priority`, `effective_from` |
| `CostLedger` | `id`, `tenant_id`, `idempotency_key`, `base_cost`, `markup_amount`, `final_cost`, `pricing_snapshot` (jsonb) |
| `BudgetLimit` | `id`, `tenant_id`, `period_type`, `limit_amount`, `alert_threshold`, `action` (alert\|block) |

### Pricing pipeline

```
POST /v1/agent/usage
        │
        ▼
  PricingEngine.Process()
        │
        ├─ 1. Resolve prices
        │      ContractPricing (tenant override) → ModelPricing (standard)
        │      Provider + ModelDef lookups Redis-cached 5 min
        │
        ├─ 2. Calculate base cost  (decimal arithmetic, never float64)
        │      Σ (tokens / unit_size × price_per_unit) per dimension
        │      Dimensions: input · output · cache_creation · cache_read · reasoning
        │
        ├─ 3. Apply markups
        │      SUM(markup.percentage) → base × (1 + total% / 100)
        │
        ├─ 4. Budget check (blocking limits only)
        │      Redis counter → fallback DB SUM → ErrBudgetExceeded → HTTP 402
        │
        ├─ 5. Write CostLedger  (idempotent on request_id)
        │
        └─ 6. Increment Redis budget counters (INCRBYFLOAT, ExpireAt end-of-period)
```

### Seeded pricing (effective 2024-01-01, per 1M tokens)

| Provider | Model | Input | Output | Cache Create | Cache Read |
|---|---|---|---|---|---|
| anthropic | claude-3-5-sonnet-20241022 | $3.00 | $15.00 | $3.75 | $0.30 |
| anthropic | claude-3-5-haiku-20241022 | $0.80 | $4.00 | $1.00 | $0.08 |
| anthropic | claude-3-opus-20240229 | $15.00 | $75.00 | $18.75 | $1.50 |
| anthropic | claude-sonnet-4-6 | $3.00 | $15.00 | $3.75 | $0.30 |
| anthropic | claude-opus-4-6 | $15.00 | $75.00 | $18.75 | $1.50 |
| openai | gpt-4o | $2.50 | $10.00 | — | — |
| openai | gpt-4o-mini | $0.15 | $0.60 | — | — |
| openai | gpt-4-turbo | $10.00 | $30.00 | — | — |
| openai | o1 | $15.00 | $60.00 | — | — |
| openai | o1-mini | $3.00 | $12.00 | — | — |
| google | gemini-1.5-pro | $1.25 | $5.00 | — | — |
| google | gemini-1.5-flash | $0.075 | $0.30 | — | — |
| google | gemini-2.0-flash | $0.10 | $0.40 | — | — |
| azure | gpt-4o | $2.50 | $10.00 | — | — |
| mistral | mistral-large | $2.00 | $6.00 | — | — |

### API endpoints

#### Public

| Method | Path | Description |
|---|---|---|
| GET | `/v1/health` | Health check |
| POST | `/v1/auth/sync` | Sync Clerk user → DB (creates tenant for new users) |

#### Agent (API key auth)

| Method | Path | Description |
|---|---|---|
| POST | `/v1/agent/usage` | Report LLM usage; cost computed server-side |

**Request:**
```json
{
  "provider": "anthropic",
  "model": "claude-sonnet-4-6",
  "input_tokens": 1200,
  "output_tokens": 400,
  "cache_creation_tokens": 0,
  "cache_read_tokens": 0,
  "reasoning_tokens": 0,
  "request_id": "req_abc123"
}
```
Legacy aliases `prompt_tokens` / `completion_tokens` are accepted.

**Response:**
```json
{
  "recorded": true,
  "base_cost": "0.02400000",
  "markup_amount": "0.00000000",
  "final_cost": "0.02400000"
}
```
Returns HTTP **402** if a blocking budget limit is exceeded. Returns HTTP **200** with `"idempotent": true` on duplicate `request_id`.

#### Viewer+

| Method | Path | Description |
|---|---|---|
| GET | `/v1/usage` | List tenant usage logs |
| GET | `/v1/usage/summary` | Aggregated stats *(not yet implemented)* |
| GET | `/v1/cost-ledger` | Paginated cost ledger (`?page=1&limit=50&from=&to=`) |
| GET | `/v1/usage/forecast` | Projected monthly spend based on daily average |

#### Admin+

| Method | Path | Description |
|---|---|---|
| GET / POST | `/v1/admin/users` | List / invite users |
| PATCH | `/v1/admin/users/:id/role` | Set role |
| PATCH | `/v1/admin/users/:id/suspend` | Suspend user |
| PATCH | `/v1/admin/users/:id/unsuspend` | Unsuspend user |
| DELETE | `/v1/admin/users/:id` | Remove user |
| GET / POST | `/v1/admin/api_keys` | List / create API keys |
| DELETE | `/v1/admin/api_keys/:id` | Revoke API key |
| GET / POST | `/v1/admin/provider_keys` | List / add provider keys |
| DELETE | `/v1/admin/provider_keys/:id` | Revoke provider key |
| GET / POST | `/v1/admin/pricing/providers` | List / add LLM providers |
| GET / POST | `/v1/admin/pricing/models` | List / add model definitions |
| GET / POST | `/v1/admin/pricing/model-pricing` | List / add versioned model pricing |
| GET / POST | `/v1/admin/pricing/markups` | List / create markup rules |
| DELETE | `/v1/admin/pricing/markups/:id` | Delete a markup rule |
| GET / POST | `/v1/admin/pricing/contracts` | List / create contract pricing overrides |
| GET | `/v1/admin/budget` | Get tenant budget limits |
| PUT | `/v1/admin/budget` | Upsert a budget limit |

> Price fields in admin requests use JSON strings (e.g. `"price_per_unit": "3.00"`) to avoid float precision loss.

#### Owner only

| Method | Path | Description |
|---|---|---|
| POST | `/v1/owner/users/:id/promote-admin` | Promote user to admin |
| DELETE | `/v1/owner/users/:id/demote-admin` | Demote admin to editor |
| POST | `/v1/owner/transfer-ownership` | Transfer workspace ownership |

### RBAC roles

| Role | Level | Permissions |
|---|---|---|
| `owner` | 4 | Everything + promote/demote admins, transfer ownership |
| `admin` | 3 | Invite/suspend/remove users, manage keys, manage pricing & budgets |
| `editor` | 2 | View usage, manage API keys |
| `viewer` | 1 | View usage, cost ledger, forecast |

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
    "input_tokens": 1200,
    "output_tokens": 400,
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
| Server-side cost computation (PricingEngine) | ✅ Live |
| Versioned model pricing catalog | ✅ Live |
| Immutable cost ledger | ✅ Live |
| Per-tenant markups | ✅ Live |
| Contract pricing overrides | ✅ Live |
| Budget enforcement (alert / block) | ✅ Live |
| Monthly spend forecast | ✅ Live |
| Usage summary / aggregation | 🚧 Not implemented |
| Provider key vault (Anthropic / OpenAI) | 🚧 Not implemented |
