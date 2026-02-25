# TokenGate Customer Integration Guide

This guide covers every supported provider + auth + billing scenario. Pick the section that matches your setup.

---

## Prerequisites

All scenarios require a **TokenGate account** and a **Gateway API Key**.

1. Sign up at [tokengate.to](https://tokengate.to) and create a workspace.
2. Go to the **Management** page and create a Gateway API Key with the provider, auth method, and billing mode that matches your scenario.
3. Copy the API key — it is shown only once.

---

## Anthropic Scenarios

### Scenario 1: Anthropic — Browser Auth + Monthly Subscription

**What this is:** Your developers use Claude Code with their own Anthropic subscriptions (Pro, Max, Team, or Enterprise). The gateway tracks usage for visibility but does not bill per token — costs are covered by each user's existing Anthropic plan.

**Step 1 — Create a Gateway API Key:**

Go to the **Management** page and create a key with:

| Field | Value |
|---|---|
| Provider | `anthropic` |
| Auth Method | `BROWSER_OAUTH` |
| Billing Mode | `MONTHLY_SUBSCRIPTION` |

**Step 2 — Developer setup (each machine):**

```bash
export ANTHROPIC_BASE_URL=https://gateway.tokengate.to
export ANTHROPIC_CUSTOM_HEADERS="X-TokenGate-Key:<tokengate-api-key>"
```

> **Important:** Do NOT include `/v1` in `ANTHROPIC_BASE_URL`. The Anthropic SDK appends `/v1/messages` automatically.

Then run `claude`. When prompted to choose an authentication method, select:

> **1. Claude account with subscription** · Pro, Max, Team, or Enterprise

A browser window will automatically open to complete the Anthropic login. Once authenticated, all requests are routed through the gateway and usage is recorded in your TokenGate dashboard.

**How it works:**
- Claude Code authenticates the user via browser OAuth with Anthropic directly.
- The gateway validates the TokenGate API key, passes the user's own auth through, and logs token usage.
- Costs appear in the dashboard for tracking, but billing happens through the user's Anthropic subscription.

---

### Scenario 2: Anthropic — Browser Auth + API Usage Billed

**What this is:** Your developers use Claude Code with their own Anthropic Console API keys. The gateway tracks usage and bills per token.

**Step 1 — Create a Gateway API Key:**

Go to the **Management** page and create a key with:

| Field | Value |
|---|---|
| Provider | `anthropic` |
| Auth Method | `BROWSER_OAUTH` |
| Billing Mode | `API_USAGE` |

**Step 2 — Developer setup (each machine):**

```bash
export ANTHROPIC_BASE_URL=https://gateway.tokengate.to
export ANTHROPIC_CUSTOM_HEADERS="X-TokenGate-Key:<tokengate-api-key>"
```

> **Important:** Do NOT include `/v1` in `ANTHROPIC_BASE_URL`. The Anthropic SDK appends `/v1/messages` automatically.

Then run `claude`. When prompted to choose an authentication method, select:

> **2. Anthropic Console account** · API usage billing

Claude Code will add its own Anthropic auth automatically. All requests are routed through the gateway with per-token billing.

**How it works:**
- The gateway validates the TokenGate API key and passes the client's Anthropic auth headers through to the upstream API.
- Token usage is recorded and billed per token (API usage mode).
- Budget limits and rate limits are enforced.

---

### Scenario 3: Anthropic — BYOK + API Usage Billed

**What this is:** Your organization stores an Anthropic API key in the TokenGate vault. Developers never see the raw key — the gateway injects it automatically. Usage is billed per token with full budget enforcement.

**Step 1 — Add a Provider Key (admin, once):**

1. Go to the **Management** page, find the **Provider Keys** section.
2. Click **Add Provider Key**, select **Anthropic**, and paste your `sk-ant-...` key.
3. Click **Activate** on the newly created key.

**Step 2 — Create a Gateway API Key:**

On the same **Management** page, create a key with:

| Field | Value |
|---|---|
| Provider | `anthropic` |
| Auth Method | `BYOK` |
| Billing Mode | `API_USAGE` |

> Remember to add your Anthropic provider key in Step 1 before using this gateway key.

**Step 3 — Use the API:**

Use the TokenGate API key in the `X-TokenGate-Key` header. The base URL is `https://gateway.tokengate.to` and API paths remain the same as the standard Anthropic API (e.g. `/v1/messages`).

No Anthropic API key is needed — the gateway injects it from the vault automatically.

**Example curl:**

```bash
curl https://gateway.tokengate.to/v1/messages \
    -H "X-TokenGate-Key: <tokengate-api-key>" \
    -H "Content-Type: application/json" \
    -d '{"model":"claude-sonnet-4-6","max_tokens":20,"messages":[{"role":"user","content":"Hello!"}]}'
```

**How it works:**
- The gateway validates the TokenGate API key, fetches your stored Anthropic key from the encrypted vault, and injects it into the upstream request.
- Developers never handle raw API credentials.
- Full budget enforcement (HTTP 402 if exceeded), rate limiting, and cost tracking apply.

---

## OpenAI Scenarios

### Scenario 4: OpenAI — Browser Auth + Monthly Subscription (Codex)

**What this is:** Your developers use the OpenAI Codex CLI with their own ChatGPT/OpenAI subscriptions. The gateway tracks usage for visibility but does not bill per token.

**Step 1 — Create a Gateway API Key:**

Go to the **Management** page and create a key with:

| Field | Value |
|---|---|
| Provider | `openai` |
| Auth Method | `BROWSER_OAUTH` |
| Billing Mode | `MONTHLY_SUBSCRIPTION` |

**Step 2 — Developer setup (each machine):**

Edit (or create) `~/.codex/config.toml` and paste the following at the top:

```toml
model_provider = "tokengate"

[model_providers.tokengate]
name = "TokenGate Proxy"
base_url = "https://gateway.tokengate.to/v1"
requires_openai_auth = true
wire_api = "responses"
http_headers = {
  "X-Tokengate-Key" = "<tokengate-api-key>"
}
```

Then run `codex` in a code repo. On first launch, Codex will prompt you to authenticate with OpenAI. You will see two options:

1. **Browser auth** — Opens a browser window for OpenAI login (recommended if you have a browser available).
2. **Device code auth** — Displays a code to enter at openai.com/device (useful for headless/remote machines).

Pick whichever suits your environment. Both work identically with the gateway.

**How it works:**
- Codex authenticates the user via OpenAI OAuth (browser or device code flow).
- The gateway validates the TokenGate API key and routes requests to the ChatGPT backend.
- Usage is estimated and tracked in the dashboard, but billing happens through the user's OpenAI subscription.

---

### Scenario 5: OpenAI — BYOK + API Usage Billed

**What this is:** Your organization stores an OpenAI API key in the TokenGate vault. The gateway injects it automatically. Usage is billed per token.

**Step 1 — Add a Provider Key (admin, once):**

1. Go to the **Management** page, find the **Provider Keys** section.
2. Click **Add Provider Key**, select **OpenAI**, and paste your `sk-...` key.
3. Click **Activate** on the newly created key.

**Step 2 — Create a Gateway API Key:**

On the same **Management** page, create a key with:

| Field | Value |
|---|---|
| Provider | `openai` |
| Auth Method | `BYOK` |
| Billing Mode | `API_USAGE` |

> Remember to add your OpenAI provider key in Step 1 before using this gateway key.

**Step 3 — Use the API:**

#### Option A: Codex CLI

Edit (or create) `~/.codex/config.toml` and replace its contents with:

```toml
model_provider = "tokengate"

[model_providers.tokengate]
name = "TokenGate Proxy"
base_url = "https://gateway.tokengate.to/v1"
wire_api = "responses"
http_headers = {
  "X-Tokengate-Key" = "<tokengate-api-key>"
}
```

Then run `codex` in a code repo. If prompted, select **"Provide your own API key"** — otherwise you are good to go. No separate OpenAI key is needed.

#### Option B: Direct API Calls (curl / SDK)

Use the TokenGate API key in the `X-TokenGate-Key` header. The base URL is `https://gateway.tokengate.to` and API paths remain the same as the standard OpenAI API (e.g. `/v1/openai/chat/completions`).

**Example curl:**

```bash
curl https://gateway.tokengate.to/v1/openai/chat/completions \
    -H "X-TokenGate-Key: <tokengate-api-key>" \
    -H "Content-Type: application/json" \
    -d '{"model":"gpt-4.1","max_tokens":20,"messages":[{"role":"user","content":"Hello!"}]}'
```

Or with environment variables for OpenAI SDK-compatible tools:

```bash
export OPENAI_BASE_URL=https://gateway.tokengate.to/v1/openai
export OPENAI_API_KEY="<tokengate-api-key>"
# No separate OpenAI key needed — the gateway uses your stored provider key
```

**How it works:**
- The gateway validates the TokenGate API key, fetches your stored OpenAI key from the encrypted vault, and injects it into the upstream request.
- Developers never handle raw API credentials.
- Full budget enforcement (HTTP 402 if exceeded), rate limiting, and cost tracking apply.

---

## API Endpoints Reference

| Method | Path | Provider | Description |
|---|---|---|---|
| POST | `/v1/messages` | Anthropic | Messages API proxy |
| GET | `/v1/models` | Anthropic | Model list passthrough |
| POST | `/v1/responses` | OpenAI / Anthropic | Responses API (provider-aware routing) |
| ANY | `/v1/openai/*` | OpenAI | OpenAI API passthrough (e.g. `/v1/openai/chat/completions`) |

---

## Budget & Rate Limit Headers

When budget alerts are triggered, the gateway adds these response headers:

| Header | Example | Description |
|---|---|---|
| `X-Tokengate-Budget-Warning` | `true` | Present when at or above alert threshold |
| `X-Tokengate-Budget-Limit` | `100.0000` | Configured limit amount |
| `X-Tokengate-Budget-Used` | `83.4200` | Current spend in the period |
| `X-Tokengate-Budget-Period` | `monthly` | `monthly`, `weekly`, or `daily` |
| `X-Tokengate-Budget-Scope` | `account` | `account` or `api_key` |

If a blocking limit is exceeded, the gateway returns **HTTP 402**:

```json
{
  "error": "budget_exceeded",
  "message": "Budget limit exceeded for period=monthly. Limit: 100.0000, Current: 105.2300"
}
```

---

## Quick Reference: Scenario Matrix

| # | Provider | Auth Method | Billing Mode | Client Tool | Auth Flow |
|---|---|---|---|---|---|
| 1 | Anthropic | Browser OAuth | Monthly Subscription | Claude Code | Browser login to Anthropic |
| 2 | Anthropic | Browser OAuth | API Usage | Claude Code | Client provides own auth |
| 3 | Anthropic | BYOK | API Usage | curl / SDK | Gateway injects key |
| 4 | OpenAI | Browser OAuth | Monthly Subscription | Codex CLI | Browser or device code login to OpenAI |
| 5 | OpenAI | BYOK | API Usage | Codex CLI / curl / SDK | Gateway injects key |

---

## Troubleshooting

| Issue | Cause | Fix |
|---|---|---|
| `404` on Anthropic requests | `/v1` included in `ANTHROPIC_BASE_URL` | Remove `/v1` — use `https://gateway.tokengate.to` only |
| `401 Unauthorized` | Invalid or expired gateway key | Check key in Management dashboard, create a new one if needed |
| `402 Budget Exceeded` | Tenant spend above blocking limit | Increase budget limit in Management dashboard |
| `403 Forbidden` | API key provider does not match the endpoint path | Ensure key provider matches: `anthropic` keys for `/v1/messages`, `openai` keys for `/v1/openai/*` |
| Codex "no auth" error | Missing `requires_openai_auth = true` in config | Add the field for Browser OAuth + Monthly Subscription scenarios |
| No usage showing in dashboard | Provider key not activated | Go to Provider Keys and click Activate |
