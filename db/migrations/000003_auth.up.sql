-- Authentication. Tokens are high-entropy random strings; only their SHA-256 digests are stored.
CREATE TABLE app_user (id uuid PRIMARY KEY DEFAULT gen_random_uuid(), email text NOT NULL CHECK (email = lower(email) AND length(email) BETWEEN 3 AND 254),
 password_hash text NOT NULL, role text NOT NULL CHECK (role IN ('user','admin')), disabled boolean NOT NULL DEFAULT false,
 created_at timestamptz NOT NULL DEFAULT now(), UNIQUE(email));
-- One row per issued access/refresh pair. Refreshing marks the row rotated and inserts a successor in
-- the same family; presenting a rotated refresh token again revokes the whole family (theft detection).
CREATE TABLE auth_session (id uuid PRIMARY KEY DEFAULT gen_random_uuid(), user_id uuid NOT NULL REFERENCES app_user(id) ON DELETE CASCADE,
 family_id uuid NOT NULL, access_hash bytea NOT NULL UNIQUE, refresh_hash bytea NOT NULL UNIQUE,
 access_expires_at timestamptz NOT NULL, refresh_expires_at timestamptz NOT NULL,
 rotated_at timestamptz, revoked_at timestamptz, user_agent text NOT NULL DEFAULT '', created_at timestamptz NOT NULL DEFAULT now());
CREATE INDEX auth_session_family_idx ON auth_session(family_id);
CREATE INDEX auth_session_user_idx ON auth_session(user_id);
-- Machine credentials (ingestion pipelines). scope read: GET and calculators; ingest: also writes.
CREATE TABLE api_key (id uuid PRIMARY KEY DEFAULT gen_random_uuid(), user_id uuid NOT NULL REFERENCES app_user(id) ON DELETE CASCADE,
 name text NOT NULL CHECK (length(name) BETWEEN 1 AND 100), scope text NOT NULL CHECK (scope IN ('read','ingest')),
 key_hash bytea NOT NULL UNIQUE, created_at timestamptz NOT NULL DEFAULT now(), last_used_at timestamptz, revoked_at timestamptz);
