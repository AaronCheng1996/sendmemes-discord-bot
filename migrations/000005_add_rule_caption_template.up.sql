-- Per-rule caption template for Discord embed descriptions. NULL/empty means
-- "use the built-in default caption" (see renderCaption in internal/controller/discord).
ALTER TABLE delivery_rules ADD COLUMN caption_template text;
