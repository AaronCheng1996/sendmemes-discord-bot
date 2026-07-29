ALTER TABLE delivery_rules
    ADD COLUMN IF NOT EXISTS use_embed      boolean,
    ADD COLUMN IF NOT EXISTS title_template text,
    ADD COLUMN IF NOT EXISTS caption_template text;

ALTER TABLE app_settings
    ADD COLUMN IF NOT EXISTS default_use_embed boolean,
    ADD COLUMN IF NOT EXISTS default_title     text,
    ADD COLUMN IF NOT EXISTS default_body      text;

UPDATE delivery_rules
SET use_embed        = (message_style ->> 'use_embed')::boolean,
    title_template   = message_style ->> 'title',
    caption_template = message_style ->> 'body';

UPDATE app_settings
SET default_use_embed = (message_style ->> 'use_embed')::boolean,
    default_title     = message_style ->> 'title',
    default_body      = message_style ->> 'body';

ALTER TABLE delivery_rules DROP COLUMN IF EXISTS message_style;
ALTER TABLE app_settings DROP COLUMN IF EXISTS message_style;
