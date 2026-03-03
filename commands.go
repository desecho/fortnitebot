package main

import (
	"fmt"
	"slices"
	"strings"
	"sync"
)

func handleMessage(provider statsProvider, season seasonProvider, status statusProvider, text string) string {
	fields := strings.Fields(strings.TrimSpace(text))
	if len(fields) == 0 {
		return ""
	}

	command := normalizeCommand(fields[0])
	args := fields[1:]

	switch command {
	case "/start", "/help":
		return helpText()
	case "/players":
		return playersText(provider)
	case "/season":
		return seasonText(season)
	case "/status":
		return statusText(status)
	case "/stats":
		return statsText(provider, args, false)
	case "/seasonstats":
		return statsText(provider, args, true)
	case "/compare":
		return compareText(provider, args, false)
	case "/seasoncompare":
		return compareText(provider, args, true)
	default:
		return "Unknown command. Use /help to see the available commands."
	}
}

func normalizeCommand(command string) string {
	command = strings.TrimSpace(command)
	if at := strings.IndexByte(command, '@'); at >= 0 {
		command = command[:at]
	}
	return strings.ToLower(command)
}

func helpText() string {
	lines := []string{
		"Fortnite stats bot",
		"",
		"Commands:",
		"/players",
		"/season",
		"/status",
		"/stats [player]",
		"/seasonstats [player]",
		"/compare <player1> <player2> [player3 ...]",
		"/seasoncompare <player1> <player2> [player3 ...]",
	}

	lines = append(lines, "", "Use /players to see the configured player names.")
	return strings.Join(lines, "\n")
}

func playersText(provider statsProvider) string {
	names := provider.Names()
	if len(names) == 0 {
		return "No players are configured."
	}

	return "Configured players:\n" + strings.Join(names, "\n")
}

func seasonText(provider seasonProvider) string {
	daysLeft, err := provider.DaysLeft()
	if err != nil {
		return fmt.Sprintf("Failed to fetch season info: %v", err)
	}
	if daysLeft == 0 {
		return "The current season has ended."
	}
	if daysLeft == 1 {
		return "Season ends in 1 day."
	}
	return fmt.Sprintf("Season ends in %d days.", daysLeft)
}

func statusText(provider statusProvider) string {
	summary, err := provider.Summary()
	if err != nil {
		return fmt.Sprintf("Failed to fetch Fortnite status: %v", err)
	}

	lines := []string{
		"Fortnite status",
		fmt.Sprintf("Epic overall: %s", fallbackText(summary.Epic, "Unknown")),
		fmt.Sprintf("Fortnite overall: %s", fallbackText(summary.Fortnite, "Unknown")),
	}

	if len(summary.Services) == 0 {
		lines = append(lines, "No Fortnite services are listed.")
		return strings.Join(lines, "\n")
	}

	lines = append(lines, "Services:")
	for _, service := range summary.Services {
		lines = append(lines, fmt.Sprintf("%s: %s", fallbackText(service.Name, "Unknown"), fallbackText(service.Status, "Unknown")))
	}

	return strings.Join(lines, "\n")
}

func statsText(provider statsProvider, args []string, season bool) string {
	if provider.Count() == 0 {
		return "No players are configured."
	}
	if len(args) > 1 {
		if season {
			return "Usage: /seasonstats [player]"
		}
		return "Usage: /stats [player]"
	}

	if len(args) == 1 {
		entry, ok := provider.Lookup(args[0])
		if !ok {
			return fmt.Sprintf("Unknown player %q. Use /players to see the configured player names.", args[0])
		}

		player, err := fetchStats(provider, entry, season)
		if err != nil {
			return fmt.Sprintf("Failed to fetch stats for %s: %v", entry.Name, err)
		}

		return formatStats(player)
	}

	results := fetchStatsBatch(provider, provider.Entries(), season)

	snapshots := make([]string, 0, len(results))
	for _, result := range results {
		if result.err != nil {
			snapshots = append(snapshots, fmt.Sprintf("%s\nFailed to fetch stats: %v", result.entry.Name, result.err))
			continue
		}
		snapshots = append(snapshots, formatStats(result.snapshot))
	}

	return strings.Join(snapshots, "\n\n")
}

func fetchStats(provider statsProvider, entry playerCatalogEntry, season bool) (playerSnapshot, error) {
	if season {
		return provider.FetchSeason(entry)
	}
	return provider.Fetch(entry)
}

func fetchStatsBatch(provider statsProvider, entries []playerCatalogEntry, season bool) []fetchResult {
	results := make([]fetchResult, len(entries))

	var wg sync.WaitGroup
	wg.Add(len(entries))

	for i, entry := range entries {
		i := i
		entry := entry

		go func() {
			defer wg.Done()

			snapshot, err := fetchStats(provider, entry, season)
			results[i] = fetchResult{
				entry:    entry,
				snapshot: snapshot,
				err:      err,
			}
		}()
	}

	wg.Wait()
	return results
}

