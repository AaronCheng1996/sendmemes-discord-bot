ALTER TABLE delivery_rules
    DROP COLUMN IF EXISTS title_template,
    DROP COLUMN IF EXISTS use_embed;

ALTER TABLE app_settings
    DROP COLUMN IF EXISTS default_body,
    DROP COLUMN IF EXISTS default_title,
    DROP COLUMN IF EXISTS default_use_embed;
