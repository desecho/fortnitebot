# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Test Commands

```bash
go build .              # Build the binary
go test ./...           # Run all tests
go test -run TestName   # Run a single test
go run .                # Run the bot (requires env vars below)
```

## Required Environment Variables

- `TELEGRAM_BOT_TOKEN` — Telegram bot token from BotFather
- `FORTNITE_API_TOKEN` — Authorization header for fortnite-api.com
- `FORTNITE_API2_TOKEN` — x-api-key header for prod.api-fortnite.com
- `MONGODB_URI` (optional) — enables session tracking and daily snapshots
- `PLAYERS_FILE` (optional, default: `players.json`) — path to player config
- `POLL_TIMEOUT_SECS` (optional, default: 30, range: 1-60) — Telegram long-poll timeout

## Architecture

Single-package Go application (`package main`). No internal packages or subdirectories for Go code — all `.go` files are in the project root.

**Key files:**

- `main.go` — config loading, cron setup (daily snapshots at midnight UTC), Telegram long-polling loop
- `types.go` — all struct definitions, interfaces, and constants
- `commands.go` — command routing (`handleMessage`) and all bot command logic (stats, compare, session, snapshot)
- `providers.go` — implementations of `statsProvider`, `seasonProvider`, and `statusProvider` (HTTP clients for external APIs)
- `telegram.go` — `botClient` implementation wrapping the go-telegram-bot-api library
- `mongo.go` — `snapshotStore` implementation using MongoDB for daily stat snapshots
- `helpers_test.go` — shared test stubs (`stubStatsProvider`, `stubSeasonProvider`, `stubStatusProvider`, `stubSnapshotStore`, `roundTripFunc`)

**Interfaces** (defined in `types.go`, used for testability):

- `statsProvider` — player lookup and stat fetching (cached + fresh + season)
- `seasonProvider` — days remaining in current season
- `statusProvider` — Epic/Fortnite service status
- `botClient` — Telegram API abstraction (getUpdates, sendMessage)
- `snapshotStore` — MongoDB persistence for daily snapshots

**Testing pattern:** Tests use stub implementations from `helpers_test.go` rather than mocking frameworks. HTTP responses are faked via `roundTripFunc` injected into `http.Client.Transport`. All tests are in `_test.go` files alongside the code they test.

**Stat caching:** `fortniteAPIStatsProvider` caches responses in memory for 1 hour (`statsCacheTTL`). `FetchFresh` bypasses the cache (used for snapshots and `/sessioncurrent`).

## Deployment

Deployed to a DigitalOcean Kubernetes cluster. Pushing to `main` triggers the GitHub Actions deployment workflow which builds a Docker image, pushes to `ghcr.io`, and restarts the k8s deployment.
