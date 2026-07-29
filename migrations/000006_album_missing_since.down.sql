ALTER TABLE images
    DROP CONSTRAINT IF EXISTS images_album_id_fkey;

ALTER TABLE images
    ADD CONSTRAINT images_album_id_fkey
    FOREIGN KEY (album_id) REFERENCES albums (id);

ALTER TABLE albums
    DROP COLUMN IF EXISTS missing_since;
