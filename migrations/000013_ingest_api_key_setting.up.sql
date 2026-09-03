-- Move the ingest credential out of the environment and into the settings row,
-- so it can be rotated from the dashboard instead of by editing .env and
-- restarting the container.
--
-- The env var stays a seed and a fallback: an existing deployment keeps working
-- untouched, and a database with no key falls back to INGEST_API_KEY rather
-- than locking the crawler out.
--
-- It is never read back over the API. The dashboard is told only whether a key
-- is set, so an admin session cannot hand the credential to anyone looking over
-- a shoulder or through the browser's network tab.
ALTER TABLE app_settings
    ADD COLUMN IF NOT EXISTS ingest_api_key text;
