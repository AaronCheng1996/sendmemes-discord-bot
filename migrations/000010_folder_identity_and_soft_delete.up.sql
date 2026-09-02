-- Follow a source folder across a rename, and turn sync-detected removals into
-- soft deletes so the dashboard can hide them without losing the rows.

-- albums.folder_id records the source's own folder identifier (a pCloud
-- folderid, or a path hash for the local source). The folder NAME remains the
-- album's identity; folder_id is the secondary key that lets a sync recognise a
-- renamed folder and carry its rating, send mode and config over instead of
-- creating a blank album beside it. NULL until the next sync backfills it.
ALTER TABLE albums
    ADD COLUMN IF NOT EXISTS folder_id bigint;

CREATE UNIQUE INDEX IF NOT EXISTS albums_folder_id_idx
    ON albums (folder_id) WHERE folder_id IS NOT NULL;

-- images.deleted_at marks a file the sync no longer finds. The row is kept so a
-- file that moves away and comes back is revived rather than re-announced, and
-- so the activity log can still name what disappeared. Every read path filters
-- on deleted_at IS NULL; the admin list opts back in explicitly.
ALTER TABLE images
    ADD COLUMN IF NOT EXISTS deleted_at timestamptz;

CREATE INDEX IF NOT EXISTS images_album_live_idx
    ON images (album_id) WHERE deleted_at IS NULL;

-- Removals and renames are recorded in the activity log as well. They are never
-- delivered to Discord (entity.SyncEventTriggerType maps them to no trigger).
ALTER TABLE sync_events
    ADD COLUMN IF NOT EXISTS removed_images int NOT NULL DEFAULT 0;

ALTER TABLE sync_events
    ADD COLUMN IF NOT EXISTS removed_videos int NOT NULL DEFAULT 0;

ALTER TABLE sync_events
    ADD COLUMN IF NOT EXISTS previous_name text;

ALTER TABLE sync_events
    DROP CONSTRAINT IF EXISTS sync_events_event_type_check;

ALTER TABLE sync_events
    ADD CONSTRAINT sync_events_event_type_check
    CHECK (event_type IN ('album_created', 'files_added', 'album_renamed', 'album_missing', 'files_removed'));
