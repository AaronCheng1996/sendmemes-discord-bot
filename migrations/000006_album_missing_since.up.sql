-- Track albums whose source folder disappeared, instead of leaving ghost rows.
-- Sync marks them rather than deleting, so positive_rating and send-mode/config
-- survive a transient source outage and are restored if the folder comes back.
ALTER TABLE albums
    ADD COLUMN IF NOT EXISTS missing_since timestamptz;

-- Deleting an album used to fail with a foreign-key violation whenever it still
-- had image rows (the original constraint had no ON DELETE action), which made
-- the dashboard's Delete button unusable for exactly the albums people want to
-- clean up. Cascade the delete instead.
ALTER TABLE images
    DROP CONSTRAINT IF EXISTS images_album_id_fkey;

ALTER TABLE images
    ADD CONSTRAINT images_album_id_fkey
    FOREIGN KEY (album_id) REFERENCES albums (id) ON DELETE CASCADE;
