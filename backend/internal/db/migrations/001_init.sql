CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE TABLE users (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email         TEXT NOT NULL UNIQUE,
    name          TEXT NOT NULL DEFAULT '',
    password_hash TEXT NOT NULL,
    role          TEXT NOT NULL DEFAULT 'member' CHECK (role IN ('admin', 'member')),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_login_at TIMESTAMPTZ
);

CREATE TABLE sessions (
    token_hash TEXT PRIMARY KEY,
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX sessions_expires_idx ON sessions (expires_at);

-- An inbox is the tenant unit: it owns SMTP credentials, an API token and its messages.
CREATE TABLE inboxes (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name            TEXT NOT NULL,
    slug            TEXT NOT NULL UNIQUE,
    description     TEXT NOT NULL DEFAULT '',
    smtp_username   TEXT NOT NULL UNIQUE,
    smtp_password   TEXT NOT NULL,
    api_token       TEXT NOT NULL UNIQUE,
    retention_hours INT CHECK (retention_hours IS NULL OR retention_hours > 0),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE inbox_members (
    inbox_id UUID NOT NULL REFERENCES inboxes(id) ON DELETE CASCADE,
    user_id  UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    PRIMARY KEY (inbox_id, user_id)
);

CREATE TABLE messages (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    inbox_id         UUID NOT NULL REFERENCES inboxes(id) ON DELETE CASCADE,
    message_id       TEXT NOT NULL DEFAULT '',
    envelope_from    TEXT NOT NULL DEFAULT '',
    envelope_to      TEXT[] NOT NULL DEFAULT '{}',
    from_header      TEXT NOT NULL DEFAULT '',
    to_header        TEXT NOT NULL DEFAULT '',
    cc_header        TEXT NOT NULL DEFAULT '',
    subject          TEXT NOT NULL DEFAULT '',
    text_body        TEXT NOT NULL DEFAULT '',
    html_body        TEXT NOT NULL DEFAULT '',
    headers          JSONB NOT NULL DEFAULT '{}',
    raw              BYTEA NOT NULL,
    size             INT NOT NULL,
    attachment_count INT NOT NULL DEFAULT 0,
    is_read          BOOLEAN NOT NULL DEFAULT false,
    client_ip        TEXT NOT NULL DEFAULT '',
    smtp_user        TEXT NOT NULL DEFAULT '',
    received_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- Denormalized lowercase string used by the free-text filter.
    search_text      TEXT NOT NULL DEFAULT ''
);
CREATE INDEX messages_inbox_received_idx ON messages (inbox_id, received_at DESC);
CREATE INDEX messages_received_idx ON messages (received_at);
CREATE INDEX messages_search_trgm_idx ON messages USING gin (search_text gin_trgm_ops);

CREATE TABLE attachments (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    message_id   UUID NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    filename     TEXT NOT NULL DEFAULT '',
    content_type TEXT NOT NULL DEFAULT 'application/octet-stream',
    content_id   TEXT NOT NULL DEFAULT '',
    inline       BOOLEAN NOT NULL DEFAULT false,
    size         INT NOT NULL,
    data         BYTEA NOT NULL
);
CREATE INDEX attachments_message_idx ON attachments (message_id);
