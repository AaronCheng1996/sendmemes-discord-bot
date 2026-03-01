DROP INDEX IF EXISTS images_file_id_idx;

ALTER TABLE images
    DROP COLUMN IF EXISTS file_id,
    DROP COLUMN IF EXISTS album_id;

DROP TABLE IF EXISTS albums;
