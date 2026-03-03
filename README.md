# Fortnite Telegram Bot

This is a small Go Telegram bot that reads Fortnite stats from the Fortnite API and responds to chat commands.

It only uses these fields from `stats.all.overall`:

- `wins`
- `kills`
- `killsPerMatch`
- `deaths`
- `kd`
- `matches`
- `winRate`
- `minutesPlayed`

The included `data.json` file is only a sample response payload reference. The bot reads the configured players from `players.json`, then fetches each player's live stats with the Fortnite API.

## Commands

- `/players`
- `/season`
- `/status`
- `/stats [player]`
- `/seasonstats [player]`
- `/compare <player1> <player2> [player3 ...]`
- `/seasoncompare <player1> <player2> [player3 ...]`

## Setup

1. Create a bot with BotFather and copy the token.
2. Export your Telegram token and Fortnite API token:

```bash
export TELEGRAM_BOT_TOKEN=your_bot_token_here
export FORTNITE_API_TOKEN=your_fortnite_api_token_here
export FORTNITE_API2_TOKEN=your_second_fortnite_api_token_here
```

3. Run the bot:

```bash
go run .
```

## Docker

Build the image:

```bash
docker build -t fortnitebot .
```

Run the bot:

```bash
docker run --rm \
  -e TELEGRAM_BOT_TOKEN=your_bot_token_here \
  -e FORTNITE_API_TOKEN=your_fortnite_api_token_here \
  -e FORTNITE_API2_TOKEN=your_second_fortnite_api_token_here \
  fortnitebot
```

If you want to use a different player list, mount your own `players.json` file to `/app/players.json`.

## Player Configuration

`players.json` is an array of player entries:

```json
[
  {
    "name": "scrap8653",
    "accountId": "72dc87e0050f48c1a692d159b8232a05"
  }
]
```

- `name`: the player name used in bot output and compare commands.
- `accountId`: the Epic account ID used to call `https://fortnite-api.com/v2/stats/br/v2/<account_id>`.

To add a friend, add another object with their player name and account ID.

## Notes

- The bot uses Telegram long polling over the official Bot API.
- The bot sends the `Authorization` header using `FORTNITE_API_TOKEN` when it calls the Fortnite API.
- The bot sends the `x-api-key` header using `FORTNITE_API2_TOKEN` when it calls `https://prod.api-fortnite.com/api/v1/season`.
- `/season` reads `seasonDateEnd` from that response and reports how many days are left in the current season.
- `/status` reads `https://status.epicgames.com/api/v2/summary.json` and reports the Epic-wide summary, the overall Fortnite status, and each Fortnite service status.
- `/stats` fetches and lists `stats.all.overall` for every player in `players.json`, or only one player when called as `/stats <player>`.
- `/seasonstats` fetches and lists `stats.all.overall` for every player in `players.json` using `?timeWindow=season`, or only one player when called as `/seasonstats <player>`.
- `/compare` compares two or more players using `stats.all.overall`.
- `/seasoncompare` compares two or more players using `stats.all.overall` from `?timeWindow=season`.
- Player stat requests are fetched in parallel to reduce response time.
- Successful stat responses are cached in memory for 1 hour.
- The bot ignores other Fortnite fields in the response and only reads `stats.all.overall`.
