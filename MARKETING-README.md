# TokenGate

**The control layer for AI coding tools.**

TokenGate sits between your AI tools (Claude Code, Codex, Cursor, etc.) and the LLM providers behind them. It gives you real-time visibility into what your team is spending, and lets you enforce budgets before the bill arrives.

Works in 30 seconds. Set two environment variables. No code changes. No SDK.

---

## The problem

AI coding tools are transforming how teams build software. But they come with a blind spot.

**If you're on a subscription** (Claude Pro, ChatGPT Plus), you don't know your real usage. You hit caps unexpectedly. You can't tell which project or developer is consuming your quota.

**If you're on API billing** (Anthropic API, OpenAI API), you find out what you spent after the bill arrives. One runaway agent can burn $200 overnight. There's no way to enforce budgets across your team before the money is gone.

Either way, you don't have control.

---

## How TokenGate works

```
Your AI tool                    TokenGate                     LLM Provider
(Claude Code,        →       (gateway proxy)        →       (Anthropic,
 Codex, etc.)                                                 OpenAI, etc.)
                         ┌─────────────────────┐
                         │  Authenticate key    │
                         │  Check budget limits │
                         │  Check rate limits   │
                         │  Forward request     │
                         │  Track tokens + cost │
                         │  Log everything      │
                         └─────────────────────┘
```

1. **Point your AI tool to TokenGate** — two environment variables. That's it.
2. **Use your tools as normal** — same commands, same editor, same speed.
3. **TokenGate tracks + enforces** — every request is logged, priced, and checked against your policies in real time.

### Setup: Claude Code (CLI)

```bash
export ANTHROPIC_BASE_URL="https://gateway.tokengate.to"
export ANTHROPIC_API_KEY="tg_xxxxx"
```

### Setup: Claude Code (VS Code Extension)

```json
// settings.json
"claudeCode.environmentVariables": [
  { "name": "ANTHROPIC_BASE_URL", "value": "https://gateway.tokengate.to" },
  { "name": "ANTHROPIC_API_KEY",  "value": "tg_xxxxx" }
]
```

### Setup: OpenAI Codex

```toml
# ~/.codex/config.toml
model_provider = "tokengate"

[model_providers.tokengate]
name     = "TokenGate Proxy"
base_url = "https://gateway.tokengate.to/v1"
wire_api = "responses"
http_headers = { "X-Tokengate-Key" = "tg_xxxxx" }
```

No SDK. No code changes. Your tools don't know TokenGate is there.

---

## What you get

### Real-time visibility

- **Cost dashboard** — see total spend, tokens, and requests by day, model, provider, and API key.
- **Per-request log** — every LLM call is recorded with model, token counts, cost, and latency.
- **Trend charts** — daily cost and token trends to spot patterns and anomalies.
- **Spend forecast** — projected monthly spend based on your actual usage so far.
- **Latency metrics** — P50 / P95 / P99 latency for every model and API key.
- **Subscription vs. API comparison** — see what your "unlimited" subscription usage would cost at API rates.

### Budget enforcement

- **Spend limits** — set monthly, weekly, or daily caps. Per account, per API key, or per provider.
- **Alert thresholds** — get warned (via response headers, Slack, email, or webhook) when spend hits 80% of your limit.
- **Hard stops** — automatically block requests (HTTP 402) before you overspend. No more surprise bills.
- **Pre-request checks** — budget limits are enforced *before* the request is forwarded to the provider. You never pay for a request that exceeds your budget.

### Rate limiting

- **Requests per minute (RPM)** — prevent runaway agents from flooding the API.
- **Input / output tokens per minute (ITPM / OTPM)** — control token velocity.
- **Model-scoped limits** — set different limits for different models (e.g., unlimited Haiku, throttled Opus).
- **Per-key limits** — give each developer their own rate allocation.

### Provider key vault

- **Centralized key management** — store your Anthropic and OpenAI API keys in an encrypted vault. Developers never see or handle raw credentials.
- **AES-256-GCM encryption** — envelope encryption with per-key data encryption keys.
- **One-click rotation** — atomic key rotation in a single operation (store new key + activate + revoke old key).
- **BYOK (Bring Your Own Key)** — the gateway injects the stored key into upstream requests. Developers only get a TokenGate key.

