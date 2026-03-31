CREATE TABLE IF NOT EXISTS discord_schedule_settings (
    guild_id          text PRIMARY KEY,
    send_channel_id   text,
    send_interval     text,
    send_history_size int,
    updated_at        timestamptz NOT NULL DEFAULT now()
);
