BEGIN;
    DROP INDEX IF EXISTS idx_api_keys_status;
    DROP INDEX IF EXISTS idx_api_keys_key_hash;
    DROP TABLE IF EXISTS api_keys;
COMMIT;
