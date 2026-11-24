-- v0.2: tags, tag rules, webhooks, background jobs, per-inbox settings, spam results, link cache, relay audit.

-- Per-inbox settings.
ALTER TABLE inboxes
    -- SMTP fault injection: {"auth"|"mail"|"rcpt"|"data": {"code": 451, "probability": 30, "message": "..."}}
    ADD COLUMN chaos              JSONB   NOT NULL DEFAULT '{}',
    -- Run the configured spam engine (SpamAssassin/Rspamd) on every received message.
    ADD COLUMN spam_check         BOOLEAN NOT NULL DEFAULT false,
    -- Private hosts/CIDRs that outbound requests (webhooks, link check) may reach for this inbox.
    ADD COLUMN allowed_hosts      TEXT[]  NOT NULL DEFAULT '{}',
    -- Domains links are expected to point to (link check flags anything else).
    ADD COLUMN link_domains       TEXT[]  NOT NULL DEFAULT '{}',
    -- Regex every RCPT TO must match ('' = accept all).
    ADD COLUMN allowed_recipients TEXT    NOT NULL DEFAULT '',
    -- Regex a release recipient must match ('' = releasing is disabled for this inbox).
    ADD COLUMN relay_recipients   TEXT    NOT NULL DEFAULT '';

ALTER TABLE messages
    ADD COLUMN spam_score  REAL,
    ADD COLUMN spam_report JSONB;
CREATE INDEX messages_size_idx ON messages (size);

CREATE TABLE message_tags (
    message_id UUID NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    tag        TEXT NOT NULL CHECK (length(tag) BETWEEN 1 AND 100),
    PRIMARY KEY (message_id, tag)
);
CREATE INDEX message_tags_tag_idx ON message_tags (lower(tag));

-- Auto-tagging: messages matching `query` (search syntax) get `tag`. inbox_id NULL = every inbox.
CREATE TABLE tag_rules (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    inbox_id   UUID REFERENCES inboxes(id) ON DELETE CASCADE,
    query      TEXT NOT NULL,
    tag        TEXT NOT NULL CHECK (length(tag) BETWEEN 1 AND 100),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX tag_rules_inbox_idx ON tag_rules (inbox_id);

CREATE TABLE webhooks (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    inbox_id         UUID NOT NULL REFERENCES inboxes(id) ON DELETE CASCADE,
    url              TEXT NOT NULL,
    secret           TEXT NOT NULL,
    events           TEXT[] NOT NULL DEFAULT '{message.received}',
    active           BOOLEAN NOT NULL DEFAULT true,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_status      INT,
    last_delivery_at TIMESTAMPTZ
);
CREATE INDEX webhooks_inbox_idx ON webhooks (inbox_id);

CREATE TABLE webhook_deliveries (
    id          BIGSERIAL PRIMARY KEY,
    webhook_id  UUID NOT NULL REFERENCES webhooks(id) ON DELETE CASCADE,
    event       TEXT NOT NULL,
    attempt     INT NOT NULL,
    status_code INT NOT NULL DEFAULT 0,
    error       TEXT NOT NULL DEFAULT '',
    duration_ms INT NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX webhook_deliveries_hook_idx ON webhook_deliveries (webhook_id, created_at DESC);

-- Durable background work, claimed with FOR UPDATE SKIP LOCKED so several replicas can share it.
CREATE TABLE jobs (
    id           BIGSERIAL PRIMARY KEY,
    kind         TEXT NOT NULL,
    payload      JSONB NOT NULL DEFAULT '{}',
    run_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    attempts     INT NOT NULL DEFAULT 0,
    max_attempts INT NOT NULL DEFAULT 5,
    locked_until TIMESTAMPTZ,
    failed       BOOLEAN NOT NULL DEFAULT false,
    last_error   TEXT NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX jobs_ready_idx ON jobs (run_at) WHERE NOT failed;

CREATE TABLE link_cache (
    url        TEXT PRIMARY KEY,
    status     INT NOT NULL,
    error      TEXT NOT NULL DEFAULT '',
    checked_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Audit trail of messages released to a real SMTP server. Kept even if the message is deleted.
CREATE TABLE relay_log (
    id         BIGSERIAL PRIMARY KEY,
    message_id UUID NOT NULL,
    inbox_id   UUID,
    user_email TEXT NOT NULL,
    recipients TEXT[] NOT NULL,
    subject    TEXT NOT NULL DEFAULT '',
    ok         BOOLEAN NOT NULL,
    error      TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX relay_log_inbox_idx ON relay_log (inbox_id, created_at DESC);
