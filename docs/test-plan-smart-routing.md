# Manual Test Plan: Smart Routing Engine

**Feature date:** 2026-03-06
**Endpoint under test:** `POST /v1/chat/completions`
**Admin API:** `/v1/admin/model-groups`

---

## Prerequisites

- Running API server with database and Redis
- At least one tenant with an API key (`X-TokenGate-Key`)
- Dashboard login with `editor` or higher role (for admin endpoints)
- Provider API keys for at least 2 of: OpenAI, Anthropic, DeepSeek, Mistral
- A tool like `curl`, Postman, or `httpie` for HTTP calls

---

## 1. Model Group CRUD API

### 1.1 Create a model group — fallback strategy

```bash
curl -X POST http://localhost:PORT/v1/admin/model-groups \
  -H "Authorization: Bearer <dashboard_token>" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "test-fallback",
    "strategy": "fallback",
    "description": "Fallback test group",
    "enabled": true,
    "deployments": [
      {
        "provider": "openai",
        "model": "gpt-4o",
        "provider_key_id": <YOUR_OPENAI_KEY_ID>,
        "priority": 1,
        "weight": 1,
        "cost_per_1k_input": 0.0025,
        "cost_per_1k_output": 0.01,
        "enabled": true
      },
      {
        "provider": "anthropic",
        "model": "claude-sonnet-4-20250514",
        "provider_key_id": <YOUR_ANTHROPIC_KEY_ID>,
        "priority": 2,
        "weight": 1,
        "cost_per_1k_input": 0.003,
        "cost_per_1k_output": 0.015,
        "enabled": true
      }
    ]
  }'
```

**Expected:** 201 Created. Response includes `id`, `name`, `strategy`, and `deployments` array with generated IDs.

### 1.2 List model groups

```bash
curl http://localhost:PORT/v1/admin/model-groups \
  -H "Authorization: Bearer <dashboard_token>"
```

**Expected:** 200 OK. Array contains the group created in 1.1.

### 1.3 Update a model group

```bash
curl -X PUT http://localhost:PORT/v1/admin/model-groups/<ID> \
  -H "Authorization: Bearer <dashboard_token>" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "test-fallback",
    "strategy": "round-robin",
    "enabled": true,
    "deployments": [
      {
        "provider": "openai",
        "model": "gpt-4o-mini",
        "priority": 1,
        "weight": 3,
        "enabled": true
      },
      {
        "provider": "anthropic",
        "model": "claude-sonnet-4-20250514",
        "priority": 2,
        "weight": 1,
        "enabled": true
      }
    ]
  }'
```

**Expected:** 200 OK. Strategy changed to `round-robin`, deployments updated.

### 1.4 Delete a model group

```bash
curl -X DELETE http://localhost:PORT/v1/admin/model-groups/<ID> \
  -H "Authorization: Bearer <dashboard_token>"
```

**Expected:** 200/204. Group no longer appears in list. Sending a chat completion with this model name returns 400 `unknown model group`.

### 1.5 Validation errors

| Test | Request | Expected |
|------|---------|----------|
| Missing name | Omit `name` field | 400 error |
| Invalid strategy | `"strategy": "random"` | 400 error |
| Empty deployments | `"deployments": []` | 400 error |
| Duplicate name | Create with same `name` as existing group | 400/409 conflict |

### 1.6 RBAC — viewer role denied

Log in as a `viewer`-role user and attempt any CRUD call.

**Expected:** 403 Forbidden.

---

## 2. Health Endpoint

### 2.1 Check health for a model group

```bash
curl http://localhost:PORT/v1/admin/model-groups/<ID>/health \
  -H "Authorization: Bearer <dashboard_token>"
```

**Expected:** 200 OK with JSON containing `model_group`, `deployments[]` each with `deployment_id`, `provider`, `model`, `healthy` (bool), and `avg_latency_ms`.

### 2.2 Health after failures

Send 3+ requests that force errors on one deployment (e.g., use an invalid provider key). Then check health.

**Expected:** The failed deployment shows `healthy: false`. Other deployments remain `healthy: true`.

---

## 3. Basic Chat Completion Routing

### 3.1 Non-streaming request through model group

```bash
curl -X POST http://localhost:PORT/v1/chat/completions \
  -H "X-TokenGate-Key: <api_key>" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "test-fallback",
    "messages": [{"role": "user", "content": "Say hello in one word."}],
    "max_tokens": 10
  }'
```

