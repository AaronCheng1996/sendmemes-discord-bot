-- The legacy translation feature (and its /v1/translation routes) has been
-- removed; the history table it wrote to is no longer read by anything.
DROP TABLE IF EXISTS history;
