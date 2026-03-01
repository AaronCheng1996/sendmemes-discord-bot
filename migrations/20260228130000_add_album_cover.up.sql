ALTER TABLE albums
    ADD COLUMN has_cover      boolean NOT NULL DEFAULT false,
    ADD COLUMN cover_image_id int     REFERENCES images(id) ON DELETE SET NULL;