func compareText(provider statsProvider, args []string, season bool) string {
	if provider.Count() < 2 {
		if season {
			return "Add at least two players to use /seasoncompare."
		}
		return "Add at least two players to use /compare."
	}
	if len(args) < 2 {
		if season {
			return "Usage: /seasoncompare <player1> <player2> [player3 ...]"
		}
		return "Usage: /compare <player1> <player2> [player3 ...]"
	}

	seen := make(map[string]struct{}, len(args))
	players := make([]playerCatalogEntry, 0, len(args))
	for _, rawName := range args {
		nameKey := strings.ToLower(strings.TrimSpace(rawName))
		if _, exists := seen[nameKey]; exists {
			return "Each player can only be listed once in a compare command."
		}

		player, ok := provider.Lookup(rawName)
		if !ok {
			return fmt.Sprintf("Unknown player %q. Use /players to see the configured player names.", rawName)
		}

		seen[nameKey] = struct{}{}
		players = append(players, player)
	}

	results := fetchStatsBatch(provider, players, season)
	for i, result := range results {
		if result.err != nil {
			return fmt.Sprintf("Failed to fetch compare stats for %s: %v", players[i].Name, result.err)
		}
	}

	snapshots := make([]playerSnapshot, 0, len(results))
	lines := make([]string, 0, 9)
	lines = append(lines, compareTitle(season))
	for _, result := range results {
		snapshots = append(snapshots, result.snapshot)
	}

	lines = append(lines,
		fmt.Sprintf("Wins leader: %s", leaderLabel(snapshots, func(line statLine) float64 { return float64(line.Wins) }, false)),
		fmt.Sprintf("Kills leader: %s", leaderLabel(snapshots, func(line statLine) float64 { return float64(line.Kills) }, false)),
		fmt.Sprintf("Kills/match leader: %s", leaderLabel(snapshots, func(line statLine) float64 { return line.KillsPerMatch }, false)),
		fmt.Sprintf("Lower deaths: %s", leaderLabel(snapshots, func(line statLine) float64 { return float64(line.Deaths) }, true)),
		fmt.Sprintf("KD leader: %s", leaderLabel(snapshots, func(line statLine) float64 { return line.KD }, false)),
		fmt.Sprintf("Matches leader: %s", leaderLabel(snapshots, func(line statLine) float64 { return float64(line.Matches) }, false)),
		fmt.Sprintf("Win rate leader: %s", leaderLabel(snapshots, func(line statLine) float64 { return line.WinRate }, false)),
		fmt.Sprintf("Hours played leader: %s", leaderLabel(snapshots, func(line statLine) float64 { return float64(line.MinutesPlayed) }, false)),
	)

	return strings.Join(lines, "\n")
}

func compareTitle(season bool) string {
	if season {
		return "Compare (season)"
	}
	return "Compare (overall)"
}

func formatStats(player playerSnapshot) string {
	line := player.stats
	lines := []string{
		playerLabel(player),
		fmt.Sprintf("Wins: %d", line.Wins),
		fmt.Sprintf("Kills: %d", line.Kills),
		fmt.Sprintf("Kills/match: %.2f", line.KillsPerMatch),
		fmt.Sprintf("Deaths: %d", line.Deaths),
		fmt.Sprintf("K/D: %.2f", line.KD),
		fmt.Sprintf("Matches: %d", line.Matches),
		fmt.Sprintf("Win rate: %.2f%%", line.WinRate),
		fmt.Sprintf("Hours played: %.2f", hoursPlayed(line.MinutesPlayed)),
	}

	return strings.Join(lines, "\n")
}

func hoursPlayed(minutes int64) float64 {
	return float64(minutes) / 60
}

func fallbackText(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func playerLabel(player playerSnapshot) string {
	return player.entry.Name
}

func leaderLabel(players []playerSnapshot, valueFn func(statLine) float64, lowerIsBetter bool) string {
	if len(players) == 0 {
		return ""
	}

	bestValue := valueFn(players[0].stats)
	winners := []string{playerLabel(players[0])}

	for _, player := range players[1:] {
		value := valueFn(player.stats)

		switch {
		case value == bestValue:
			winners = append(winners, playerLabel(player))
		case lowerIsBetter && value < bestValue:
			bestValue = value
			winners = []string{playerLabel(player)}
		case !lowerIsBetter && value > bestValue:
			bestValue = value
			winners = []string{playerLabel(player)}
		}
	}

	if len(winners) == 1 {
		return winners[0]
	}

	slices.Sort(winners)
	return "Tie (" + strings.Join(winners, " / ") + ")"
}
