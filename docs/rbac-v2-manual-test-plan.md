# RBAC v2 Manual Click Test Plan

This version is written for manual QA execution.
Each test case includes:
- Exact UI actions (what to click)
- Expected UI result (what you should see)
- Expected backend change (API/DB)

## Test Environment
- Dashboard URL: `http://localhost:5173` (or your staging URL)
- API URL: `http://localhost:8080`
- Test users (already created):
  - `owner_a` (owner in Tenant A)
  - `admin_a` (admin in Tenant A)
  - `editor_a` (editor in Tenant A)
  - `viewer_a` (viewer in Tenant A)
  - `owner_b` (owner in Tenant B)
- Tenant A has projects:
  - `Default`
  - `Project Alpha`
  - `Project Beta`

## Quick Role Matrix (for expected access)
- `viewer`: Dashboard, Audit, Integration, read-only usage/billing status
- `editor`: viewer + Management/Limits/Notifications/Pricing Config
- `admin`: editor + provider key management + billing mutations
- `owner`: admin + Settings owner actions (promote/demote/transfer/delete account)

---

## TC-01: Tenant Switcher updates scope
Precondition:
- Login user belongs to 2+ tenants.

Steps:
1. Sign in to dashboard.
2. In top navbar, click the tenant dropdown (shown as `Tenant Name (role)`).
3. Select another tenant.
4. Wait for auto refresh/reload.
5. Open browser DevTools -> Network.
6. Refresh page and inspect any `/v1/...` API request headers.

Expected UI:
- Navbar now shows the newly selected tenant.
- Data on pages (Dashboard/Management/Settings) reflects the selected tenant.

Expected backend:
- Requests include `X-Tenant-Id: <selected_tenant_id>`.
- Returned data belongs only to that tenant.

---

## TC-02: Viewer cannot access Management page actions
Precondition:
- Sign in as `viewer_a` in Tenant A.

Steps:
1. In navbar, confirm `Management` link is hidden.
2. Manually open `/management` in browser URL bar.

Expected UI:
- You are redirected to `/dashboard`.
- No create/revoke key actions visible.

Expected backend:
- `GET /v1/admin/api_keys` would return `403` if called.
- No DB changes.

---

## TC-03: Editor can access Management but not owner-only actions
Precondition:
- Sign in as `editor_a` in Tenant A.

Steps:
1. Click `Management` in navbar.
2. Verify page loads.
3. Open avatar menu (top-right).
4. Check if `Settings`/`Plan`/`Billing` entries appear.

Expected UI:
- Management page is accessible.
- Owner/admin-only menu items are not shown for editor.

Expected backend:
- `GET /v1/admin/api_keys` returns `200`.
- `GET /v1/owner/settings` returns `403` for editor.

---

## TC-04: Owner can access owner settings
Precondition:
- Sign in as `owner_a` in Tenant A.

Steps:
1. Open avatar menu.
2. Click `Settings`.
3. In DevTools Network, inspect `GET /v1/owner/settings`.

Expected UI:
- Settings page loads owner controls (team/user/project controls).

Expected backend:
- `GET /v1/owner/settings` returns `200`.
- Response contains tenant name/plan/limits.

---

## TC-05: Create Project (admin+) and verify membership auto-add
Precondition:
- Sign in as `admin_a` (or owner) in Tenant A.

Steps:
1. Go to `Settings` page.
2. In Projects section, click `Create Project`.
3. Enter `Project Gamma` (description optional).
4. Click confirm/create.
5. In project row, click `Members` (or equivalent view members action).

Expected UI:
- New project `Project Gamma` appears in list.
- Your user is shown as project member (project admin).

Expected backend:
- `POST /v1/projects` returns `201`.
- New row in `projects` with `tenant_id = Tenant A` and `name='Project Gamma'`.
- New row in `project_memberships` for creator with `project_role='project_admin'`.

---

## TC-06: Prevent duplicate project name in same tenant
Precondition:
- `Project Alpha` already exists in Tenant A.

Steps:
1. In `Settings` page Projects section, click `Create Project`.
2. Enter `Project Alpha`.
3. Submit.

Expected UI:
- Error message shown (duplicate/conflict).
- No new project row shown.

Expected backend:
- `POST /v1/projects` returns `409`.
- No insert in `projects`.

---

## TC-07: Default project cannot be deleted
Precondition:
- Signed in as admin/owner in Tenant A.

Steps:
1. Go to `Settings` -> Projects.
2. Locate project marked `Default`.
3. Click `Delete` for that project.

Expected UI:
- Error message: default project cannot be deleted.
- `Default` project remains.

Expected backend:
- `DELETE /v1/projects/:id` returns `409` with `cannot_delete_default`.
- No status update on the default project.

---

## TC-08: Delete project blocked when active API key exists
Precondition:
- Signed in as admin/owner in Tenant A.
- At least one active API key bound to `Project Alpha`.

Steps:
1. Go to `Management`.
2. Click `Create Key`.
3. Select project `Project Alpha`, complete key creation.
4. Go to `Settings` -> Projects.
5. Attempt to delete `Project Alpha`.

Expected UI:
- Delete fails with message indicating active keys exist.

Expected backend:
- `DELETE /v1/projects/:id` returns `409` with `has_active_keys`.
- `projects.status` unchanged (still active).

---

## TC-09: API Key creation requires project selection
Precondition:
- Signed in as editor/admin/owner with Management access.

Steps:
1. Go to `Management`.
2. Click `Create Key`.
3. Fill label/provider/auth/billing but leave `Project` as empty.
4. Click `Create`.

Expected UI:
- Inline error: `Please select a project`.
- Key is not created.

