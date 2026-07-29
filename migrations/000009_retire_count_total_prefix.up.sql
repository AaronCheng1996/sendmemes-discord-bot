-- Retire the {count}, {total} and {prefix} caption placeholders.
--
-- {total} meant a different thing in every send mode — the candidate pool in
-- Random/Custom, the real page count in Order, always 1 in Single/Video — so
-- captions built on it reported numbers nobody could interpret. It becomes
-- {album_total}: how much media the album actually holds. {count} becomes
-- {shown}, the files in this one message, which is what it always meant.
--
-- {prefix} is dropped outright. It expanded to "[TEST] " for admin previews,
-- which meant any template that forgot it produced test posts indistinguishable
-- from real ones; the marker is now applied automatically.
--
-- Unknown placeholders are left verbatim by the renderer rather than rendering
-- as 0, so without this rewrite every stored template would visibly break.
-- Placeholders only ever appear inside JSON string values, never as keys, so
-- rewriting the document as text is unambiguous.

UPDATE app_settings
SET message_style = replace(replace(replace(
        message_style::text, '{count}', '{shown}'),
        '{total}', '{album_total}'),
        '{prefix}', '')::jsonb
WHERE message_style::text LIKE '%{count}%'
   OR message_style::text LIKE '%{total}%'
   OR message_style::text LIKE '%{prefix}%';

UPDATE delivery_rules
SET message_style = replace(replace(replace(
        message_style::text, '{count}', '{shown}'),
        '{total}', '{album_total}'),
        '{prefix}', '')::jsonb
WHERE message_style::text LIKE '%{count}%'
   OR message_style::text LIKE '%{total}%'
   OR message_style::text LIKE '%{prefix}%';

UPDATE albums
SET send_config_json = replace(replace(replace(
        send_config_json::text, '{count}', '{shown}'),
        '{total}', '{album_total}'),
        '{prefix}', '')::jsonb
WHERE send_config_json::text LIKE '%{count}%'
   OR send_config_json::text LIKE '%{total}%'
   OR send_config_json::text LIKE '%{prefix}%';