### Audit & compliance

- **Audit logs** — immutable trail of every administrative action (key creation, team changes, budget modifications).
- **Audit reports** — generate PDF or CSV reports for any date range with customizable filters. Reports are generated async and stored securely in cloud storage.
- **Presigned downloads** — download audit reports directly via secure, time-limited URLs.
- **Cost ledger** — immutable record of every priced request with full pricing breakdown.

### Team management

- **Role-based access control** — Owner, Admin, Editor, and Viewer roles with granular permissions.
- **Invite by email** — add team members with in-app invitation notifications (accept/deny).
- **Projects** — organize API keys and team members into projects with separate budgets and permissions.
- **Notifications** — configurable alerts via email, Slack, or webhook for budget warnings and team events.

---

## Who is TokenGate for?

### Solo developers (Pro)

You use Claude Code every day. You're on the API or a Pro subscription. You want to know what you're actually spending and set guardrails so a bad prompt doesn't blow your budget.

**What you get:**
- Real-time cost tracking across Anthropic + OpenAI
- Hard budget caps — auto-block at your limit
- Daily, weekly, and monthly spend limits
- Rate limiting per model
- 90-day data history
- Slack alerts when you approach your budget
- Audit report export (CSV)

**$15/mo** (or $10/mo billed annually)

### Small teams (Team)

You have 3-10 developers sharing AI tools. You need per-developer visibility, shared budgets, and the ability to control who can do what.

**What you get:**
- Everything in Pro
- Up to 10 team members with role-based access
- Per-API-key budget limits — give each developer their own cap
- Per-key and per-model rate limits
- Project-based organization with granular permissions
- Webhook notifications for budget alerts
- Cost attribution by project
- 180-day data history

**$39/mo** (or $33/mo billed annually)

### Engineering organizations (Business)

You're running AI tools across an engineering org. You need governance, compliance, and enterprise-grade controls.

**What you get:**
- Everything in Team
- Unlimited team members, API keys, and projects
- Unlimited spend and rate limit rules
- Full audit logs with exportable reports
- 1+ year data retention
- Key rotation tracking
- SSO (Google / GitHub)
- Priority support + SLA
- Dedicated onboarding
- Advanced RBAC with fine-grained permissions

**$199/mo**

### Getting started (Free)

Just want to see what's happening? Start free. One user, one API key, basic visibility, 7-day retention. No credit card required. Upgrade when you need control.

---

## Plan comparison

| | Free | Pro | Team | Business |
|---|---|---|---|---|
| **Price** | $0 | $15/mo | $39/mo | $199/mo |
| **Users** | 1 | 1 | 10 | Unlimited |
| **API keys** | 1 | 5 | 20 | Unlimited |
| **Providers** | 1 | Multiple | Multiple | Multiple |
| **Spend limits** | 1 (alert only) | 5 (alert + block) | 20 (alert + block) | Unlimited |
| **Rate limits** | 1 | 5 | 20 | Unlimited |
| **Per-key budgets** | -- | -- | Yes | Yes |
| **Per-key rate limits** | -- | -- | Yes | Yes |
| **Projects** | 1 | 3 | 10 | Unlimited |
| **Team roles (RBAC)** | -- | -- | Yes | Yes |
| **Audit reports** | -- | CSV | CSV / PDF | CSV / PDF |
| **Notification channels** | 1 | 5 | 20 | 100 |
| **Data retention** | 7 days | 90 days | 180 days | 1+ year |

---

## Supported tools and providers

**AI coding tools:**
- Claude Code (CLI + VS Code extension)
- OpenAI Codex
- Any tool that uses the Anthropic or OpenAI API

**LLM providers:**
- Anthropic (Claude Opus, Sonnet, Haiku)
- OpenAI (GPT-4o, GPT-4.1, o3, o4-mini, Codex models)
- Google Gemini
- Azure OpenAI
- Mistral
- More coming soon

