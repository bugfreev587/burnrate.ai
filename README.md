# TokenGate

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
- **API key management** – Admins create and revoke agent API keys; secrets are stored hashed and shown only once. Key limits, team size, budget options, and data retention are gated by the tenant's plan tier.
- **Plan tiers** – Free / Pro / Team / Business. Each tier controls API key count, team member count, allowed budget period types, hard-block permissions, per-key budget scope, and data retention window.
- **Multi-tenant isolation** – Every organization gets its own workspace; data is fully separated.

## Architecture

```
tokengate/
├── api-server/   # Go 1.24 backend (Gin + PostgreSQL + Redis)
└── dashboard/    # React 19 + TypeScript frontend (Vite + Clerk)
```

The frontend is hosted on **Vercel**. The backend runs in a **Docker container on Railway** backed by a managed PostgreSQL and Redis instance.

---

## Anthropic Gateway Proxy

The gateway acts as a drop-in replacement for the Anthropic API. Configure Claude Code (or any Anthropic SDK client) like this:

```bash
export ANTHROPIC_BASE_URL=https://your-gateway.railway.app/v1
export ANTHROPIC_API_KEY=<key_id>:<secret>   # your TokenGate tg_xxx key
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
| `X-Tokengate-Budget-Warning` | `true` | Present when at or above alert threshold |
| `X-Tokengate-Budget-Limit` | `100.0000` | The configured limit amount |
| `X-Tokengate-Budget-Used` | `83.4200` | Current spend in the period |
| `X-Tokengate-Budget-Period` | `monthly` | Budget period: `monthly`, `weekly`, or `daily` |
| `X-Tokengate-Budget-Scope` | `account` | `account` or `api_key` |

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

### Usage log insertion: N SSE events → 1 row

A single LLM request streams many SSE chunks, but always results in **exactly one `usage_logs` row**. The aggregation happens in layers:

```
Anthropic SSE stream (many events)
  │
  │  message_start  → captures input_tokens, cache_creation_tokens,
  │                    cache_read_tokens, message_id, model
  │  content_block_* → forwarded to client, no token data
  │  message_delta  → captures output_tokens (final cumulative count)
  │  message_stop   → end of stream
  │
  ▼
ParseSSE()  (proxy/stream.go)
  Reads every chunk, forwards each to the client in real-time via http.Flusher,
  accumulates token fields into a single TokenCounts struct.
  Returns ONE TokenCounts when the stream ends.
  │
  ▼
handler.go  publishes ONE UsageEventMsg to Redis Streams
  (message_id from message_start becomes the idempotency key)
  │
  ▼
Redis Stream  "tokengate:usage:events"
  │
  ▼
UsageWorker.process()  (events/worker.go)
  Reads the single stream message, creates ONE UsageLog record,
  inserts via UsageLogService.Create() (GORM db.Create).
  request_id column has a UNIQUE constraint — duplicate delivery
  from Redis is detected and silently skipped, not double-counted.
  Also runs PricingEngine for cost ledger + budget counter updates.
  ACKs the message on success; re-delivers on failure.
  │
  ▼
