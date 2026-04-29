# sendmemes-discord-bot

A Discord bot that periodically posts albums of memes (sourced from a pCloud
folder) to a Discord channel, with a small admin REST API and a Vue admin
dashboard for managing the catalog.

The repository started from a Go Clean Architecture template, so the layout
keeps that structure: `cmd/`, `config/`, `internal/{app,entity,usecase,repo,controller}`,
`pkg/`, `migrations/`, with the admin UI pulled in as a git submodule under
`ui/`.

## Features

### Discord side

- **Scheduled album sends** — picks a random album and uploads its images to a
  configured channel on a fixed interval. Recently sent albums are skipped via
  `last_sent_at`; the exclusion window resets automatically once the bot has
  cycled through every album.
- **Reaction feedback** — any non-bot reaction on a scheduled message
  increments the album's `positive_rating` (in-memory map of the latest 200
  message → album mappings).
- **Per-guild override** — `discord_schedule_settings` lets the channel,
  interval, and history size be set per guild without redeploying. Env vars
  act as fallbacks.
- **pCloud sync** — periodically walks the configured pCloud root folder and
  reconciles albums/images. Image download URLs are short-lived, so the bot
  resolves them on demand and caches them in memory (~50 min TTL) to keep
  pCloud API usage low.
- **Manual trigger** — admin endpoint can fire a scheduled send immediately.

### Admin REST API (`/v1/admin/*`, gated by `X-Admin-Key`)

- Albums CRUD (`/albums`)
- Images CRUD (`/images`, optional `album_id` scope)
- Per-guild schedule read/update (`/schedule`)
- Manual schedule trigger (`/schedule/trigger-now`)
- Aggregated system status (DB ping + Discord session + effective schedule) at
  `/system/status`
- Audit trail in `admin_audit_logs` (actor from `X-Admin-Actor`, otherwise
  `api_key`)

List endpoints return a paginated envelope and embed a resolved
`preview_url` per row so the dashboard can render thumbnails without extra
round-trips:

```json
{ "items": [...], "total": 0, "offset": 0, "limit": 50 }
```

### Other endpoints

- `GET /healthz` — liveness probe
- `GET /metrics` — Prometheus, when `METRICS_ENABLED=true`
- `GET /swagger/*` — Swagger UI for the legacy translation routes (kept while
  those routes remain wired)

## Project layout

```
cmd/app                 # main entry point
config                  # env-driven config
internal/app            # wiring (Run) and migration init (build tag: migrate)
internal/controller
    restapi             # Fiber HTTP router, middleware, v1 handlers
    discord             # discordgo bot, scheduler, command handlers
internal/usecase        # business logic (admin, images, sync, settings, ...)
internal/repo
    persistent          # PostgreSQL implementations
    webapi              # external APIs (pCloud)
internal/entity         # domain types
migrations              # single consolidated init migration
pkg/{httpserver,logger,postgres}
sample                  # default fallback image (embedded)
ui                      # Vue 3 admin dashboard (git submodule)
```

## Configuration

All configuration is driven by environment variables (see `.env.example`).

Highlights:

| Variable | Purpose |
|---|---|
| `HTTP_PORT`, `HTTP_PUBLIC_URL` | HTTP server bind/port and external base URL used in resolved preview URLs |
| `POSTGRES_*` / `PG_URL` | PostgreSQL connection (PG_URL takes precedence) |
| `ADMIN_API_KEY` | Required for every `/v1/admin/*` request and for the UI sign-in |
| `DISCORD_TOKEN`, `DISCORD_APPLICATION_ID`, `DISCORD_GUILD_ID` | Discord bot identity |
| `DISCORD_CHANNEL_ID`, `DISCORD_SEND_INTERVAL`, `DISCORD_SEND_HISTORY_SIZE` | Defaults for scheduled sends; per-guild overrides live in `discord_schedule_settings` |
| `PCLOUD_ACCESS_TOKEN` *or* `PCLOUD_USERNAME` + `PCLOUD_PASSWORD` | pCloud authentication. Token (cookie/userinfo) is preferred; pCloud's API does not support 2FA |
| `CLOUD_MAIN_FOLDER_ID` | pCloud folder ID that holds album subfolders |
| `PCLOUD_API_ENDPOINT` | `https://api.pcloud.com` (US) or `https://eapi.pcloud.com` (EU) |
| `PCLOUD_SYNC_INTERVAL` | How often the bot reconciles pCloud → DB |
| `METRICS_ENABLED`, `SWAGGER_ENABLED` | Toggle Prometheus and Swagger handlers |

## Running locally

The repository ships with a Docker Compose stack that runs PostgreSQL, the bot,
and an Nginx reverse proxy.

```sh
git submodule update --init --recursive
cp .env.example .env
# Fill in DISCORD_*, PCLOUD_*, ADMIN_API_KEY in .env
docker compose up -d --build
docker compose logs -f app
```

To run just the database and the Go binary on the host (good for debugging):

```sh
make compose-up   # postgres only
make run          # builds with the migrate tag and runs cmd/app
```

The bot runs database migrations automatically when built with the `migrate`
build tag (already enabled in the Dockerfile and `make run`).

## Database migrations

`migrations/000001_init.up.sql` is the single source of truth — there are no
incremental migrations because the project is still pre-production. Reset
the schema with:

```sh
make db-reset       # drops everything, reapplies init
```

If the schema needs to change after the project goes live, add new migration
files normally (`make migrate-create name=<title>`).

## Admin UI (frontend)

The Vue 3 dashboard lives in the `ui/` submodule. See `ui/README.md` for
setup. From the repo root:

```sh
cd ui
npm install
npm run dev   # http://localhost:5173 by default
```

Sign in with the `ADMIN_API_KEY` you set in `.env`. The key is held only in
the browser's `sessionStorage`.

## Development workflow

| Task | Command |
|---|---|
| Format Go code | `make format` |
| Lint | `make linter-golangci` |
| Unit tests | `make test` |
| Integration test stack (legacy translation API) | `make compose-up-integration-test` |
| Regenerate Swagger | `make swag-v1` |
| Regenerate mocks | `make mock` |
| Pre-commit bundle | `make pre-commit` |

## Roadmap / known follow-ups

- Drop the legacy translation feature (`/v1/translation/*`, `history` table,
  `internal/usecase/translation`, `internal/repo/webapi/translation_google`,
  swagger spec, integration test) once the demo wiring is no longer needed.
- Server-side filtering / sorting for the admin list endpoints (current UI
  sorts and filters within the loaded page).
- Weighted album selection that biases toward higher `positive_rating`
  (`ORDER BY RANDOM() * (1 + positive_rating) DESC`).
- `/album_stats` slash command — surface top-rated albums in Discord.
- Move in-code tunables (`albumBatchSize`, `reactMapMaxSize`,
  `downloadTimeout`) to env when deployments need to differ.
- Replace the `*` CORS allow-list with an explicit dashboard origin once a
  hosted UI URL is decided.
- Audit-log retention / API surface (currently write-only).

## License

MIT — see `LICENSE`.
