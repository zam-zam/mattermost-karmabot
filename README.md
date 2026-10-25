# karmabot

**English** | [Русский](README-RU.md)

[![Website](https://img.shields.io/badge/website-zam--zam.github.io-4da3ff?style=flat-square)](https://zam-zam.github.io/mattermost-karmabot/)

A karma bot for [Mattermost](https://mattermost.com) (tested against 10.10.x).
Teammates thank each other by mentioning the bot; the bot tracks per-channel
karma scores over calendar periods (weekly by default, or monthly) with
daily limits, and announces the period's results at each rollover.

- Bot replies are localized: **English by default**, Russian via
  `KARMABOT_LANGUAGE=ru`; commands are English-only.
- SQLite (default) or PostgreSQL storage; karma is isolated per channel.
- The bot always replies in the command's thread; a command sent inside
  an existing thread stays in it.
- Karma periods follow the **calendar**: weeks (default) or months,
  rolling over at `KARMABOT_ROLLOVER_TIME` (09:00 by default) in
  `KARMABOT_TIMEZONE` (UTC by default; the IANA database is embedded, so
  it works in any container). Soft reset: history is kept, each period
  starts a fresh scoreboard.
- Mattermost API calls are retried automatically on transient failures
  (network errors, `429`/`502`/`503`, and `500`/`504` for read-only requests)
  with capped exponential backoff, so a blip does not drop a karma reply; the
  WebSocket reconnects the same way.

## Commands

In a channel (invite `@karmabot` first). The bot must be the **first word**
of the message; a message that mentions the bot anywhere else (mid-text,
with punctuation glued to the name) gets a hint with the correct format:

| Command | Effect |
| --- | --- |
| `@karmabot start` | Enable karma in the channel |
| `@karmabot stop` | Pause karma (data kept; `start` resumes the current period) |
| `@karmabot ++ @user [@user2 …] [reason]` | Give +1 karma to each mentioned user; the text after a mention becomes that user's reason |
| `@karmabot top` | Top-10 for the current period with who gave karma and why |
| `@karmabot help` | Cheat sheet |

In a direct message to the bot:

| Command | Effect |
| --- | --- |
| `karma` | Your karma for the current period, per channel |
| `help` | Cheat sheet |

### Limits

Per channel, per calendar day in the configured timezone, a giver can hand
out:

- at most **5 karma in total** (default; `KARMABOT_DAILY_TOTAL_LIMIT`), and
- at most **2 karma to the same person** (default;
  `KARMABOT_DAILY_PER_TARGET_LIMIT`).

Each limit can be set to `0` for unlimited. When both are set, the
per-target limit must not exceed the total.

Every `++` reply reports whether the karma was applied, the target's current
channel total, and the remaining budgets (unlimited budgets are omitted).
Self-karma is blocked; `--` is not supported.

### Reasons

The text following a mention becomes that user's karma reason — what the
karma was given for:

- `@karmabot ++ @user for the code review` — the reason is «for the code
  review»;
- `@karmabot ++ @user1 for the review @user2 for the deploy` — each user
  gets their own reason;
- no text after a mention — karma without a reason, as before.

The reason is echoed in the grant reply, stored in the grant ledger, and
shown in the summaries (`top` and the period summary): under each entry,
grouped by giver — reasoned grants list their reasons (at most 3, then
«and N more»), reasonless ones show just the count. A reason is capped at
200 characters, and the ledger is pruned at each rollover once the
results have been posted.

```
🏅 Top-10 this week (24 August – 30 August):
1. @alice — ⭐⭐⭐ 3
  from @bob: 2 — for the code review, for the release help
  from @bob: 1
2. @dave — ⭐ 1
  from @bob: 1
```

Scores everywhere (grant replies, `top`, period summary, DM report) are
shown as stars: one star per point, then the total — e.g. 3 karma renders
as `⭐⭐⭐ 3`.

### Period summary

Periods follow the calendar: **weeks** (default, Monday to Sunday) or
**months**, rolling over at `KARMABOT_ROLLOVER_TIME` (09:00 by default) in
`KARMABOT_TIMEZONE` (UTC by default). At each rollover the bot posts two
messages to every enabled channel: the finished period's top-5 with its
bounds and each entry's giver breakdown (a date range for weeks, e.g.
«24 August – 30 August»; the month name for months), and the new period's
announcement with its bounds. New karma after that moment counts toward
the new period.

> `KARMABOT_PERIOD_DAYS` (period length in days) is no longer supported;
> if it is still set, the bot logs a warning at startup. Switching the
> period kind on an existing database starts a fresh scoreboard: older
> rows are kept but no longer shown, and the bot logs a warning at startup
> about the ignored rows. Weekly data written by previous versions stays
> compatible.

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
| `KARMABOT_DAILY_TOTAL_LIMIT` | `5` | Max karma a giver may hand out per channel per day (in `KARMABOT_TIMEZONE`); `0` = unlimited |
| `KARMABOT_DAILY_PER_TARGET_LIMIT` | `2` | Max karma to the same person per day; `0` = unlimited; must not exceed the total limit |
| `KARMABOT_PERIOD` | `week` | Scoreboard period: `week` (calendar weeks from Monday) or `month` (calendar months from the 1st) |
| `KARMABOT_TIMEZONE` | `UTC` | IANA timezone (e.g. `Europe/Moscow`) for period boundaries, the rollover time and daily limits; tzdata is embedded, so slim containers work too |
| `KARMABOT_ROLLOVER_TIME` | `09:00` | Time of day (HH:MM, 24-hour) when one period ends and the next begins, in `KARMABOT_TIMEZONE` |
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
