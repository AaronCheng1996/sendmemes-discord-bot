# Config Inventory

## Environment Variables

- `APP_NAME`, `APP_VERSION`
- `HTTP_PORT`, `HTTP_USE_PREFORK_MODE`, `HTTP_PUBLIC_URL`
- `LOG_LEVEL`
- `PG_POOL_MAX`, `PG_URL`, `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_HOST`, `POSTGRES_PORT`, `POSTGRES_DB`, `POSTGRES_SSL_MODE`
- `ADMIN_API_KEY`
- `DISCORD_TOKEN`, `DISCORD_APPLICATION_ID`, `DISCORD_GUILD_ID`, `DISCORD_CHANNEL_ID`, `DISCORD_SEND_INTERVAL`, `DISCORD_SEND_HISTORY_SIZE`, `DISCORD_VERBOSE_LOG`
- `PCLOUD_ACCESS_TOKEN`, `PCLOUD_USERNAME`, `PCLOUD_PASSWORD`, `CLOUD_MAIN_FOLDER_ID`, `PCLOUD_API_ENDPOINT`, `PCLOUD_SYNC_INTERVAL`
- `METRICS_ENABLED`, `SWAGGER_ENABLED`

## Runtime DB Settings (discord_schedule_settings)

- Table: `discord_schedule_settings`
- Columns:
  - `guild_id` (PK)
  - `send_channel_id`
  - `send_interval` (Go duration string)
  - `send_history_size`
  - `updated_at`

## Override Rules

- Schedule values are resolved per guild.
- DB value wins when present and valid.
- Missing DB value falls back to env:
  - `send_channel_id` -> `DISCORD_CHANNEL_ID`
  - `send_interval` -> `DISCORD_SEND_INTERVAL`
  - `send_history_size` -> `DISCORD_SEND_HISTORY_SIZE`

## In-Code Tunables (Not Yet Externalized)

- `albumBatchSize` (default 10)
- `albumPoolSize` (default `albumBatchSize * 2`)
- `discordMsgLimit` (24 MB)
- `downloadTimeout` (5 minutes)
- `reactMapMaxSize` (200 tracked scheduled messages)

## Suggested Next Externalization Targets

- Move `albumBatchSize` and `albumPoolSize` to env if you want to tune sending aggressiveness by deployment.
- Move `reactMapMaxSize` to env if reaction tracking retention should vary by server traffic.
- Move `downloadTimeout` to env when different network regions need different HTTP timeout behavior.
