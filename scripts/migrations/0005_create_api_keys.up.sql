BEGIN;
    CREATE TABLE IF NOT EXISTS api_keys(
        id UUID PRIMARY KEY,
        name TEXT NOT NULL,
        key_hash TEXT NOT NULL UNIQUE,
        status TEXT NOT NULL,
        created_at TIMESTAMP WITH TIME ZONE NOT NULL,
        updated_at TIMESTAMP WITH TIME ZONE NOT NULL,
        last_used_at TIMESTAMP WITH TIME ZONE,
        expires_at TIMESTAMP WITH TIME ZONE
    );

    -- Create index on key_hash for faster lookups
    CREATE INDEX idx_api_keys_key_hash ON api_keys(key_hash);

    -- Create index on status for faster filtering
    CREATE INDEX idx_api_keys_status ON api_keys(status);

COMMIT;
