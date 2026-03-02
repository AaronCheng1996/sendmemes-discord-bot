-- Track when each album was last sent in the scheduled channel,
-- and reserve a column for the future positive-rating feedback system.
ALTER TABLE albums
    ADD COLUMN last_sent_at    timestamptz,
    ADD COLUMN positive_rating int NOT NULL DEFAULT 0;

-- Index used by GetRandomExcludeRecent (ORDER BY last_sent_at DESC LIMIT n).
CREATE INDEX IF NOT EXISTS albums_last_sent_at_idx ON albums (last_sent_at DESC NULLS LAST);
