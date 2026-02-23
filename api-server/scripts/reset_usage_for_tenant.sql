-- Reset usage and cost data for a specific tenant.
-- Usage: psql "$POSTGRES_DB_URL" -f reset_usage_for_tenant.sql -v tenant_id=8

BEGIN;

DELETE FROM cost_ledgers WHERE tenant_id = :tenant_id;
DELETE FROM usage_logs   WHERE tenant_id = :tenant_id;

COMMIT;