Expected backend:
- No `POST /v1/admin/api_keys` should fire from UI; if manually fired without `project_id`, API returns `400`.
- No insert into `api_keys`.

---

## TC-10: API Key list and filter by project
Precondition:
- Signed in with Management access.
- Have keys in at least two projects.

Steps:
1. Go to `Management`.
2. In Gateway API Keys section, open project filter dropdown.
3. Select `Project Alpha`.
4. Observe table.
5. Switch filter back to `All Projects`.

Expected UI:
- Only keys for selected project appear when filtered.
- `Project` column shows correct project name for each key.

Expected backend:
- Data comes from `GET /v1/admin/api_keys` plus local project mapping.
- No DB writes.

---

## TC-11: Add project member and update project role
Precondition:
- Signed in as admin/owner.
- Target user is already tenant member.

Steps:
1. Go to `Settings` -> Projects.
2. Open members for `Project Beta`.
3. Click `Add Member`.
4. Select user (e.g., `viewer_a`) and role `project_viewer`, submit.
5. Change same user role to `project_editor`.

Expected UI:
- User appears in member list after add.
- Role badge updates after role change.

Expected backend:
- `POST /v1/projects/:id/members` returns `201`, inserts into `project_memberships`.
- `PATCH /v1/projects/:id/members/:user_id` returns `200`, updates `project_role`.

---

## TC-12: Remove project member
Precondition:
- Member exists in selected project.

Steps:
1. In `Settings` -> project members list.
2. Click `Remove` for a project member.
3. Confirm removal.

Expected UI:
- Member disappears from list.

Expected backend:
- `DELETE /v1/projects/:id/members/:user_id` returns `200`.
- Row removed from `project_memberships`.

---

## TC-13: Invite member with viewer/editor role
Precondition:
- Signed in as admin/owner in Tenant A.

Steps:
1. Go to `Settings`.
2. In Team Members section, click `Invite Member`.
3. Enter email, set role to `viewer`, click `Send Invite`.
4. Repeat with role `editor`.

Expected UI:
- Success message appears for each invite.
- Invited user appears with pending/expected status.

Expected backend:
- `POST /v1/admin/users/invite` returns `201`.
- Insert/update in `users` (pending placeholder if new email).
- Insert in `tenant_memberships` with `status='pending'` and chosen `org_role`.

---

## TC-14: Invite with admin role is rejected
Precondition:
- Signed in as admin/owner.

Steps:
1. Call invite API manually (Postman/curl) with payload role=`admin`.

Example:
```bash
curl -i -X POST http://localhost:8080/v1/admin/users/invite \
  -H 'Content-Type: application/json' \
  -H 'X-User-ID: <admin_or_owner_user_id>' \
  -H 'X-Tenant-Id: <tenant_a_id>' \
  -d '{"email":"badrole@test.com","role":"admin"}'
```

Expected UI/API:
- Request fails with `400 invalid_role`.

Expected backend:
- No membership created.

---

## TC-15: Suspend and unsuspend membership
Precondition:
- Signed in as admin/owner.
- Target user is not owner.

Steps:
1. In `Settings` -> Team Members, click `Suspend` for `viewer_a`.
2. In another browser/session as `viewer_a`, refresh dashboard.
3. Attempt to load protected pages.
4. Back as admin/owner, click `Unsuspend`.
5. Retry as `viewer_a`.

Expected UI:
- Suspended user loses access to protected tenant data.
- After unsuspend, access restored.

Expected backend:
- `PATCH /v1/admin/users/:id/suspend` sets `tenant_memberships.status='suspended'`.
- `PATCH /v1/admin/users/:id/unsuspend` sets status back to `active`.

---

## TC-16: Cross-tenant isolation via tenant switch
Precondition:
- One user has memberships in Tenant A and Tenant B.
- Tenants have different project/key names.

Steps:
1. Select Tenant A in navbar.
2. Go to `Management` and note keys/projects shown.
3. Switch to Tenant B.
4. Go to `Management` and compare.
5. Repeat in `Settings` and `Dashboard`.

Expected UI:
- Tenant A data disappears when Tenant B is selected and vice versa.
- No mixed data across tenants.

Expected backend:
- Every dashboard API request carries selected `X-Tenant-Id`.
- Queries are scoped to that tenant in results.

---

## TC-17: Auth sync returns memberships (multi-tenant)
Precondition:
- User belongs to multiple tenants.

Steps:
1. Sign out.
2. Sign in again.
3. In DevTools Network, open `/v1/auth/sync` response.

Expected UI:
- Tenant switcher appears in navbar (if 2+ memberships).

Expected backend:
- Response JSON contains `memberships` array with `tenant_id`, `tenant_name`, `org_role`.

---

## TC-18: Migration smoke check (upgrade path)
Precondition:
- DB snapshot from pre-RBAC-v2 version.

Steps:
1. Deploy/run new API build.
2. Watch startup logs.
3. Login as existing legacy user and open dashboard.
4. Verify keys/projects pages load.

Expected UI:
- Existing users can log in normally.
- Data is accessible, no blank-state regression caused by migration.

Expected backend:
- Logs show RBAC migration completed.
- `tenant_memberships`, `projects`, `project_memberships` populated.
- Existing `api_keys.project_id` backfilled (non-null/non-zero).

---

## Suggested Run Order
1. TC-18 (migration smoke)
2. TC-17, TC-01 (auth + tenant context)
3. TC-02/03/04 (role gates)
4. TC-05..12 (projects + API keys + project memberships)
5. TC-13/14/15 (team membership operations)
6. TC-16 (isolation final pass)
