-- Initial schema for sendmemes-discord-bot.
-- Project is still in development, so the schema is consolidated into a single
-- migration. Run against an empty database; previous incremental migrations
-- are no longer supported.

CREATE TABLE IF NOT EXISTS albums (
    id              serial      PRIMARY KEY,
    name            text        NOT NULL UNIQUE,
    has_cover       boolean     NOT NULL DEFAULT false,
    cover_image_id  int,
    send_mode       text        NOT NULL DEFAULT 'Random',
    send_config_json jsonb      NOT NULL DEFAULT '{}'::jsonb,
    last_sent_at    timestamptz,
    positive_rating int         NOT NULL DEFAULT 0,
    created_at      timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE albums
    ADD CONSTRAINT albums_send_mode_check
    CHECK (send_mode IN ('Order', 'Random', 'Single', 'Custom'));

CREATE TABLE IF NOT EXISTS images (
    id          serial      PRIMARY KEY,
    url         text        NOT NULL,
    source      text,
    guild_id    text,
    album_id    int         REFERENCES albums (id),
    file_id     bigint,
    created_at  timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE albums
    ADD CONSTRAINT albums_cover_image_id_fkey
    FOREIGN KEY (cover_image_id) REFERENCES images (id) ON DELETE SET NULL;

CREATE UNIQUE INDEX IF NOT EXISTS images_file_id_idx
    ON images (file_id) WHERE file_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS albums_last_sent_at_idx
    ON albums (last_sent_at DESC NULLS LAST);

CREATE TABLE IF NOT EXISTS discord_schedule_settings (
    guild_id          text        PRIMARY KEY,
    send_channel_id   text,
    send_interval     text,
    send_history_size int,
    updated_at        timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS admin_audit_logs (
    id          bigserial   PRIMARY KEY,
    actor       text        NOT NULL,
    action      text        NOT NULL,
    target_type text        NOT NULL,
    target_id   text        NOT NULL,
    metadata    jsonb       NOT NULL DEFAULT '{}'::jsonb,
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS admin_audit_logs_created_at_idx
    ON admin_audit_logs (created_at DESC);

-- Translation history table is retained because the legacy /v1/translation
-- routes still wire it up. Remove with the rest of the translation feature
-- when the bot drops the imported template demo.
CREATE TABLE IF NOT EXISTS history (
    id          serial PRIMARY KEY,
    source      varchar(255),
    destination varchar(255),
    original    varchar(255),
    translation varchar(255)
);
