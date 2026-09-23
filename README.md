# karmabot

**English** | [Русский](README-RU.md)

[![Website](https://img.shields.io/badge/website-zam--zam.github.io-4da3ff?style=flat-square)](https://zam-zam.github.io/mattermost-karmabot/)

A karma bot for [Mattermost](https://mattermost.com) (tested against 10.10.x).
Teammates thank each other by mentioning the bot; the bot tracks per-channel
karma scores over rolling periods (weekly by default) with daily limits, and
announces the period's results at each rollover.

- Bot replies are localized: **English by default**, Russian via
  `KARMABOT_LANGUAGE=ru`; commands are English-only.
- SQLite (default) or PostgreSQL storage; karma is isolated per channel.
- The bot always replies in the command's thread; a command sent inside
  an existing thread stays in it.
- Karma periods roll over every **7 days by default** at 00:00 UTC
  (`KARMABOT_PERIOD_DAYS`; soft reset: history is kept, each period starts a
  fresh scoreboard).
- Mattermost API calls are retried automatically on transient failures
  (network errors, `429`/`502`/`503`, and `500`/`504` for read-only requests)
  with capped exponential backoff, so a blip does not drop a karma reply; the
  WebSocket reconnects the same way.

## Commands

In a channel (invite `@karmabot` first):

| Command | Effect |
| --- | --- |
| `@karmabot start` | Enable karma in the channel |
| `@karmabot stop` | Pause karma (data kept; `start` resumes the week) |
| `@karmabot ++ @user [@user2 …]` | Give +1 karma to each mentioned user |
| `@karmabot top` | Top-10 for the current period |
| `@karmabot help` | Cheat sheet |

In a direct message to the bot:

| Command | Effect |
| --- | --- |
| `karma` | Your karma for the current period, per channel |
| `help` | Cheat sheet |

### Limits

Per channel, per UTC calendar day, a giver can hand out:

- at most **5 karma in total** (default; `KARMABOT_DAILY_TOTAL_LIMIT`), and
- at most **2 karma to the same person** (default;
  `KARMABOT_DAILY_PER_TARGET_LIMIT`).

Each limit can be set to `0` for unlimited. When both are set, the
per-target limit must not exceed the total.

Every `++` reply reports whether the karma was applied, the target's current
channel total, and the remaining budgets (unlimited budgets are omitted).
Self-karma is blocked; `--` is not supported.

Scores everywhere (grant replies, `top`, weekly summary, DM report) are
shown as stars: one star per point, then the total — e.g. 3 karma renders
as `⭐⭐⭐ 3`.

### Period summary

At each period rollover (every `KARMABOT_PERIOD_DAYS` days, 7 by default,
at 00:00 UTC) the bot posts the finished period's top-5 to every enabled
channel that had any karma. New karma after that moment counts toward the
new period.

> Changing the period length on an existing database starts a fresh
> scoreboard: older rows are kept but no longer shown, and the bot logs a
> warning at startup about the ignored rows.

## Setup

### 1. Create the bot account

1. Mattermost **System Console → Integrations → Bot Accounts**: make sure
   bot accounts are enabled.
2. **Integrations → Bot Accounts → Add Bot Account**: create a bot, e.g.
   `karmabot`.
3. Create a **personal access token** for the bot (on the bot's profile or
   via the account settings) and copy it.

### 2. Configure

Copy `.env.example` to `.env` (or export the variables):

| Param | Default | Description |
| --- | --- | --- |
| `KARMABOT_MATTERMOST_URL` | — (required) | Mattermost server base URL (`http://` or `https://`) |
| `KARMABOT_MATTERMOST_TOKEN` | — (required) | Bot account personal access token |
| `KARMABOT_DB_DRIVER` | `sqlite` | Database backend: `sqlite` or `postgres` |
| `KARMABOT_DB_PATH` | `./data/karmabot.db` | SQLite file location; directory is created if missing |
| `KARMABOT_DB_DSN` | — (required for postgres) | PostgreSQL connection string, e.g. `postgres://user:pass@host:5432/karmabot` |
| `KARMABOT_LOG_LEVEL` | `info` | Log level: `debug`, `info`, `warn`, or `error` |
| `KARMABOT_LANGUAGE` | `en` | Reply language: `en` or `ru`; commands are English-only |
| `KARMABOT_DAILY_TOTAL_LIMIT` | `5` | Max karma a giver may hand out per channel per UTC day; `0` = unlimited |
| `KARMABOT_DAILY_PER_TARGET_LIMIT` | `2` | Max karma to the same person per UTC day; `0` = unlimited; must not exceed the total limit |
| `KARMABOT_PERIOD_DAYS` | `7` | Scoreboard reset interval in days at 00:00 UTC (1–365) |
| `KARMABOT_ADMIN_USERNAME` | — (unset) | Mattermost username of the admin; enables admin commands; unset disables them |

Admin commands (work in a channel or as a direct message to the bot, and
only for the configured admin):

| Command | Effect |
| --- | --- |
| `@karmabot status` | list every channel where karma is enabled |

### 3. Run

With Docker Compose (SQLite storage, default):

```bash
cp .env.example .env   # fill in your Mattermost URL and token
docker compose up -d
```

With Docker Compose and PostgreSQL 18:

```bash
cp .env.example .env   # optionally set POSTGRES_PASSWORD there
docker compose -f docker-compose.postgres.yml up -d
```

Each file is a complete, standalone stack — pick the one you need, no
combining required. Both pull `ghcr.io/zam-zam/mattermost-karmabot:latest`;
add `--build` to run an image built from your checkout instead. The
PostgreSQL stack adds a `postgres` service with a health check the bot
waits for. To follow the logs:

```bash
docker logs -f karmabot-karmabot-1            # SQLite stack
docker logs -f karmabot-postgres-karmabot-1   # PostgreSQL stack
```

From source (Go ≥ 1.26):

```bash
go build -o karmabot ./cmd/karmabot
./karmabot
```

With plain Docker:

```bash
docker build -t karmabot .
docker run -d --name karmabot --env-file .env -v karmabot-data:/data karmabot
```

Invite the bot to a channel, then say `@karmabot start`.

> The bot needs to be a member of a channel to hear and post there; there is
> no way for it to join channels by itself.

## Development

```bash
go test ./...   # unit tests (parser, periods, limits, storage)
go vet ./...
```

Layout:

```
cmd/karmabot/          wiring, signal handling
internal/config/       envconfig + .env loading
internal/storage/      SQLite/PostgreSQL schema and queries
internal/mmclient/     Mattermost REST + WebSocket client with retries and reconnect
internal/bot/          event routing, commands, localized messages, periods
internal/scheduler/    period-rollover summary
```
