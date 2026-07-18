ALTER TABLE albums
    ADD COLUMN IF NOT EXISTS send_mode text NOT NULL DEFAULT 'Random';

ALTER TABLE albums
    ADD COLUMN IF NOT EXISTS send_config_json jsonb NOT NULL DEFAULT '{}'::jsonb;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'albums_send_mode_check'
    ) THEN
        ALTER TABLE albums
            ADD CONSTRAINT albums_send_mode_check
            CHECK (send_mode IN ('Order', 'Random', 'Single', 'Custom'));
    END IF;
END $$;
