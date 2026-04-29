DROP TABLE IF EXISTS history;
DROP INDEX IF EXISTS admin_audit_logs_created_at_idx;
DROP TABLE IF EXISTS admin_audit_logs;
DROP TABLE IF EXISTS discord_schedule_settings;
DROP INDEX IF EXISTS albums_last_sent_at_idx;
DROP INDEX IF EXISTS images_file_id_idx;

ALTER TABLE IF EXISTS albums
    DROP CONSTRAINT IF EXISTS albums_cover_image_id_fkey;

DROP TABLE IF EXISTS images;
DROP TABLE IF EXISTS albums;
