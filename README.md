# burnrate-ai

A multi-tenant usage tracking and management gateway for Claude Code and other LLM agents. Teams use it to report, visualize, and control their LLM API spending — or route all requests through the built-in Anthropic reverse proxy so usage is captured automatically.

## What it does

- **Anthropic reverse proxy** – Agents set `ANTHROPIC_BASE_URL` to the gateway; the gateway authenticates the `br_xxx` key, fetches the tenant's stored Anthropic key, and forwards the request. Token usage is extracted from the SSE stream and logged automatically.
- **Provider key vault** – Anthropic and OpenAI keys are stored with AES-256-GCM envelope encryption (per-key DEK, master key in env). Admins add, activate, and rotate keys from the Management dashboard; developers never touch raw credentials.
- **Usage tracking** – Agents can also call a simple HTTP endpoint to report token consumption after each LLM request (legacy path, still supported).
- **Server-side cost computation** – Cost is computed authoritatively from token counts using versioned, per-provider pricing. Client-provided costs are ignored.
- **Cost ledger** – Every priced request is written to an immutable ledger for financial auditing and forecasting.
- **Spend forecasting** – Daily-average extrapolation gives a projected monthly spend based on actual usage so far.
- **Budget enforcement** – Per-tenant spend limits (monthly / weekly / daily) can alert or block requests when exceeded. Limits apply at the account scope or per API key. Blocking limits are checked **before** forwarding to Anthropic (HTTP 402 if exceeded). Warning headers are set on responses when spend crosses the alert threshold.
- **Markup / monetization** – Admins configure percentage markups per provider, model, or globally to bill tenants above cost.
- **Dashboard** – Owners and team members see total requests, tokens, and costs with a per-request history table.
- **Team management** – Invite members by email, assign roles, suspend or remove users.
- **API key management** – Admins create and revoke agent API keys; secrets are stored hashed and shown only once. Each tenant has a configurable limit (default 5, owner-adjustable up to 100).
- **Multi-tenant isolation** – Every organization gets its own workspace; data is fully separated.

## Architecture

```
burnrate-ai/
├── api-server/   # Go 1.24 backend (Gin + PostgreSQL + Redis)
└── dashboard/    # React 19 + TypeScript frontend (Vite + Clerk)
```

The frontend is hosted on **Vercel**. The backend runs in a **Docker container on Railway** backed by a managed PostgreSQL and Redis instance.

---

## Anthropic Gateway Proxy

The gateway acts as a drop-in replacement for the Anthropic API. Configure Claude Code (or any Anthropic SDK client) like this:

```bash
export ANTHROPIC_BASE_URL=https://your-gateway.railway.app/v1
export ANTHROPIC_API_KEY=<key_id>:<secret>   # your burnrate br_xxx key
```

