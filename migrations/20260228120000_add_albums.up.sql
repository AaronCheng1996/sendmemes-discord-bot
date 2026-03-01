CREATE TABLE IF NOT EXISTS albums (
    id         serial PRIMARY KEY,
    name       text NOT NULL UNIQUE,
    created_at timestamptz DEFAULT now()
);

ALTER TABLE images
    ADD COLUMN IF NOT EXISTS album_id int REFERENCES albums(id),
    ADD COLUMN IF NOT EXISTS file_id  bigint;

CREATE UNIQUE INDEX IF NOT EXISTS images_file_id_idx ON images (file_id) WHERE file_id IS NOT NULL;
