# TokenGate Manual Test Plan

## Overview

This test plan covers end-to-end manual testing of the TokenGate gateway across all 4 plan tiers (Free, Pro, Team, Business) and all RBAC roles (Owner, Admin, Editor, Viewer). It ensures correct permission gating, plan limit enforcement, and feature availability.

---

## Part 1: New User Sign-Up — Per-Plan Experience

### Test Setup

For each plan, sign up with a fresh user via Clerk. After sign-up:
1. The auth sync (`POST /v1/auth/sync`) creates:
   - A User record (status=active)
   - A personal Tenant (plan=free by default)
   - A "Default" Project
   - TenantMembership (org_role=owner)
   - ProjectMembership (project_role=project_admin)
2. Response includes `memberships[]` with one entry
3. Upgrade to the target plan via billing if needed (Free users skip this step)

---

### 1.1 Free Plan — New User

**Precondition:** Fresh user, plan=free

#### Dashboard (`/dashboard`)
| # | Check | Expected |
|---|-------|----------|
| 1 | Page loads | Dashboard renders with summary cards |
| 2 | Summary cards | Total spend, token count, request count, success rate — all zero |
| 3 | Budget status | Empty or "No budgets configured" |
| 4 | Daily cost chart | Empty chart, no data points |
| 5 | Breakdown panels | By Model and By API Key — empty |
| 6 | Data retention banner | Shows "7 days" retention |

#### Navbar Visibility
| # | Nav Item | Visible? |
|---|----------|----------|
| 1 | Dashboard | Yes |
| 2 | Management | Yes (owner has editor+ permission) |
| 3 | Limits | Yes |
| 4 | Notifications | Yes |
| 5 | Pricing Config | Yes |
| 6 | Integration | Yes |
| 7 | Audit | Yes |
| 8 | Plans (dropdown) | Yes (owner has admin+ permission) |
| 9 | Billing (dropdown) | Yes |
| 10 | Settings (dropdown) | Yes |
| 11 | Tenant Switcher | Hidden (only 1 membership) |

#### Management Page (`/management`)
| # | Check | Expected |
|---|-------|----------|
| 1 | API Keys section | Shows "0/1 API Keys" |
| 2 | Create API Key | Form shows; project picker defaults to "Default" |
| 3 | Create 1 key | Succeeds, shows "1/1 API Keys" |
| 4 | Create 2nd key | Button disabled or error: limit reached |
| 5 | Provider Keys section | Visible (owner=admin+) |
| 6 | Provider Keys limit | Shows "0/1 Provider Keys" |
| 7 | Create 1 provider key | Succeeds |
| 8 | Create 2nd provider key | Blocked: limit reached |

#### Limits Page (`/limits`)
| # | Check | Expected |
|---|-------|----------|
| 1 | Spend Limits section | Shows "0/1" |
| 2 | Create 1 spend limit | Succeeds with scope options: account |
| 3 | Per-key budget scope | Not available (AllowPerKeyBudget=false) |
| 4 | Create 2nd spend limit | Button disabled: limit reached |
| 5 | Rate Limits section | Shows "0/1" |
| 6 | Create 1 rate limit | Succeeds |
| 7 | Create 2nd rate limit | Button disabled: limit reached |

#### Notifications Page (`/notifications`)
| # | Check | Expected |
|---|-------|----------|
| 1 | Channel limit | "0/1 Channels" |
| 2 | Create 1 channel | Succeeds (email/slack/webhook) |
| 3 | Create 2nd channel | Blocked: limit reached |

#### Audit Page (`/audit`)
| # | Check | Expected |
|---|-------|----------|
| 1 | Audit reports list | Empty |
| 2 | Export button | Hidden or disabled (AllowExport=false) |
| 3 | Audit logs tab | Visible (admin+ role) |

#### Plan Page (`/plan`)
| # | Check | Expected |
|---|-------|----------|
| 1 | Current plan | Shows "Free" highlighted |
| 2 | Usage meters | API Keys: 0/1, Provider Keys: 0/1, Members: 1/1, Projects: 1/1 |
| 3 | Upgrade buttons | Pro/Team/Business upgrade options available |

