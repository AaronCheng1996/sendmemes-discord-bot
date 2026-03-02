DROP INDEX IF EXISTS albums_last_sent_at_idx;
ALTER TABLE albums
    DROP COLUMN IF EXISTS positive_rating,
    DROP COLUMN IF EXISTS last_sent_at;
