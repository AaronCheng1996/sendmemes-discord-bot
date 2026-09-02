ALTER TABLE sync_events
    DROP CONSTRAINT IF EXISTS sync_events_event_type_check;

-- The narrower CHECK cannot be restored while rows carry the new types.
DELETE FROM sync_events
    WHERE event_type NOT IN ('album_created', 'files_added');

ALTER TABLE sync_events
    ADD CONSTRAINT sync_events_event_type_check
    CHECK (event_type IN ('album_created', 'files_added'));

ALTER TABLE sync_events DROP COLUMN IF EXISTS previous_name;
ALTER TABLE sync_events DROP COLUMN IF EXISTS removed_videos;
ALTER TABLE sync_events DROP COLUMN IF EXISTS removed_images;

DROP INDEX IF EXISTS images_album_live_idx;

-- Without the column a soft-deleted row would come back to life, so drop the
-- rows the sync had already retired.
DELETE FROM images WHERE deleted_at IS NOT NULL;

ALTER TABLE images DROP COLUMN IF EXISTS deleted_at;

DROP INDEX IF EXISTS albums_folder_id_idx;

ALTER TABLE albums DROP COLUMN IF EXISTS folder_id;