#### Settings Page (`/settings`)
| # | Check | Expected |
|---|-------|----------|
| 1 | Workspace name | Editable (owner) |
| 2 | Team members | Shows only self (1/1 members) |
| 3 | Invite button | Disabled or blocked (MaxMembers=1) |
| 4 | Projects section | Shows "Default" project only |
| 5 | Create project | Blocked (MaxProjects=1) |
| 6 | Delete default project | Blocked (undeletable) |
| 7 | Danger zone | Visible — "Delete Workspace" button |

---

### 1.2 Pro Plan — New User

**Precondition:** Fresh user, upgraded to plan=pro

#### Dashboard
| # | Check | Expected |
|---|-------|----------|
| 1 | Page loads | Same as Free but with longer data retention |
| 2 | Data retention | 90 days |
| 3 | Gateway performance metrics | Visible if proxy has been used |

#### Management Page
| # | Check | Expected |
|---|-------|----------|
| 1 | API Keys limit | "0/5 API Keys" |
| 2 | Create up to 5 keys | All succeed |
| 3 | 6th key | Blocked: limit reached |
| 4 | Provider Keys limit | "0/3 Provider Keys" |
| 5 | Create up to 3 provider keys | All succeed |
| 6 | 4th provider key | Blocked |

#### Limits Page
| # | Check | Expected |
|---|-------|----------|
| 1 | Spend limits | "0/5" max |
| 2 | Per-key budget scope | Not available (AllowPerKeyBudget=false) |
| 3 | Rate limits | "0/5" max |
| 4 | Per-key rate limit | Not available (AllowPerKeyRateLimit=false) |

#### Notifications Page
| # | Check | Expected |
|---|-------|----------|
| 1 | Channel limit | "0/5 Channels" |

#### Audit Page
| # | Check | Expected |
|---|-------|----------|
| 1 | Export button | Enabled (AllowExport=true) |
| 2 | Generate PDF/CSV | Succeeds |

#### Plan Page
| # | Check | Expected |
|---|-------|----------|
| 1 | Current plan | "Pro" highlighted |
| 2 | Usage meters | API Keys: 0/5, Provider Keys: 0/3, Members: 1/1, Projects: 0/3 |

#### Settings Page
| # | Check | Expected |
|---|-------|----------|
| 1 | Members | 1/1 — invite button disabled (MaxMembers=1) |
| 2 | Projects | Can create up to 3 total |
| 3 | Create 3rd project | Succeeds |
| 4 | Create 4th project | Blocked |

---

### 1.3 Team Plan — New User

**Precondition:** Fresh user, upgraded to plan=team

#### Dashboard
| # | Check | Expected |
|---|-------|----------|
| 1 | Data retention | 180 days |
| 2 | Full analytics suite | All metrics panels visible |

#### Management Page
| # | Check | Expected |
|---|-------|----------|
| 1 | API Keys limit | "0/50 API Keys" |
| 2 | Provider Keys limit | "0/10 Provider Keys" |
| 3 | Project filter dropdown | Visible (can filter keys by project) |

#### Limits Page
| # | Check | Expected |
|---|-------|----------|
| 1 | Spend limits | "0/20" max |
| 2 | Per-key budget scope | Available (AllowPerKeyBudget=true) |
| 3 | Rate limits | "0/20" max |
| 4 | Per-key rate limit | Available (AllowPerKeyRateLimit=true) |

#### Notifications Page
| # | Check | Expected |
|---|-------|----------|
| 1 | Channel limit | "0/20 Channels" |
| 2 | Webhook channel type | Available |

#### Settings Page
| # | Check | Expected |
|---|-------|----------|
| 1 | Members | 1/10 — invite button enabled |
| 2 | Invite user | Succeeds; new member shows as "pending" |
| 3 | Invite up to 10 total | All succeed |
| 4 | 11th invite | Blocked: member limit reached |
| 5 | Projects | Can create up to 20 total |
| 6 | Role management | Can assign viewer/editor roles to members |
| 7 | Promote to admin | Visible (owner only) |

#### Plan Page
| # | Check | Expected |
|---|-------|----------|
| 1 | Current plan | "Team" highlighted |
| 2 | Usage meters | API Keys: 0/50, Members: 1/10, Projects: 1/20 |

---

### 1.4 Business Plan — New User

**Precondition:** Fresh user, upgraded to plan=business

#### Dashboard
| # | Check | Expected |
|---|-------|----------|
| 1 | Data retention | Unlimited |
| 2 | Full analytics suite | All panels visible |

