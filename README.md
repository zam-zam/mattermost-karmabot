# karmabot

A karma bot for [Mattermost](https://mattermost.com) (tested against 10.10.x).
Teammates thank each other by mentioning the bot; the bot tracks weekly,
per-channel karma scores with daily limits, and announces the week's results
before the Monday reset.

- Bot replies are localized: **English by default**, Russian via
  `KARMABOT_LANGUAGE=ru`; commands are English-only.
- One SQLite file stores everything; karma is isolated per channel.
- The week rolls over at **Monday 00:00 UTC** (soft reset: history is kept,
  new karma starts a fresh week).

## Commands

In a channel (invite `@karmabot` first):

| Command | Effect |
| --- | --- |
| `@karmabot start` | Enable karma in the channel |
| `@karmabot stop` | Pause karma (data kept; `start` resumes the week) |
| `@karmabot ++ @user [@user2 …]` | Give +1 karma to each mentioned user |
| `@karmabot top` | Top-10 for the current week |
| `@karmabot help` | Cheat sheet |

In a direct message to the bot:

| Command | Effect |
| --- | --- |
| `karma` | Your karma for the current week, per channel |
| `help` | Cheat sheet |

### Limits

Per channel, per UTC calendar day, a user can give:

- at most **5 karma in total**, and
- at most **2 karma to the same person**.

Both limits are configurable: `KARMABOT_DAILY_TOTAL_LIMIT` (default 5) and
`KARMABOT_DAILY_PER_TARGET_LIMIT` (default 2); each must be at least 1, and
the per-target limit must not exceed the total.

Every `++` reply reports whether the karma was applied, the target's current
channel total, and both remaining budgets. Self-karma is blocked; `--` is
not supported.

Scores everywhere (grant replies, `top`, weekly summary, DM report) are
shown as stars: one star per point, then the total — e.g. 3 karma renders
as `⭐⭐⭐ 3`.

### Weekly summary

At Monday 00:00 UTC the bot posts the finished week's top-5 to every enabled
channel that had any karma. New karma after that moment counts toward the
new week.

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

```bash
KARMABOT_MATTERMOST_URL=https://mattermost.example.com
KARMABOT_MATTERMOST_TOKEN=<bot token>
KARMABOT_DB_PATH=./data/karmabot.db   # default
KARMABOT_LOG_LEVEL=info               # default
KARMABOT_LANGUAGE=en                  # default; ru for Russian replies
KARMABOT_DAILY_TOTAL_LIMIT=5          # default
KARMABOT_DAILY_PER_TARGET_LIMIT=2     # default
```

### 3. Run

From source (Go ≥ 1.26):

```bash
go build -o karmabot ./cmd/karmabot
./karmabot
```

With Docker:

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
internal/storage/      SQLite schema and queries
internal/mmclient/     Mattermost REST + WebSocket client with reconnect
internal/bot/          event routing, commands, localized messages, periods
internal/scheduler/    Monday 00:00 UTC weekly summary
```
