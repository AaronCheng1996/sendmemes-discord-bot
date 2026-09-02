-- Let a delivery rule apply to only part of the library, addressed by where a
-- folder lives rather than by listing albums one at a time.
--
-- The motivating case: a crawler drops artwork into Crawler/<Artist>, minting a
-- new album per artist. Those should stay out of the daily scheduled push but
-- still fire a "new files" notification, and nobody wants to tick a box every
-- time the crawler finds a new artist.

-- albums.source_path is the folder's full path from the walked root, the root's
-- own name included (e.g. "Crawler/SomeArtist"). The album is still identified
-- by its NAME; this is only what the path filters below match against. NULL
-- until the next sync fills it in.
ALTER TABLE albums
    ADD COLUMN IF NOT EXISTS source_path text;

-- album_filter narrows which albums the rule applies to:
--   {}                                          every album (the default)
--   {"mode":"exclude","paths":["Crawler"]}      everything except that subtree
--   {"mode":"include","paths":["Crawler"]}      only that subtree
-- A path matches the folder itself and everything under it, case-insensitively.
ALTER TABLE delivery_rules
    ADD COLUMN IF NOT EXISTS album_filter jsonb NOT NULL DEFAULT '{}'::jsonb;