**Expected:** 200 OK. Response in OpenAI format with `id`, `choices[0].message.content`, `usage` block.

### 3.2 Streaming request

```bash
curl -X POST http://localhost:PORT/v1/chat/completions \
  -H "X-TokenGate-Key: <api_key>" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "test-fallback",
    "messages": [{"role": "user", "content": "Count to 3."}],
    "stream": true,
    "stream_options": {"include_usage": true},
    "max_tokens": 50
  }'
```

**Expected:**
- Response headers: `Content-Type: text/event-stream`, `Cache-Control: no-cache`
- Body: `data: {...}\n\n` SSE chunks in OpenAI delta format
- Final chunk contains `usage` block (if `include_usage` is set)
- Ends with `data: [DONE]\n\n`

### 3.3 Unknown model group

```bash
curl -X POST http://localhost:PORT/v1/chat/completions \
  -H "X-TokenGate-Key: <api_key>" \
  -H "Content-Type: application/json" \
  -d '{"model": "nonexistent-group", "messages": [{"role": "user", "content": "hi"}]}'
```

**Expected:** 400 with `{"error": {"message": "unknown model group: nonexistent-group", "type": "invalid_request_error"}}`.

### 3.4 Missing model field

Send a request with no `model` field.

**Expected:** 400 error.

### 3.5 Disabled model group

Create a model group with `"enabled": false`, then send a chat completion using its name.

**Expected:** 400 `unknown model group` (disabled groups are not registered in the router).

---

## 4. Format Translation per Provider

For each test, create a model group with a single deployment pointing to the target provider.

### 4.1 OpenAI — basic passthrough

Create group with `provider: "openai"`, `model: "gpt-4o-mini"`. Send a chat completion.

**Expected:** Valid OpenAI-format response. Verify `model` field in response matches the OpenAI model.

### 4.2 Anthropic — system message extraction

Create group with `provider: "anthropic"`, `model: "claude-sonnet-4-20250514"`.

```json
{
  "model": "my-anthropic-group",
  "messages": [
    {"role": "system", "content": "You are a pirate."},
    {"role": "user", "content": "Say hello."}
  ],
  "max_tokens": 50
}
```

**Expected:** Response reflects the system prompt behavior (pirate-style reply). Response is in OpenAI format (not Anthropic Messages format).

### 4.3 Anthropic — tool use round-trip

```json
{
  "model": "my-anthropic-group",
  "messages": [
    {"role": "user", "content": "What's the weather in Paris?"}
  ],
  "tools": [
    {
      "type": "function",
      "function": {
        "name": "get_weather",
        "description": "Get weather for a city",
        "parameters": {
          "type": "object",
          "properties": {"city": {"type": "string"}},
          "required": ["city"]
        }
      }
    }
  ],
  "max_tokens": 200
}
```

**Expected:** Response contains `choices[0].message.tool_calls` with a function call to `get_weather`. `finish_reason` is `"tool_calls"`.

### 4.4 Anthropic — streaming format translation

Same as 4.2 but with `"stream": true`.

**Expected:** SSE chunks are in OpenAI delta format (`choices[0].delta.content`), NOT Anthropic event types. Anthropic's `content_block_delta`, `message_delta` events are translated transparently.

### 4.5 DeepSeek — reasoning content

Create group with `provider: "deepseek"`, `model: "deepseek-reasoner"`.

```json
{
  "model": "my-deepseek-group",
  "messages": [{"role": "user", "content": "What is 15 * 37?"}],
  "max_tokens": 500
}
```

**Expected (non-streaming):** Response content starts with `<think>...</think>` containing the reasoning, followed by the answer. All in OpenAI format.

**Expected (streaming):** Reasoning content arrives as regular content deltas.

### 4.6 Mistral — field stripping

Create group with `provider: "mistral"`, `model: "mistral-small-latest"`.

```json
{
  "model": "my-mistral-group",
  "messages": [{"role": "user", "content": "Hello"}],
  "frequency_penalty": 0.5,
  "presence_penalty": 0.5,
  "seed": 42,
  "max_tokens": 50
}
```

**Expected:** Request succeeds (unsupported fields `frequency_penalty`, `presence_penalty`, `seed` are stripped before forwarding). Response in OpenAI format.

