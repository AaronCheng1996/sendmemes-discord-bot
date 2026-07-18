ALTER TABLE albums
    DROP CONSTRAINT IF EXISTS albums_send_mode_check;

ALTER TABLE albums
    DROP COLUMN IF EXISTS send_config_json;

ALTER TABLE albums
    DROP COLUMN IF EXISTS send_mode;
