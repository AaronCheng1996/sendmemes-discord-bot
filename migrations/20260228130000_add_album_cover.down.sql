ALTER TABLE albums
    DROP COLUMN IF EXISTS cover_image_id,
    DROP COLUMN IF EXISTS has_cover;