**Proxy endpoints:**

| Endpoint | What it proxies |
|---|---|
| `/v1/messages` | Anthropic Messages API (streaming + non-streaming) |
| `/v1/responses` | OpenAI Responses API (provider-aware routing) |
| `/v1/openai/*` | OpenAI API passthrough |
| `/v1/gemini/*` | Gemini API passthrough |
| `/v1/models` | Anthropic models list |

---

## Architecture

```
┌──────────────┐     ┌──────────────────────────────────┐     ┌───────────────┐
│  Dashboard   │     │         API Gateway (Go)          │     │  LLM Provider │
│  React/Vite  │────▶│                                    │────▶│  (Anthropic,  │
│  (Vercel)    │     │  Auth · Budget · Rate limit · Log  │     │   OpenAI...)  │
└──────────────┘     │  Pricing · Encryption · Streaming  │     └───────────────┘
                     └─────────────┬──────────────────────┘
                                   │
                          ┌────────┴────────┐
                          │                 │
                     ┌────▼────┐      ┌─────▼─────┐
                     │ Postgres │      │   Redis    │
                     │ (data)   │      │ (cache +   │
                     └──────────┘      │  streams)  │
                                       └────────────┘
```

- **Gateway** — Go 1.24, Gin framework. Handles proxy, auth, budget enforcement, rate limiting, and async usage processing.
- **Dashboard** — React 19, TypeScript, Vite. Hosted on Vercel. Auth via Clerk.
- **PostgreSQL** — usage logs, cost ledger, team data, provider keys, audit logs.
- **Redis** — budget counters, rate limit windows, API key cache, async event streams.

SSE streaming passes through with zero buffering. Your AI tool gets the same latency as a direct connection.

---

## Security

- **Provider keys** are encrypted at rest with AES-256-GCM envelope encryption (per-key DEK, master key in env).
- **API keys** are stored as HMAC-SHA256 hashes. The raw secret is shown once at creation and never stored.
- **Key rotation** is atomic — new key activated and old key revoked in a single database transaction.
- **Decrypted keys** are cached in-process with a 30-second sliding TTL and a 5-minute hard ceiling. Redis never stores plaintext keys.
- **Budget enforcement** happens before the upstream request — you never pay for a blocked request.
- **Auth** via Clerk (dashboard) and API key validation (gateway). Role-based access control at every endpoint.

---

## Getting started

1. **Sign up** at [tokengate.to](https://tokengate.to) — free, no credit card.
2. **Create a Gateway API Key** from the Management page.
3. **Add your provider key** (Anthropic or OpenAI) to the encrypted vault.
4. **Set two environment variables** and start using your AI tools.

```bash
export ANTHROPIC_BASE_URL="https://gateway.tokengate.to"
export ANTHROPIC_API_KEY="tg_xxxxx"

claude  # that's it — usage appears in your dashboard
```

---

## FAQ

**Does TokenGate add latency?**
Minimal. The gateway streams SSE responses with zero buffering. Budget and rate limit checks happen in-memory via Redis. Typical overhead is single-digit milliseconds.

**Does TokenGate see my code or prompts?**
No. TokenGate forwards requests to the LLM provider without inspecting or storing prompt content. Only token counts, model names, and costs are logged.

**What happens if TokenGate goes down?**
Your AI tools will receive connection errors, the same as if the LLM provider were down. TokenGate runs on redundant infrastructure to minimize downtime.

**Can I use my existing subscription (Claude Pro, ChatGPT Plus)?**
Yes. TokenGate supports both subscription passthrough (for visibility) and API-key injection (for full budget enforcement). You choose per API key.

**Do I need to change my code?**
No. TokenGate is a transparent proxy. Set environment variables and your tools route through it automatically.

**Can I self-host TokenGate?**
Not currently. TokenGate is a hosted service. Contact us if you need on-premise deployment.

---

Free plan is free forever. No credit card required.

[Start Free](https://tokengate.to/sign-up) · [View Pricing](https://tokengate.to/pricing) · [Questions? Talk to us](mailto:sales@tokengate.to)
