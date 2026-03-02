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
- `/stats [player]`
- `/seasonstats [player]`
- `/compare <player1> <player2>`
- `/seasoncompare <player1> <player2>`

## Setup

1. Create a bot with BotFather and copy the token.
2. Export your Telegram token and Fortnite API token:

```bash
export TELEGRAM_BOT_TOKEN=your_bot_token_here
export FORTNITE_API_TOKEN=your_fortnite_api_token_here
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

- `name`: the player name used in bot output and for `/compare`.
- `accountId`: the Epic account ID used to call `https://fortnite-api.com/v2/stats/br/v2/<account_id>`.

To add a friend, add another object with their player name and account ID.

## Notes

- The bot uses Telegram long polling over the official Bot API.
- The bot sends the `Authorization` header using `FORTNITE_API_TOKEN` when it calls the Fortnite API.
- `/stats` fetches and lists `stats.all.overall` for every player in `players.json`, or only one player when called as `/stats <player>`.
- `/seasonstats` fetches and lists `stats.all.overall` for every player in `players.json` using `?timeWindow=season`, or only one player when called as `/seasonstats <player>`.
- `/seasoncompare` compares two players using `stats.all.overall` from `?timeWindow=season`.
- Player stat requests are fetched in parallel to reduce response time.
- Successful stat responses are cached in memory for 1 hour.
- The bot ignores other Fortnite fields in the response and only reads `stats.all.overall`.