### 4.7 Mistral — tool_choice translation

```json
{
  "model": "my-mistral-group",
  "messages": [{"role": "user", "content": "Get weather in London"}],
  "tools": [{"type": "function", "function": {"name": "get_weather", "description": "Get weather", "parameters": {"type": "object", "properties": {"city": {"type": "string"}}}}}],
  "tool_choice": "required",
  "max_tokens": 200
}
```

**Expected:** Request succeeds. `tool_choice: "required"` is translated to `"any"` for Mistral. Response includes a tool call.

---

## 5. Routing Strategies

### 5.1 Fallback strategy

**Setup:** Model group with strategy `fallback`, 2 deployments: OpenAI (priority 1) and Anthropic (priority 2).

**Test A — happy path:** Send a request.
**Expected:** Routed to OpenAI (priority 1). Check server logs for the selected deployment.

**Test B — primary failure:** Temporarily revoke or invalidate the OpenAI provider key. Send a request.
**Expected:** OpenAI attempt fails, automatically falls back to Anthropic. Response still succeeds. Server logs show 2 attempts.

### 5.2 Round-robin strategy

**Setup:** Model group with strategy `round-robin`, 2 deployments with equal weight.

**Test:** Send 10 requests sequentially.
**Expected:** Requests alternate between the two deployments roughly 50/50. Check server logs for distribution.

### 5.3 Weighted round-robin

**Setup:** Model group with strategy `round-robin`, deployment A weight=3, deployment B weight=1.

**Test:** Send 8 requests.
**Expected:** ~6 go to deployment A, ~2 go to deployment B (3:1 ratio). Verify in logs.

### 5.4 Lowest-latency strategy

**Setup:** Model group with strategy `lowest-latency`, 2 deployments (e.g., one fast model like `gpt-4o-mini`, one slower model).

**Test:** Send 20+ requests to build up latency data.
**Expected:** After initial exploration phase (~10% of requests go to the slower deployment), the majority route to the faster one. Check health endpoint for `avg_latency_ms` values.

### 5.5 Cost-optimized strategy

**Setup:** Model group with strategy `cost-optimized`, 2 deployments:
- Deployment A: `cost_per_1k_input: 0.001`, `cost_per_1k_output: 0.002`
- Deployment B: `cost_per_1k_input: 0.01`, `cost_per_1k_output: 0.03`

**Test:** Send 5 requests.
**Expected:** All requests route to deployment A (cheapest). Verify in logs.

---

## 6. Health Tracking and Cooldown

### 6.1 Cooldown after 3 failures

**Setup:** Model group with fallback strategy. Deployment A uses an invalid API key.

**Test:** Send 3+ requests that hit deployment A first.
**Expected:** After 3 failures on deployment A, it enters cooldown (30s). Subsequent requests skip A and go directly to the next deployment. Health endpoint shows A as `healthy: false`.

### 6.2 Recovery after cooldown

**Test:** Wait 30+ seconds after 6.1, then send a request.
**Expected:** Deployment A is retried (cooldown expired). If it still fails, cooldown escalates (60s next time).

### 6.3 Success resets health

**Test:** Fix the invalid key for deployment A, wait for cooldown to expire, send a request.
**Expected:** Once A succeeds, health is fully reset. Health endpoint shows A as `healthy: true`.

---

## 7. Retry and Backoff

### 7.1 Retryable errors trigger retry

**Setup:** Model group with 2 deployments. First deployment returns 500/503/429 errors (use an overloaded provider or invalid key that returns 500).

**Expected:** Request is retried on the second deployment. Final response succeeds. Server logs show multiple attempts.

### 7.2 Non-retryable errors stop immediately

**Test:** Send a request that triggers a 400-level error (e.g., malformed tool schema that the provider rejects as 400).

**Expected:** No retry occurs. Error returned immediately to client. Verify only 1 attempt in logs.

### 7.3 Max retries exhausted

**Setup:** Model group with 2 deployments, both with invalid keys.

**Test:** Send a request.
**Expected:** 502 response with an error message indicating all deployments were exhausted. Max attempts = min(4, number of deployments).

### 7.4 Retry-After header respected

**Test:** Trigger a 429 from a provider that returns `Retry-After` header (e.g., hit OpenAI rate limit).

