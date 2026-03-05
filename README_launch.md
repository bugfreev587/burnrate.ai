Product Hunt / Hacker News Launch README

# TokenGate

### Control and track LLM usage for AI agents before your bill explodes.

TokenGate is a **gateway for LLM APIs and AI agents** that helps developers and teams:

• track token usage  
• enforce budgets  
• secure API keys  
• monitor AI cost in real time  

Instead of letting every AI agent call LLM APIs directly, TokenGate acts as a **control layer between your agents and LLM providers**.

Think of it as:

> **API Gateway for AI agents**

---

# The Problem

AI agents and coding assistants are everywhere now:

- Claude Code
- OpenAI Codex
- internal AI tools
- automation agents

But teams quickly lose visibility into:

- which models are used
- who is generating tokens
- how much each agent costs
- why the bill suddenly doubled

Most companies only notice when **the monthly invoice arrives**.

---

# The Solution

TokenGate sits between your agents and LLM providers.

AI Agent
↓
TokenGate Gateway
↓
LLM Provider (Anthropic / OpenAI / Gemini)

Every request is:

1. authenticated
2. budget-checked
3. forwarded to provider
4. usage recorded
5. cost computed

All without changing your code.

---

# Features

### LLM API Gateway

Drop-in proxy for major providers:

- Anthropic
- OpenAI
- Gemini
- Bedrock
- Vertex

Streaming supported.

---

### Usage Tracking

Track:

- token usage
- model usage
- requests
- cost

Across your entire workspace.

---

### Budget Enforcement

Stop runaway agents.

Example:

$100/month Anthropic
$50/month OpenAI

When the budget is exceeded:

HTTP 402 Budget Exceeded

---

### API Key Vault

Secure provider keys with encryption.

Developers never see raw provider keys.

---

### Rate Limits

Protect infrastructure with:

- RPM
- input tokens per minute
- output tokens per minute

---

### Team Management

Invite team members and assign roles:

- owner
- admin
- editor
- viewer

---

# Example: Claude Code

Set the gateway as your API base URL.

export ANTHROPIC_BASE_URL=“https://gateway.tokengate.to”
export ANTHROPIC_API_KEY=“tg_xxx:secret”

Then run:

claude

All requests automatically go through TokenGate.

---

# Why We Built This

AI agents are becoming the **runtime of modern software**.

But today there is no infrastructure for:

- AI cost governance
- agent usage monitoring
- API key sharing
- guardrails for LLM usage

TokenGate provides that missing layer.

---

# Who This Is For

### AI Developers

Monitor your own usage while building with LLM APIs.

---

### AI Startups

Track usage across developers and projects.

---

### Companies Using AI Internally

Add governance and budget control to AI tools.

---

# Architecture

Backend

Go + Gin
PostgreSQL
Redis

Frontend

React
Vercel

Infrastructure

Railway
Cloudflare R2
Stripe
Clerk

---

# Get Started

Create a workspace:

https://tokengate.to

Generate a gateway API key and connect your AI agents.

---

# Roadmap

Coming soon:

- runaway agent detection
- model allowlists
- cost efficiency insights
- session kill switch
- per-model budgets

---

# License

MIT