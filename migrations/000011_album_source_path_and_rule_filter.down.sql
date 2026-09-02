ALTER TABLE delivery_rules DROP COLUMN IF EXISTS album_filter;

ALTER TABLE albums DROP COLUMN IF EXISTS source_path;
