-- Restore the old placeholder names. {prefix} cannot be brought back: the up
-- migration deleted it rather than renaming it, so templates that used it come
-- back without their test marker (which the renderer now adds by itself).

UPDATE app_settings
SET message_style = replace(replace(
        message_style::text, '{shown}', '{count}'),
        '{album_total}', '{total}')::jsonb
WHERE message_style::text LIKE '%{shown}%'
   OR message_style::text LIKE '%{album_total}%';

UPDATE delivery_rules
SET message_style = replace(replace(
        message_style::text, '{shown}', '{count}'),
        '{album_total}', '{total}')::jsonb
WHERE message_style::text LIKE '%{shown}%'
   OR message_style::text LIKE '%{album_total}%';

UPDATE albums
SET send_config_json = replace(replace(
        send_config_json::text, '{shown}', '{count}'),
        '{album_total}', '{total}')::jsonb
WHERE send_config_json::text LIKE '%{shown}%'
   OR send_config_json::text LIKE '%{album_total}%';