PostgreSQL  usage_logs table  (1 row per request)
```

**Non-streaming** (regular JSON response): `extractTokensFromJSON()` reads the body once and returns the same `TokenCounts` shape — the rest of the pipeline is identical.

**Idempotency**: `UsageLog.RequestID` maps to the Anthropic `message_id` (e.g. `msg_01XYZ…`). The unique index means even if the Redis consumer crashes mid-processing and re-delivers the same message, no duplicate row is inserted.

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

After each proxied request completes, a `UsageEventMsg` is published to the Redis stream `tokengate:usage:events`. A background worker (`UsageWorker`) consumes from consumer group `tokengate:usage:workers`:

```
proxy handler  ──XADD──▶  tokengate:usage:events  ──XREADGROUP──▶  UsageWorker
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
    │   ├── provider_keys.go          # Provider key CRUD + activate + rotate
    │   └── middleware.go             # CORS · logger (errors + slow only) · rate-limit
    ├── middleware/
    │   ├── auth.go                   # API key validation
    │   └── rbac.go                   # RBAC (user lookup + role gates)
    ├── models/
    │   ├── models.go                 # Tenant · User · APIKey · ProviderKey
    │   │                             # TenantProviderSettings · UsageLog
    │   ├── plans.go                  # Plan tier constants + PlanLimits struct + GetPlanLimits()
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
    │   └── providerkey_service.go    # AES-256-GCM envelope encryption + in-process cache (30s idle / 5m hard)
    │                                 # + Redis TPS cache (30s idle / 5m hard) + atomic Rotate + policy_version
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
| `Tenant` | `id`, `name`, `plan` (free\|pro\|team\|business, default free), `max_api_keys` (derived from plan) |
| `User` | `id` (Clerk ID), `tenant_id`, `email`, `role`, `status` |
| `APIKey` | `key_id`, `tenant_id`, `label`, `hash`, `salt`, `scopes`, `expires_at` |
| `ProviderKey` | `id`, `tenant_id`, `provider`, `label`, `encrypted_key`, `key_nonce`, `encrypted_dek`, `dek_nonce` |
| `TenantProviderSettings` | `tenant_id`, `provider`, `active_key_id`, `policy_version` (bumped on every activate/rotate) |
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
- Decrypted plaintexts are cached **in-process** (`sync.RWMutex` map keyed by `ProviderKey.ID`) with a **sliding idle TTL of 30 seconds** (reset on every cache hit) and an **absolute hard ceiling of 5 minutes** (set once at write time, never extended). A TOCTOU guard re-checks the entry under a write lock before extending the idle expiry. Redis never stores plaintext keys.
- `TenantProviderSettings` (the active key pointer) is cached in Redis under `tps:{tenantID}:{provider}` with a **sliding idle TTL of 30 seconds** (refreshed via `EXPIRE` on each hit) and a **hard ceiling of 5 minutes** embedded as a `hard_expiry` JSON field in the value. On a cache hit the hard expiry is checked first; if exceeded the entry is DEL'd and the lookup falls through to Postgres.
- Both caches are invalidated immediately on `Activate` and `Rotate` — the in-process entry is evicted by key ID, the Redis TPS entry is DEL'd — so every pod picks up the new key on its very next request.
- `policy_version` on `TenantProviderSettings` is a monotonically-increasing counter bumped atomically (SQL `+ 1`) on every `Activate` or `Rotate`. It is stored in the Redis TPS cache entry and serves as a staleness signal: if the TTL-based self-heal fires, the version mismatch is observable in the cache entry.
- Revoking a key evicts it from the in-process cache and clears `TenantProviderSettings` if it was the active key.

### Provider key rotation

A single atomic endpoint replaces the old 3-step manual flow:

```bash
# Atomic rotate: store new key + activate + revoke old key in one DB transaction
curl -X POST https://your-gateway/v1/admin/provider_keys/<old_key_id>/rotate \
  -H "X-User-ID: <clerk_user_id>" \
  -H "Content-Type: application/json" \
  -d '{"label": "prod-key-v2", "api_key": "sk-ant-new..."}'
```

The gateway will:
1. Verify the old key belongs to the tenant and is not already revoked.
2. Encrypt the new key (fresh DEK + nonces).
3. In a single DB transaction: INSERT new key → UPDATE `TenantProviderSettings` (`active_key_id` = new, `policy_version + 1`) → SET `old_key.revoked = true`.
4. Evict the old key from the in-process plaintext cache.
5. DEL the Redis TPS cache entry so every pod re-fetches from DB on the next request.

Returns the new key's metadata (`id`, `provider`, `label`, `is_active: true`, `created_at`).

### Plan tiers

Every tenant has a `plan` field that gates feature access. New tenants start on `free`.

| | Free | Pro | Team | Business |
|---|---|---|---|---|
| **API keys** | 1 | 5 | Unlimited | Unlimited |
| **Team members** | 1 | 1 | 10 | Unlimited |
| **Budget periods** | Monthly only | Monthly · Weekly · Daily | Monthly · Weekly · Daily | Monthly · Weekly · Daily |
| **Hard block** (`action: "block"`) | — | ✓ | ✓ | ✓ |
| **Per-key budget scope** | — | — | ✓ | ✓ |
| **Data retention** | 30 days | 90 days | 1 year | Unlimited |