#### Management Page
| # | Check | Expected |
|---|-------|----------|
| 1 | API Keys limit | "0/200 API Keys" |
| 2 | Provider Keys limit | "0/50 Provider Keys" |

#### Limits Page
| # | Check | Expected |
|---|-------|----------|
| 1 | Spend limits | "0/100" max |
| 2 | Per-key budget scope | Available |
| 3 | Rate limits | "0/100" max |
| 4 | Per-key rate limit | Available |

#### Notifications Page
| # | Check | Expected |
|---|-------|----------|
| 1 | Channel limit | "0/100 Channels" |

#### Settings Page
| # | Check | Expected |
|---|-------|----------|
| 1 | Members | 1/Unlimited — invite always enabled |
| 2 | Projects | Unlimited — create always enabled |

#### Plan Page
| # | Check | Expected |
|---|-------|----------|
| 1 | Current plan | "Business" highlighted |
| 2 | Usage meters | Shows "Unlimited" for members and projects |

---

## Part 2: RBAC Role Visibility

Test what each role sees when accessing the same Team/Business tenant. The owner invites 3 users with viewer, editor, and admin roles.

### 2.1 Viewer Role

**Precondition:** User invited to a Team tenant with org_role=viewer

| # | Page | Accessible? | Notes |
|---|------|-------------|-------|
| 1 | Dashboard | Yes | Read-only summary data |
| 2 | Management | No | Redirected to /dashboard |
| 3 | Limits | No | Redirected to /dashboard |
| 4 | Notifications | No | Redirected to /dashboard |
| 5 | Pricing Config | No | Redirected to /dashboard |
| 6 | Integration | Yes | Public docs page |
| 7 | Audit | Yes | Can view reports but not create/export |
| 8 | Plan | No | Redirected to /dashboard |
| 9 | Billing | No | Redirected to /dashboard |
| 10 | Settings | No | Redirected to /dashboard |

**Navbar items hidden:** Management, Limits, Notifications, Pricing Config, Plans, Billing, Settings

**API-level checks:**
| # | Endpoint | Expected |
|---|----------|----------|
| 1 | GET /v1/usage | 200 OK |
| 2 | GET /v1/dashboard/config | 200 OK |
| 3 | POST /v1/admin/api_keys | 403 Forbidden |
| 4 | GET /v1/admin/api_keys | 403 Forbidden |
| 5 | POST /v1/billing/checkout | 403 Forbidden |
| 6 | GET /v1/owner/settings | 403 Forbidden |
| 7 | GET /v1/projects | 200 OK |
| 8 | POST /v1/projects | 403 Forbidden |

### 2.2 Editor Role

**Precondition:** User invited with org_role=editor

| # | Page | Accessible? | Notes |
|---|------|-------------|-------|
| 1 | Dashboard | Yes | Full read access |
| 2 | Management | Yes | Can manage API keys |
| 3 | Management — Provider Keys | No | Section hidden (needs admin+) |
| 4 | Limits | Yes | Can create/edit spend & rate limits |
| 5 | Notifications | Yes | Can manage notification channels |
| 6 | Pricing Config | Yes | Can manage pricing configs |
| 7 | Integration | Yes | |
| 8 | Audit | Yes | Can view reports, export if plan allows |
| 9 | Plan | No | Redirected to /dashboard |
| 10 | Billing | No | Redirected to /dashboard |
| 11 | Settings | No | Redirected to /dashboard |

**Navbar items hidden:** Plans, Billing, Settings

**API-level checks:**
| # | Endpoint | Expected |
|---|----------|----------|
| 1 | POST /v1/admin/api_keys | 200 OK |
| 2 | GET /v1/admin/provider_keys | 200 OK (route allows editor+) |
| 3 | POST /v1/admin/provider_keys | 200 OK (route allows editor+) |
| 4 | POST /v1/billing/checkout | 403 Forbidden |
| 5 | POST /v1/projects | 403 Forbidden (needs admin+) |
| 6 | GET /v1/projects | 200 OK |

### 2.3 Admin Role

**Precondition:** User invited with org_role=admin

