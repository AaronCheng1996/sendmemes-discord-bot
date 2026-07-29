-- Message presentation is resolved in three layers: app defaults, then the
-- delivery rule, then the album's send_config_json. Every column here is
-- nullable/empty by design — that is what "inherit from the layer below" means.

-- App-wide defaults.
ALTER TABLE app_settings
    ADD COLUMN IF NOT EXISTS default_use_embed boolean,
    ADD COLUMN IF NOT EXISTS default_title     text,
    ADD COLUMN IF NOT EXISTS default_body      text;

-- Per-rule overrides. caption_template (added in 000005) keeps its name and now
-- acts as the rule's body, so existing templates keep working untouched.
ALTER TABLE delivery_rules
    ADD COLUMN IF NOT EXISTS use_embed      boolean,
    ADD COLUMN IF NOT EXISTS title_template text;
