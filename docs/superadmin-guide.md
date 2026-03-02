# Super Admin Guide

## Setup

### 1. Configure the allowlist

Add a comma-separated list of admin emails to your environment:

```bash
# .env or Railway / hosting provider
SUPER_ADMIN_EMAILS=alice@company.com,bob@company.com
```

These must match the email addresses users signed up with in Clerk. The check is case-insensitive.

### 2. Restart the API server

The env var is read once at startup. After changing it, restart the server for changes to take effect.

### 3. Access the UI

1. Sign in to the dashboard with an email on the allowlist.
2. Click your avatar in the top-right corner.
3. Select **Super Admin** from the dropdown menu.
4. Or navigate directly to `/superadmin`.

Non-admin users will be silently redirected to `/dashboard`.

---

## Features

### Platform Overview

Four stat cards at the top of the page:

| Card | What it shows |
|------|---------------|
| **Total Tenants** | Count with breakdown by plan (free / pro / team / business) |
| **Active Users** | All users with `active` status |
| **Active API Keys** | Non-revoked keys across all tenants |
| **30-Day Usage** | Request count and total cost over the last 30 days |

### Tenant Management

A searchable, filterable, paginated table of every tenant on the platform.

**Filters:**
- **Search** — matches tenant name or billing email (case-insensitive)
- **Plan** — filter by free, pro, team, or business
- **Status** — filter by active or suspended

**Table columns:** ID, Name, Plan, Status, Members, API Keys, Created, Actions

**Actions per tenant:**
- **View** — opens a detail modal
- **Change Plan** — opens the plan change modal

### Tenant Detail Modal

Shows everything about a single tenant:

- **Tenant Info** — ID, plan, status, billing email, Stripe customer/subscription IDs, pending plan changes, creation date
- **Quick Stats** — API keys, provider keys, projects, 30-day usage and cost
- **Members** — table of all members with email, name, role, and status
- **Actions** — Change Plan, Suspend / Unsuspend buttons

### Change Plan

1. Click **Change Plan** on a tenant (from the table or detail modal).
2. Select the target plan (free / pro / team / business).
3. Click **Confirm**.

If the tenant exceeds the target plan's limits (e.g., too many API keys for a downgrade), the error message will explain exactly what needs to be reduced first.

### Suspend / Unsuspend

Click the **Suspend** or **Unsuspend** button in the tenant detail modal. Suspended tenants cannot use the platform until unsuspended.

---

## API Reference

All endpoints require the `X-User-ID` header (set automatically by the dashboard). No `X-Tenant-Id` is needed.

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/v1/superadmin/whoami` | Check if current user is a super admin |
| `GET` | `/v1/superadmin/stats` | Platform-wide metrics |
| `GET` | `/v1/superadmin/tenants` | List tenants (supports `?search=`, `?plan=`, `?status=`, `?page=`, `?per_page=`) |
| `GET` | `/v1/superadmin/tenants/:id` | Full tenant detail with members and stats |
| `PATCH` | `/v1/superadmin/tenants/:id/plan` | Change plan — body: `{ "plan": "business" }` |
| `PATCH` | `/v1/superadmin/tenants/:id/status` | Suspend/unsuspend — body: `{ "status": "suspended" }` |

### Example: change a tenant to business via curl

```bash
curl -X PATCH https://api.yourapp.com/v1/superadmin/tenants/42/plan \
  -H "Content-Type: application/json" \
  -H "X-User-ID: user_2lXYZ..." \
  -d '{"plan": "business"}'
```

---

## Security Notes

- Access is controlled by email allowlist only — there is no role or permission in the database. To revoke access, remove the email from `SUPER_ADMIN_EMAILS` and restart.
- The navbar caches the admin check in `sessionStorage`. If you remove someone's access, they will lose the link on their next browser session (or if they clear session storage).
- All denied access attempts are logged server-side at `WARN` level with the user ID and email.
