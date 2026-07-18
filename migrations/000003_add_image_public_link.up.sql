-- Permanent pCloud public share link for a media file. Unlike getfilelink URLs
-- (which are temporary and bound to the requesting IP), a public link created via
-- getfilepublink never expires and is viewable from any IP, so it is persisted
-- once and reused for every delivery.
ALTER TABLE images
    ADD COLUMN IF NOT EXISTS public_link text;