The gateway will:
1. Validate the `br_xxx` key and resolve the tenant (and the key's own ID for per-key budget tracking).
2. **Pre-check budget** — if the tenant's current spend already equals or exceeds a blocking budget limit, return HTTP 402 immediately (no upstream call made). If spend is above the alert threshold, response headers are set (see below).
3. Fetch the tenant's active Anthropic provider key from the encrypted vault.
4. Forward the request to `api.anthropic.com` with the real key.
5. Stream the SSE response directly to the client with no buffering.
6. Parse token usage from `message_start` / `message_delta` SSE events on-the-fly.
7. Publish a usage event (including the originating `key_id`) to Redis Streams for async processing (usage log + cost ledger + budget counter increments).

### Proxy endpoints

| Method | Path | Description |
|---|---|---|
| POST | `/v1/messages` | Full Anthropic Messages API proxy (streaming + non-streaming) |
| GET | `/v1/models` | Anthropic models list passthrough |

### Budget response headers

When the tenant's spend is at or above the configured `alert_threshold` percentage of any applicable limit, the proxy sets these headers on the response:

| Header | Example | Description |
|---|---|---|
| `X-Burnrate-Budget-Warning` | `true` | Present when at or above alert threshold |
| `X-Burnrate-Budget-Limit` | `100.0000` | The configured limit amount |
| `X-Burnrate-Budget-Used` | `83.4200` | Current spend in the period |
| `X-Burnrate-Budget-Period` | `monthly` | Budget period: `monthly`, `weekly`, or `daily` |
| `X-Burnrate-Budget-Scope` | `account` | `account` or `api_key` |

If a **blocking** limit is exceeded, the proxy returns HTTP **402** with:
```json
{
  "error": "budget_exceeded",
  "message": "Budget limit exceeded for period=monthly. Limit: 100.0000, Current: 105.2300"
}
```

### SSE streaming implementation

- **No full-body buffering**: bytes flow from Anthropic → gateway → client continuously via `bufio.Scanner` + `http.Flusher`.
- **Anti-buffering headers**: `X-Accel-Buffering: no` and `Cache-Control: no-cache` prevent Railway's proxy layer from buffering the stream.
- **No overall HTTP timeout**: the upstream client uses `ResponseHeaderTimeout: 30s` (fail fast if Anthropic is unresponsive) with no body-read timeout, so long streaming responses are never cut off.
- **Client disconnect propagation**: upstream request is bound to `c.Request.Context()`; if the client disconnects, the upstream connection is cancelled automatically.
- **Streaming detection**: from the upstream `Content-Type: text/event-stream` response header (not the request body).
- **Token extraction**: parses `message_start` for input/cache tokens and message ID; `message_delta` for output tokens. Extraction is independent of forwarding — bytes are never delayed for parsing.

### Setting up provider keys

```bash
# 1. Generate a 32-byte master encryption key (do this once, store it safely)
openssl rand -hex 32

# 2. Set it in Railway:
#    PROVIDER_KEY_ENCRYPTION_KEY=<64-hex-chars>

# 3. Add and activate a key via the Management dashboard, or via API:
curl -X POST https://your-gateway/v1/admin/provider_keys \
  -H "X-User-ID: <clerk_user_id>" \
  -H "Content-Type: application/json" \
  -d '{"provider": "anthropic", "label": "prod-key", "api_key": "sk-ant-..."}'

curl -X PUT https://your-gateway/v1/admin/provider_keys/<id>/activate \
  -H "X-User-ID: <clerk_user_id>"
```

### Async usage processing (Redis Streams)

After each proxied request completes, a `UsageEventMsg` is published to the Redis stream `burnrate:usage:events`. A background worker (`UsageWorker`) consumes from consumer group `burnrate:usage:workers`:

```
proxy handler  ──XADD──▶  burnrate:usage:events  ──XREADGROUP──▶  UsageWorker
                                                                         │
                                                              ┌──────────┴──────────┐
                                                              ▼                     ▼
                                                         UsageLog DB          PricingEngine
                                                         (request_id          (cost ledger +
                                                          idempotency)         budget counters)
```

Messages are ACK'd only on successful processing. Failed messages are redelivered automatically.

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
    │   ├── provider_keys.go          # Provider key CRUD + activate
    │   └── middleware.go             # CORS · logger (errors + slow only) · rate-limit
    ├── middleware/
    │   ├── auth.go                   # API key validation
    │   └── rbac.go                   # RBAC (user lookup + role gates)
    ├── models/
    │   ├── models.go                 # Tenant · User · APIKey · ProviderKey
    │   │                             # TenantProviderSettings · UsageLog
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
    │   ├── usage_service.go          # Usage log CRUD
    │   └── providerkey_service.go    # AES-256-GCM envelope encryption + in-process cache
    ├── events/
    │   ├── queue.go                  # Redis Streams producer (XADD)
    │   └── worker.go                 # Redis Streams consumer (XREADGROUP + XACK)
    ├── proxy/
    │   ├── handler.go                # Reverse proxy for /v1/messages and /v1/models
    │   └── stream.go                 # SSE parser + token extractor
    ├── db/postgres.go                # GORM init + AutoMigrate + seed
    └── config/config.go              # YAML config + env overrides
```

### Data models

| Model | Key fields |
|---|---|
| `Tenant` | `id`, `name`, `max_api_keys` (default 5) |
| `User` | `id` (Clerk ID), `tenant_id`, `email`, `role`, `status` |
| `APIKey` | `key_id`, `tenant_id`, `label`, `hash`, `salt`, `scopes`, `expires_at` |
| `ProviderKey` | `id`, `tenant_id`, `provider`, `label`, `encrypted_key`, `key_nonce`, `encrypted_dek`, `dek_nonce` |
| `TenantProviderSettings` | `tenant_id`, `provider`, `active_key_id` |
| `UsageLog` | `id`, `tenant_id`, `provider`, `model`, `prompt_tokens`, `completion_tokens`, `cache_creation_tokens`, `cache_read_tokens`, `reasoning_tokens`, `cost` (decimal), `request_id` |
| `Provider` | `id`, `name`, `display_name`, `currency` |
| `ModelDef` | `id`, `provider_id`, `model_name`, `billing_unit_type` |
| `ModelPricing` | `id`, `model_id`, `price_type`, `price_per_unit` (decimal/1M tokens), `effective_from`, `effective_to` |
| `ContractPricing` | `id`, `tenant_id`, `model_id`, `price_type`, `price_override` (decimal), `effective_from`, `effective_to` |
| `PricingMarkup` | `id`, `tenant_id`, `provider_id?`, `model_id?`, `percentage` (decimal), `priority`, `effective_from` |
| `CostLedger` | `id`, `tenant_id`, `idempotency_key`, `base_cost`, `markup_amount`, `final_cost`, `pricing_snapshot` (jsonb) |
| `BudgetLimit` | `id`, `tenant_id`, `scope_type` (account\|api_key), `scope_id` (key_id or ""), `period_type`, `limit_amount`, `alert_threshold`, `action` (alert\|block) |
| `APIKeyConfig` | `id`, `tenant_id`, `key_id` (varchar 64), `config_id` |

### Provider key encryption

Provider keys use AES-256-GCM **envelope encryption**:

```
plaintext_key
      │
      ▼ AES-256-GCM (random DEK)
encrypted_key + key_nonce
                              DEK
                               │
                               ▼ AES-256-GCM (master key)
                          encrypted_dek + dek_nonce
```

- A fresh 32-byte DEK and 12-byte nonce are generated per key using `crypto/rand`. Nonces are never reused.
- The master key is a 32-byte value decoded from `PROVIDER_KEY_ENCRYPTION_KEY` (64-char hex). The server refuses to start if this variable is unset.
- Decrypted keys are cached in-process with a 5-minute TTL (`sync.RWMutex` map). Redis never stores plaintext keys.
- Revoking a key evicts it from cache and clears `TenantProviderSettings` if it was active.

### Pricing pipeline

```
POST /v1/agent/usage   (or via proxy → Redis Streams → worker)
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
        ├─ 4. Budget check (blocking limits, scope-aware)
        │      Checks account-level AND api_key-level limits
        │      Redis counter → fallback DB SUM (account) → ErrBudgetExceeded
        │      Note: proxy pre-checks BEFORE forwarding (step 2 above);
        │            worker re-checks on final cost after the fact
        │
        ├─ 5. Write CostLedger  (idempotent on request_id / message_id)
        │
        └─ 6. Increment Redis budget counters (INCRBYFLOAT, ExpireAt end-of-period)
               Increments both account-level and api_key-level counters per period
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

#### Agent / proxy (API key auth)

| Method | Path | Description |
|---|---|---|
| POST | `/v1/messages` | Anthropic Messages API proxy (streaming + non-streaming) |
| GET | `/v1/models` | Anthropic models list passthrough |
| POST | `/v1/agent/usage` | Report LLM usage directly (legacy path); cost computed server-side |

**Legacy usage request:**
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
| GET / POST | `/v1/admin/api_keys` | List / create API keys (response includes `count`, `limit`, `slots_left`) |
| DELETE | `/v1/admin/api_keys/:id` | Revoke API key |
| GET / POST | `/v1/admin/provider_keys` | List / add provider keys |
| DELETE | `/v1/admin/provider_keys/:id` | Revoke provider key |
| PUT | `/v1/admin/provider_keys/:id/activate` | Set as active key for its provider |
| GET / POST | `/v1/admin/pricing/providers` | List / add LLM providers |
| GET / POST | `/v1/admin/pricing/models` | List / add model definitions |
| GET / POST | `/v1/admin/pricing/model-pricing` | List / add versioned model pricing |
| GET / POST | `/v1/admin/pricing/markups` | List / create markup rules |
| DELETE | `/v1/admin/pricing/markups/:id` | Delete a markup rule |
| GET / POST | `/v1/admin/pricing/contracts` | List / create contract pricing overrides |
| GET | `/v1/admin/budget` | List all budget limits for the tenant |
| PUT | `/v1/admin/budget` | Upsert a budget limit (scope: `account` or `api_key`) |
| DELETE | `/v1/admin/budget/:budget_id` | Delete a budget limit by ID |

> Price fields in admin requests use JSON strings (e.g. `"price_per_unit": "3.00"`) to avoid float precision loss.

**Budget upsert request body:**
```json
{
  "scope_type": "account",   // "account" (default) | "api_key"
  "scope_id": "",            // "" for account scope; key_id for api_key scope
  "period_type": "monthly",  // "monthly" | "weekly" | "daily"
  "limit_amount": "100.00",  // decimal string
  "alert_threshold": "80",   // percentage to trigger warning headers (default: 80)
  "action": "block"          // "alert" (headers only) | "block" (HTTP 402)
}
```

#### Owner only

| Method | Path | Description |
|---|---|---|
| POST | `/v1/owner/users/:id/promote-admin` | Promote user to admin |
| DELETE | `/v1/owner/users/:id/demote-admin` | Demote admin to editor |
| POST | `/v1/owner/transfer-ownership` | Transfer workspace ownership |
| GET | `/v1/owner/settings` | View tenant settings (`name`, `max_api_keys`) |
| PATCH | `/v1/owner/settings` | Update tenant settings — `{ "max_api_keys": 25 }` (range: 1–100) |

### RBAC roles

| Role | Level | Permissions |
|---|---|---|
| `owner` | 4 | Everything + promote/demote admins, transfer ownership, adjust API key limit |
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
| `PROVIDER_KEY_ENCRYPTION_KEY` | 64-char hex (32-byte) AES master key for provider key encryption. **Required** — server fails to start if unset. Generate with `openssl rand -hex 32`. |

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
│   │   ├── ManagementPage.tsx   # Team, API key, and provider key management
│   │   └── PricingConfigPage.tsx# Per-key pricing overrides
│   ├── components/
│   │   ├── Navbar.tsx           # Top nav + user menu
│   │   └── APIKeyModal.tsx      # One-time secret display
│   └── hooks/
│       ├── useUserSync.ts       # Clerk ↔ backend sync
│       ├── useUsageData.ts      # Usage log fetcher
│       └── usePricingConfig.ts  # Pricing config fetcher
├── vercel.json                  # SPA rewrite (all routes → index.html)
└── .env.example
```

### Pages

- **Dashboard** – Summary cards (requests / tokens / cost) and a paginated usage table.
- **ManagementPage** – Team members table (invite, change role, suspend, remove), Gateway API Keys table (create, revoke, one-time secret display), Provider Keys table (add, activate, revoke).
- **PricingConfigPage** – Create named pricing configs and assign them to individual API keys for per-key price overrides.

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
PROVIDER_KEY_ENCRYPTION_KEY=<openssl rand -hex 32>
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

### Using the Anthropic gateway proxy

```bash
# One-time setup: add and activate a provider key via the Management dashboard.
# Then configure Claude Code:
export ANTHROPIC_BASE_URL=https://burnrateai-production.up.railway.app/v1
export ANTHROPIC_API_KEY=<key_id>:<secret>

# All claude / SDK requests now route through the gateway automatically.
claude -p "Hello"
```

### Agent reporting usage (legacy direct path)

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
| Per-tenant API key limit (default 5, owner-configurable) | ✅ Live |
| Server-side cost computation (PricingEngine) | ✅ Live |
| Versioned model pricing catalog | ✅ Live |
| Immutable cost ledger | ✅ Live |
| Per-tenant markups | ✅ Live |
| Contract pricing overrides | ✅ Live |
| Budget enforcement (alert / block) | ✅ Live |
| Monthly spend forecast | ✅ Live |
| Provider key vault (AES-256-GCM envelope encryption) | ✅ Live |
| Anthropic reverse proxy (`/v1/messages`) | ✅ Live |
| SSE streaming proxy with token extraction | ✅ Live |
| Async usage processing via Redis Streams | ✅ Live |
| Usage summary / aggregation | 🚧 Not implemented |
