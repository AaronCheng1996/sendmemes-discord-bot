-- Consolidate message presentation into a single JSONB column per layer.
-- 000007 modelled it as one column per field, which meant a migration for every
-- new knob (colour, footer, author, image/thumbnail/timestamp toggles…). The
-- JSON shape mirrors entity.MessageStyle, where an absent key means "inherit".

ALTER TABLE app_settings
    ADD COLUMN IF NOT EXISTS message_style jsonb NOT NULL DEFAULT '{}'::jsonb;

ALTER TABLE delivery_rules
    ADD COLUMN IF NOT EXISTS message_style jsonb NOT NULL DEFAULT '{}'::jsonb;

-- Carry over anything already configured through the 000007 columns.
UPDATE app_settings
SET message_style = jsonb_strip_nulls(jsonb_build_object(
        'use_embed', default_use_embed,
        'title', NULLIF(default_title, ''),
        'body', NULLIF(default_body, '')))
WHERE message_style = '{}'::jsonb;

UPDATE delivery_rules
SET message_style = jsonb_strip_nulls(jsonb_build_object(
        'use_embed', use_embed,
        'title', NULLIF(title_template, ''),
        'body', NULLIF(caption_template, '')))
WHERE message_style = '{}'::jsonb;

ALTER TABLE app_settings
    DROP COLUMN IF EXISTS default_use_embed,
    DROP COLUMN IF EXISTS default_title,
    DROP COLUMN IF EXISTS default_body;

ALTER TABLE delivery_rules
    DROP COLUMN IF EXISTS use_embed,
    DROP COLUMN IF EXISTS title_template,
    DROP COLUMN IF EXISTS caption_template;
