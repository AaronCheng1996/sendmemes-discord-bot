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
    CHECK (send_mode IN ('Order', 'Random', 'Single', 'Video', 'Custom'));

CREATE TABLE IF NOT EXISTS images (
    id          serial      PRIMARY KEY,
    url         text        NOT NULL,
    source      text,
    guild_id    text,
    album_id    int         REFERENCES albums (id),
    file_id     bigint,
    kind        text        NOT NULL DEFAULT 'image' CHECK (kind IN ('image', 'video')),
    size_bytes  bigint,
    created_at  timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE albums
    ADD CONSTRAINT albums_cover_image_id_fkey
    FOREIGN KEY (cover_image_id) REFERENCES images (id) ON DELETE SET NULL;

CREATE UNIQUE INDEX IF NOT EXISTS images_file_id_idx
    ON images (file_id) WHERE file_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS albums_last_sent_at_idx
    ON albums (last_sent_at DESC NULLS LAST);

-- Delivery rules drive both scheduled album sends and "new content" posts.
-- trigger_type: 'scheduled' fires every interval; 'new_album' / 'new_files'
-- fire when a pCloud sync discovers a new album / new files in an album.
-- interval and history_size only apply to 'scheduled' rules.
CREATE TABLE IF NOT EXISTS delivery_rules (
    id           bigserial   PRIMARY KEY,
    name         text        NOT NULL DEFAULT '',
    guild_id     text        NOT NULL DEFAULT '',
    trigger_type text        NOT NULL CHECK (trigger_type IN ('new_album', 'new_files', 'scheduled')),
    channel_id   text        NOT NULL,
    send_interval text,
    history_size int         NOT NULL DEFAULT 10,
    enabled      boolean     NOT NULL DEFAULT true,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS delivery_rules_trigger_enabled_idx
    ON delivery_rules (trigger_type, enabled);

-- Singleton table (id can only be true) for global runtime settings.
CREATE TABLE IF NOT EXISTS app_settings (
    id            boolean     PRIMARY KEY DEFAULT true CHECK (id),
    sync_interval text,
    updated_at    timestamptz NOT NULL DEFAULT now()
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

-- Per-album discovery events recorded by pCloud sync runs; surfaced in the
-- admin UI Activity page and used for Discord "new content" notifications.
CREATE TABLE IF NOT EXISTS sync_events (
    id          bigserial   PRIMARY KEY,
    event_type  text        NOT NULL CHECK (event_type IN ('album_created', 'files_added')),
    album_id    int         REFERENCES albums (id) ON DELETE SET NULL,
    album_name  text        NOT NULL,
    new_images  int         NOT NULL DEFAULT 0,
    new_videos  int         NOT NULL DEFAULT 0,
    file_names  jsonb       NOT NULL DEFAULT '[]'::jsonb,
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS sync_events_created_at_idx
    ON sync_events (created_at DESC);

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