**Expected:** Backoff delay matches the `Retry-After` value (capped at 8s). Check timing in logs.

---

## 8. Streaming Fallback

### 8.1 Connection-phase fallback

**Setup:** Model group with 2 deployments. First deployment's key is invalid (will fail before streaming starts).

**Test:** Send a streaming request.
**Expected:** First deployment fails at connection phase, silently falls back to second deployment. Client receives a valid SSE stream with no indication of the failover.

### 8.2 No mid-stream fallback

**Setup:** Model group with a working deployment.

**Test:** Send a streaming request. While streaming is in progress, note that if the provider drops mid-stream, the stream ends with an error (not retried).

**Expected:** Once SSE data starts flowing, there is no fallback. The stream either completes or ends with an error.

---

## 9. Auth and Billing Integration

### 9.1 Tenant auth required

```bash
curl -X POST http://localhost:PORT/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model": "test-fallback", "messages": [{"role": "user", "content": "hi"}]}'
```

**Expected:** 401 Unauthorized (no API key provided).

### 9.2 Invalid API key

```bash
curl -X POST http://localhost:PORT/v1/chat/completions \
  -H "X-TokenGate-Key: invalid-key" \
  -H "Content-Type: application/json" \
  -d '{"model": "test-fallback", "messages": [{"role": "user", "content": "hi"}]}'
```

**Expected:** 401 Unauthorized.

### 9.3 Usage event published to Redis

After a successful smart-routed request, check Redis for the usage event:

```bash
redis-cli XRANGE tokengate:usage:events - + COUNT 5
```

**Expected:** Event contains `tenant_id`, `project_id`, `key_id`, `provider`, `model`, `input_tokens`, `output_tokens`, `latency_ms`, and `api_usage_billed` fields.

### 9.4 Usage appears in dashboard

After a successful request, check the dashboard usage page.

**Expected:** The request appears in usage logs with correct provider, model, and token counts.

### 9.5 Cross-tenant isolation

Use an API key from tenant A. Create a model group under tenant B.

**Test:** Send a request with `model` set to tenant B's model group name.

**Expected:** 400 `unknown model group` — tenant A cannot access tenant B's model groups.

---

## 10. Edge Cases

### 10.1 Single deployment group

Create a model group with only 1 deployment.

**Test:** Send a request.
**Expected:** Works normally. No fallback is attempted (only 1 deployment exists).

### 10.2 All deployments disabled

Create a model group where all deployments have `"enabled": false`.

**Expected:** Request returns an error (no available deployments).

### 10.3 Large payload

Send a request with a long conversation (50+ messages, ~10k tokens).

**Expected:** Request completes successfully. Token counts in response reflect the large input.

### 10.4 Concurrent requests

Send 20 concurrent requests to a round-robin model group:

```bash
for i in $(seq 1 20); do
  curl -s -X POST http://localhost:PORT/v1/chat/completions \
    -H "X-TokenGate-Key: <key>" \
    -H "Content-Type: application/json" \
    -d '{"model": "test-rr", "messages": [{"role": "user", "content": "Say ok"}], "max_tokens": 5}' &
done
wait
```

**Expected:** All 20 requests succeed. No race conditions. Load is distributed per strategy.

### 10.5 Model group updated mid-traffic

While sending a sequence of requests, update the model group (change strategy or swap deployments).

**Expected:** New requests use the updated configuration. In-flight requests complete with the old configuration.

### 10.6 Provider key using tenant's active key (nil provider_key_id)

Create a deployment without specifying `provider_key_id`.

**Expected:** Falls back to the tenant's active provider key for that provider.

---

## Test Matrix Summary

| # | Area | Tests | Priority |
|---|------|-------|----------|
| 1 | CRUD API | 1.1–1.6 | P0 |
| 2 | Health endpoint | 2.1–2.2 | P1 |
| 3 | Basic routing | 3.1–3.5 | P0 |
| 4 | Format translation | 4.1–4.7 | P0 |
| 5 | Routing strategies | 5.1–5.5 | P0 |
| 6 | Health/cooldown | 6.1–6.3 | P1 |
| 7 | Retry/backoff | 7.1–7.4 | P1 |
| 8 | Streaming fallback | 8.1–8.2 | P1 |
| 9 | Auth/billing | 9.1–9.5 | P0 |
| 10 | Edge cases | 10.1–10.6 | P2 |