| # | Page | Accessible? | Notes |
|---|------|-------------|-------|
| 1 | Dashboard | Yes | Full access |
| 2 | Management | Yes | Can manage API keys AND provider keys |
| 3 | Limits | Yes | Full access |
| 4 | Notifications | Yes | Full access |
| 5 | Pricing Config | Yes | Full access |
| 6 | Integration | Yes | |
| 7 | Audit | Yes | Can create/export reports |
| 8 | Audit Logs | Yes | Full access |
| 9 | Plan | Yes | Can view and initiate plan changes |
| 10 | Billing | Yes | Can view invoices, manage subscription |
| 11 | Settings | Yes | Can manage team members |
| 12 | Settings — Workspace Name | No | Owner-only field |
| 13 | Settings — Danger Zone | No | Owner-only section |
| 14 | Settings — Promote Admin | No | Owner-only action |

**API-level checks:**
| # | Endpoint | Expected |
|---|----------|----------|
| 1 | All /v1/admin/* | 200 OK |
| 2 | POST /v1/projects | 200 OK |
| 3 | GET /v1/audit-logs | 200 OK |
| 4 | POST /v1/billing/checkout | 200 OK |
| 5 | GET /v1/owner/settings | 403 Forbidden |
| 6 | POST /v1/owner/transfer-ownership | 403 Forbidden |

### 2.4 Owner Role

**Precondition:** The original tenant creator (org_role=owner)

| # | Page | Accessible? | Notes |
|---|------|-------------|-------|
| 1 | All pages | Yes | Full access everywhere |
| 2 | Settings — Workspace Name | Yes | Can edit |
| 3 | Settings — Danger Zone | Yes | Can delete workspace |
| 4 | Settings — Promote Admin | Yes | Can promote/demote admins |
| 5 | Settings — Transfer Ownership | Yes | Can transfer to another admin |

---

## Part 3: Multi-Tenant Experience

### 3.1 Tenant Switching

**Precondition:** User belongs to 2+ tenants with different roles

| # | Check | Expected |
|---|-------|----------|
| 1 | Tenant switcher | Visible in navbar (2+ memberships) |
| 2 | Switch tenant | Page reloads with new tenant context |
| 3 | Role changes per tenant | If owner in Tenant A, viewer in Tenant B — permissions change on switch |
| 4 | Plan limits change | Tenant A=Pro, Tenant B=Free — limits update on switch |
| 5 | Data isolation | API keys from Tenant A not visible in Tenant B |
| 6 | localStorage | `active_tenant_id` persists across page refreshes |

### 3.2 Cross-Tenant Isolation

| # | Check | Expected |
|---|-------|----------|
| 1 | API keys | Cannot see/modify keys from another tenant |
| 2 | Provider keys | Isolated per tenant |
| 3 | Projects | Cannot access projects from another tenant |
| 4 | Usage data | Only shows current tenant's usage |
| 5 | Budget limits | Scoped to current tenant |
| 6 | Members | Only shows current tenant's members |

---

## Part 4: Project Management (Team/Business)

### 4.1 Project CRUD

**Precondition:** Team or Business plan, org_role=admin+

| # | Check | Expected |
|---|-------|----------|
| 1 | Default project | Exists, named "Default", cannot be deleted |
| 2 | Create project | Succeeds with name + description |
| 3 | Duplicate name | Rejected (unique constraint per tenant) |
| 4 | Edit project name | Succeeds |
| 5 | Delete project (no keys) | Succeeds |
| 6 | Delete project (has keys) | Blocked with 409 error |
| 7 | Project limit | Enforced per plan (1/3/20/unlimited) |

### 4.2 API Key ↔ Project Binding

| # | Check | Expected |
|---|-------|----------|
| 1 | Create key without project_id | Rejected (400) |
| 2 | Create key with valid project_id | Succeeds |
| 3 | Create key with project from other tenant | Rejected |
| 4 | Filter keys by project | Shows only keys in selected project |

---

## Part 5: Plan Upgrade/Downgrade

### 5.1 Upgrade Flow (Free → Pro)

| # | Check | Expected |
|---|-------|----------|
| 1 | Click upgrade on Plan page | Redirected to Stripe checkout |
| 2 | Complete Stripe payment | Webhook received, plan updates to "pro" |
| 3 | API key limit | Changes from 1 to 5 |
| 4 | Export enabled | Audit export button becomes available |
| 5 | Data retention | Changes from 7 to 90 days |

### 5.2 Downgrade Check (Team → Pro)

| # | Check | Expected |
|---|-------|----------|
| 1 | Resources over limit | Shows blocking items (too many keys/members/projects) |
| 2 | Downgrade blocked | Cannot proceed until resources reduced |
| 3 | Resources reduced | Downgrade proceeds |
| 4 | Pending downgrade | Shows countdown to period end |
| 5 | Cancel downgrade | Reverts to current plan |

### 5.3 Payment Failure Recovery

| # | Check | Expected |
|---|-------|----------|
| 1 | invoice.payment_failed webhook | plan_status → past_due |
| 2 | Dashboard indicator | Shows past_due warning |
| 3 | invoice.paid webhook | plan_status → active |
| 4 | Dashboard indicator | Warning clears |

---

## Part 6: Gateway Proxy

### 6.1 Basic Proxy Flow

| # | Check | Expected |
|---|-------|----------|
| 1 | POST /v1/messages with valid key | Proxied to Anthropic, 200 |
| 2 | POST /v1/openai/v1/chat/completions | Proxied to OpenAI, 200 |
| 3 | Invalid API key | 401 Unauthorized |
| 4 | Revoked API key | 401 Unauthorized |
| 5 | Expired API key | 401 Unauthorized |
| 6 | Usage log created | Entry in /v1/usage after request |
| 7 | Cost calculated | Cost appears in dashboard/ledger |

### 6.2 Rate Limiting

| # | Check | Expected |
|---|-------|----------|
| 1 | Within rate limit | Requests succeed |
| 2 | Exceed RPM | 429 Too Many Requests |
| 3 | Rate limit notification | Notification sent if configured |

### 6.3 Budget Enforcement

| # | Check | Expected |
|---|-------|----------|
| 1 | Within budget | Requests succeed |
| 2 | Exceed budget (warn action) | Notification sent, requests continue |
| 3 | Exceed budget (block action) | 402 Payment Required |

---

## Part 7: Quick Reference — Plan Limits

| Feature | Free | Pro | Team | Business |
|---------|------|-----|------|----------|
| API Keys | 1 | 5 | 50 | 200 |
| Provider Keys | 1 | 3 | 10 | 50 |
| Members | 1 | 1 | 10 | Unlimited |
| Projects | 1 | 3 | 20 | Unlimited |
| Spend Limits | 1 | 5 | 20 | 100 |
| Rate Limits | 1 | 5 | 20 | 100 |
| Notification Channels | 1 | 5 | 20 | 100 |
| Per-Key Budget | No | No | Yes | Yes |
| Per-Key Rate Limit | No | No | Yes | Yes |
| Data Retention | 7 days | 90 days | 180 days | Unlimited |
| Audit Export | No | Yes | Yes | Yes |
| Block Action | Yes | Yes | Yes | Yes |

## Part 8: Quick Reference — Role Permissions

| Page/Feature | Viewer | Editor | Admin | Owner |
|-------------|--------|--------|-------|-------|
| Dashboard (read) | Yes | Yes | Yes | Yes |
| Usage/Forecast (read) | Yes | Yes | Yes | Yes |
| Management (API keys) | No | Yes | Yes | Yes |
| Management (Provider keys) | No | No* | Yes | Yes |
| Limits (spend/rate) | No | Yes | Yes | Yes |
| Notifications | No | Yes | Yes | Yes |
| Pricing Config | No | Yes | Yes | Yes |
| Integration docs | Yes | Yes | Yes | Yes |
| Audit (view reports) | Yes | Yes | Yes | Yes |
| Audit (create/export) | No | No | Yes | Yes |
| Audit Logs | No | No | Yes | Yes |
| Plan page | No | No | Yes | Yes |
| Billing page | No | No | Yes | Yes |
| Settings (team) | No | No | Yes | Yes |
| Settings (workspace name) | No | No | No | Yes |
| Settings (danger zone) | No | No | No | Yes |
| Promote/Demote admin | No | No | No | Yes |
| Transfer ownership | No | No | No | Yes |
| Projects (read) | Yes | Yes | Yes | Yes |
| Projects (create/update/delete) | No | No | Yes | Yes |
| Project members (manage) | No | No | Yes | Yes |

*Note: Backend routes allow editor+ for provider keys, but frontend hides the section for non-admin roles.