Plan limits are defined in `internal/models/plans.go` and enforced at the API layer:

- `POST /v1/admin/api_keys` — returns HTTP 422 (`plan_limit_reached`) if active key count ≥ plan limit
- `POST /v1/admin/users/invite` — returns HTTP 422 (`plan_limit_reached`) if member count ≥ plan limit
- `PUT /v1/admin/budget` — returns HTTP 403 (`plan_restriction`) if the requested period type, `block` action, or `api_key` scope is not allowed on the current plan
- `GET /v1/usage` and `GET /v1/cost-ledger` — results are silently bounded to the plan's data retention window

Plan limits are returned in `GET /v1/owner/settings` under the `plan_limits` key.

The plan field is updated directly in the database (no self-serve upgrade UI yet — intended for future billing integration).

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
| GET / POST | `/v1/admin/api_keys` | List / create API keys (response includes `count`, `limit`, `slots_left`, `plan`; `limit`/`slots_left` are `null` for unlimited plans) |
| DELETE | `/v1/admin/api_keys/:id` | Revoke API key |
| GET / POST | `/v1/admin/provider_keys` | List / add provider keys |
| DELETE | `/v1/admin/provider_keys/:id` | Revoke provider key |
| PUT | `/v1/admin/provider_keys/:id/activate` | Set as active key for its provider (bumps `policy_version`) |
| POST | `/v1/admin/provider_keys/:id/rotate` | Atomic rotate: store new key, activate, revoke old — one transaction |
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
  "scope_type": "account",   // "account" (default) | "api_key" (Team+ only)
  "scope_id": "",            // "" for account scope; key_id for api_key scope
  "period_type": "monthly",  // "monthly" | "weekly" | "daily" (weekly/daily require Pro+)
  "limit_amount": "100.00",  // decimal string
  "alert_threshold": "80",   // percentage to trigger warning headers (default: 80)
  "action": "block"          // "alert" (headers only) | "block" (HTTP 402, requires Pro+)
}
```

Returns HTTP **403** with `"error": "plan_restriction"` if `period_type`, `action`, or `scope_type` is not permitted on the tenant's current plan.

#### Owner only

| Method | Path | Description |
|---|---|---|
| POST | `/v1/owner/users/:id/promote-admin` | Promote user to admin |
| DELETE | `/v1/owner/users/:id/demote-admin` | Demote admin to editor |
| POST | `/v1/owner/transfer-ownership` | Transfer workspace ownership |
| GET | `/v1/owner/settings` | View tenant settings: `name`, `plan`, `max_api_keys`, `plan_limits` object |
| PATCH | `/v1/owner/settings` | Update `max_api_keys` (range: 1–1000, capped at plan ceiling; only meaningful for unlimited plans) |

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

**Stack:** React 19 · TypeScript · Vite · Tailwind CSS v3 · Clerk · Recharts · React Router

### Directory layout

```
dashboard/
├── public/
│   └── favicon.svg              # TokenGate shield icon (custom SVG)
├── src/
│   ├── assets/
│   │   ├── logo-light.svg       # Full logo for light backgrounds (gradient shield + wordmark)
│   │   └── logo-dark.svg        # Full logo for dark backgrounds (brighter gradient)
│   ├── main.tsx                 # Clerk provider setup
│   ├── App.tsx                  # Routes + auth guards
│   ├── pages/
│   │   ├── LandingPage.tsx      # Marketing landing page (/)
│   │   ├── SignInPage.tsx       # Clerk sign-in embed
│   │   ├── SignUpPage.tsx       # Clerk sign-up embed
│   │   ├── Dashboard.tsx        # Usage summary + log table
│   │   ├── ProfilePage.tsx      # Clerk profile embed
│   │   ├── ManagementPage.tsx   # Team, API key, and provider key management
│   │   ├── PricingConfigPage.tsx# Per-key pricing overrides
│   │   └── PlanPage.tsx         # Plan tier + usage meters + comparison table (owner only)
│   ├── components/
│   │   ├── Navbar.tsx           # Dashboard top nav + user menu (dark theme, logo-dark.svg)
│   │   ├── APIKeyModal.tsx      # One-time secret display
│   │   └── landing/             # Landing page components (Tailwind-scoped)
│   │       ├── LandingNav.tsx   # Auth-aware landing nav (logo-light.svg, Dashboard/avatar when signed in)
│   │       ├── LandingHero.tsx
│   │       ├── LandingProblem.tsx
│   │       ├── LandingFeatures.tsx
│   │       ├── LandingHowItWorks.tsx
│   │       ├── LandingPricing.tsx
│   │       └── LandingFooter.tsx
│   └── hooks/
│       ├── useUserSync.ts       # Clerk ↔ backend sync
│       ├── useUsageData.ts      # Usage log fetcher
│       └── usePricingConfig.ts  # Pricing config fetcher
├── tailwind.config.ts           # Tailwind v3; preflight disabled; scoped to landing/** only
├── postcss.config.js            # PostCSS (tailwindcss + autoprefixer)
├── vercel.json                  # SPA rewrite (all routes → index.html)
└── .env.example
```

### Pages

- **LandingPage** – Public marketing page at `/` with hero, problem, features, how-it-works, pricing, and footer sections. Built with Tailwind CSS (scoped to avoid conflict with the existing dark-theme CSS variables used by the dashboard).
- **Dashboard** – Summary cards (requests / tokens / cost) and a paginated usage table.
- **ManagementPage** – Team members table (invite, change role, suspend, remove), Gateway API Keys table (create, revoke, one-time secret display), Provider Keys table (add, activate, revoke).
- **PricingConfigPage** – Create named pricing configs and assign them to individual API keys for per-key price overrides.
- **PlanPage** – Owner-only. Shows current plan badge, live usage meters (API keys used / limit, members used / limit), a full four-tier comparison table with the current plan highlighted, and an upgrade CTA.

### Key hooks

- **`useUserSync`** – Runs once after Clerk sign-in. Calls `POST /v1/auth/sync`, stores `userId`, `tenantId`, `role`, and `status` in state and `localStorage`. Handles three cases: existing user, pending email invitation, and brand-new user (creates tenant).
- **`useUsageData`** – Calls `GET /v1/usage`, returns logs + refresh function.

### Branding

- `logo-light.svg` — used in the landing page nav (light white header background). Blue `#0A6BFF` → green `#14B86A` gradient shield with white keyhole.
- `logo-dark.svg` — used in the dashboard nav (dark background). Brighter gradient `#2D7DFF` → `#23D17E` with full-opacity white strokes for contrast.
- `favicon.svg` — simplified 32 × 32 shield + keyhole, same gradient. Referenced from `index.html` via `<link rel="icon" type="image/svg+xml" href="/favicon.svg">`.

### Environment variables

```
VITE_CLERK_PUBLISHABLE_KEY=pk_live_...
VITE_API_SERVER_URL=https://gateway.tokengate.to
```

---

## Deployment

### Backend (Railway)

The API server is deployed as a Docker container on Railway.

```bash
docker build -t tokengate-api ./api-server
```

Required Railway environment variables:

```
POSTGRES_DB_URL=postgresql://...
REDIS_URL=redis://...
API_KEY_PEPPER=<random-secret>
CORS_ORIGINS=https://app.tokengate.to
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
VITE_API_SERVER_URL=https://gateway.tokengate.to
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
export ANTHROPIC_BASE_URL=https://gateway.tokengate.to/v1
export ANTHROPIC_API_KEY=<key_id>:<secret>

# All claude / SDK requests now route through the gateway automatically.
claude -p "Hello"
```

### Agent reporting usage (legacy direct path)

```bash
curl -X POST https://gateway.tokengate.to/v1/agent/usage \
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
| Plan tier enforcement (free / pro / team / business) | ✅ Live |
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
