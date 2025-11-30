-- Rollback multi-tenancy migration

-- Drop triggers
DROP TRIGGER IF EXISTS update_organizations_updated_at ON organizations;
DROP TRIGGER IF EXISTS update_users_updated_at ON users;
DROP TRIGGER IF EXISTS update_organization_members_updated_at ON organization_members;
DROP TRIGGER IF EXISTS update_organization_invitations_updated_at ON organization_invitations;

-- Drop function
DROP FUNCTION IF EXISTS update_updated_at_column();

-- Drop tables (in reverse order of dependencies)
DROP TABLE IF EXISTS audit_logs;
DROP TABLE IF EXISTS organization_invitations;
DROP TABLE IF EXISTS user_sessions;
DROP TABLE IF EXISTS organization_members;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS organizations;

-- Drop enum
DROP TYPE IF EXISTS organization_role;

-- Remove columns from existing tables
ALTER TABLE jobs DROP COLUMN IF EXISTS organization_id;
ALTER TABLE jobs DROP COLUMN IF EXISTS created_by;

ALTER TABLE api_keys DROP COLUMN IF EXISTS organization_id;
ALTER TABLE api_keys DROP COLUMN IF EXISTS created_by;

ALTER TABLE gmaps_jobs DROP COLUMN IF EXISTS organization_id;
ALTER TABLE gmaps_jobs DROP COLUMN IF EXISTS created_by;
