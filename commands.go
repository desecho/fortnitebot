package main

import (
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"
)

func handleMessage(provider statsProvider, season seasonProvider, status statusProvider, store snapshotStore, text string) string {
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
	case "/session":
		return sessionText(provider, store, args)
	case "/sessioncurrent":
		return sessionCurrentText(provider, store, args)
	case "/sessions":
		return sessionsText(provider, store, args)
	case "/snapshot":
		return snapshotText(provider, store)
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
	return strings.Join([]string{
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
		"/session [player]",
		"/sessioncurrent [player]",
		"/sessions [player]",
		"",
		"Use /players to see the configured player names.",
	}, "\n")
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

func statusEmoji(status string) string {
	if status == "Operational" {
		return "🟢"
	}
	return "🔴"
}

func statusText(provider statusProvider) string {
	summary, err := provider.Summary()
	if err != nil {
		return fmt.Sprintf("Failed to fetch Fortnite status: %v", err)
	}

	lines := []string{
		"Fortnite status",
		fmt.Sprintf("Fortnite overall: %s %s", fallbackText(summary.Fortnite, "Unknown"), statusEmoji(summary.Fortnite)),
	}

	if len(summary.Services) == 0 {
		lines = append(lines, "No Fortnite services are listed.")
		return strings.Join(lines, "\n")
	}

	lines = append(lines, "Services:")
	for _, service := range summary.Services {
		lines = append(lines, fmt.Sprintf("%s: %s %s", fallbackText(service.Name, "Unknown"), fallbackText(service.Status, "Unknown"), statusEmoji(service.Status)))
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
		fmt.Sprintf("🏆 Wins leader: %s", leaderLabel(snapshots, func(line statLine) float64 { return float64(line.Wins) }, false)),
		fmt.Sprintf("💀 Kills leader: %s", leaderLabel(snapshots, func(line statLine) float64 { return float64(line.Kills) }, false)),
		fmt.Sprintf("🎯 Kills/match leader: %s", leaderLabel(snapshots, func(line statLine) float64 { return line.KillsPerMatch }, false)),
		fmt.Sprintf("☠️ Lower deaths: %s", leaderLabel(snapshots, func(line statLine) float64 { return float64(line.Deaths) }, true)),
		fmt.Sprintf("⚔️ KD leader: %s", leaderLabel(snapshots, func(line statLine) float64 { return line.KD }, false)),
		fmt.Sprintf("🎮 Matches leader: %s", leaderLabel(snapshots, func(line statLine) float64 { return float64(line.Matches) }, false)),
		fmt.Sprintf("📈 Win rate leader: %s", leaderLabel(snapshots, func(line statLine) float64 { return line.WinRate }, false)),
		fmt.Sprintf("⏱️ Hours played leader: %s", leaderLabel(snapshots, func(line statLine) float64 { return float64(line.MinutesPlayed) }, false)),
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
		player.entry.Name,
		fmt.Sprintf("🏆 Wins: %d", line.Wins),
		fmt.Sprintf("💀 Kills: %d", line.Kills),
		fmt.Sprintf("🎯 Kills/match: %.2f", line.KillsPerMatch),
		fmt.Sprintf("☠️ Deaths: %d", line.Deaths),
		fmt.Sprintf("⚔️ K/D: %.2f", line.KD),
		fmt.Sprintf("🎮 Matches: %d", line.Matches),
		fmt.Sprintf("📈 Win rate: %.2f%%", line.WinRate),
		fmt.Sprintf("⏱️ Hours played: %.2f", hoursPlayed(line.MinutesPlayed)),
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

func sessionText(provider statsProvider, store snapshotStore, args []string) string {
	if store == nil {
		return "Session tracking is not configured."
	}
	if provider.Count() == 0 {
		return "No players are configured."
	}
	if len(args) > 1 {
		return "Usage: /session [player]"
	}

	if len(args) == 1 {
		entry, ok := provider.Lookup(args[0])
		if !ok {
			return fmt.Sprintf("Unknown player %q. Use /players to see the configured player names.", args[0])
		}
		return formatPlayerSession(store, entry)
	}

	var results []string
	for _, entry := range provider.Entries() {
		results = append(results, formatPlayerSession(store, entry))
	}
	return strings.Join(results, "\n\n")
}

func formatPlayerSession(store snapshotStore, entry playerCatalogEntry) string {
	snapshots, err := store.RecentSnapshots(entry.AccountID, 2)
	if err != nil {
		return fmt.Sprintf("%s\nFailed to fetch session data: %v", entry.Name, err)
	}
	if len(snapshots) < 2 {
		return fmt.Sprintf("%s\nNot enough data to detect a session yet.", entry.Name)
	}

	session := detectSession(entry.Name, snapshots[0], snapshots[1])
	if session == nil {
		return fmt.Sprintf("%s\nNo recent gaming session detected.", entry.Name)
	}

	return formatSession(*session)
}

func sessionCurrentText(provider statsProvider, store snapshotStore, args []string) string {
	if store == nil {
		return "Session tracking is not configured."
	}
	if provider.Count() == 0 {
		return "No players are configured."
	}
	if len(args) > 1 {
		return "Usage: /sessioncurrent [player]"
	}

	if len(args) == 1 {
		entry, ok := provider.Lookup(args[0])
		if !ok {
			return fmt.Sprintf("Unknown player %q. Use /players to see the configured player names.", args[0])
		}
		return formatPlayerSessionCurrent(provider, store, entry)
	}

	var results []string
	for _, entry := range provider.Entries() {
		results = append(results, formatPlayerSessionCurrent(provider, store, entry))
	}
	return strings.Join(results, "\n\n")
}

func formatPlayerSessionCurrent(provider statsProvider, store snapshotStore, entry playerCatalogEntry) string {
	snapshots, err := store.RecentSnapshots(entry.AccountID, 1)
	if err != nil {
		return fmt.Sprintf("%s\nFailed to fetch session data: %v", entry.Name, err)
	}
	if len(snapshots) == 0 {
		return fmt.Sprintf("%s\nNo snapshot data available.", entry.Name)
	}

	live, err := provider.FetchFresh(entry)
	if err != nil {
		return fmt.Sprintf("%s\nFailed to fetch live stats: %v", entry.Name, err)
	}

	liveSnapshot := dailySnapshot{
		AccountID: entry.AccountID,
		Name:      entry.Name,
		Date:      time.Now().UTC().Format("2006-01-02"),
		Stats:     extractDailyStats(live.stats),
		CreatedAt: time.Now().UTC(),
	}

	session := detectSession(entry.Name, liveSnapshot, snapshots[0])
	if session == nil {
		return fmt.Sprintf("%s\nNo activity since last snapshot.", entry.Name)
	}

	return formatSession(*session)
}

func sessionsText(provider statsProvider, store snapshotStore, args []string) string {
	if store == nil {
		return "Session tracking is not configured."
	}
	if provider.Count() == 0 {
		return "No players are configured."
	}
	if len(args) > 1 {
		return "Usage: /sessions [player]"
	}

	if len(args) == 1 {
		entry, ok := provider.Lookup(args[0])
		if !ok {
			return fmt.Sprintf("Unknown player %q. Use /players to see the configured player names.", args[0])
		}
		return formatPlayerSessions(store, entry)
	}

	var results []string
	for _, entry := range provider.Entries() {
		results = append(results, formatPlayerSessions(store, entry))
	}
	return strings.Join(results, "\n\n")
}

func formatPlayerSessions(store snapshotStore, entry playerCatalogEntry) string {
	snapshots, err := store.RecentSnapshots(entry.AccountID, 8)
	if err != nil {
		return fmt.Sprintf("%s\nFailed to fetch session data: %v", entry.Name, err)
	}
	if len(snapshots) < 2 {
		return fmt.Sprintf("%s\nNot enough data to detect sessions yet.", entry.Name)
	}

	var sessions []sessionSummary
	for i := 0; i < len(snapshots)-1; i++ {
		session := detectSession(entry.Name, snapshots[i], snapshots[i+1])
		if session != nil {
			sessions = append(sessions, *session)
		}
	}

	if len(sessions) == 0 {
		return fmt.Sprintf("%s\nNo recent gaming sessions detected.", entry.Name)
	}

	lines := []string{fmt.Sprintf("%s - Recent sessions", entry.Name)}
	for _, s := range sessions {
		lines = append(lines, formatSessionCompact(s))
	}
	return strings.Join(lines, "\n")
}

func detectSession(playerName string, newer, older dailySnapshot) *sessionSummary {
	if newer.Stats.Matches <= older.Stats.Matches {
		return nil
	}

	delta := dailyStatLine{
		Wins:          newer.Stats.Wins - older.Stats.Wins,
		Top3:          newer.Stats.Top3 - older.Stats.Top3,
		Top5:          newer.Stats.Top5 - older.Stats.Top5,
		Top6:          newer.Stats.Top6 - older.Stats.Top6,
		Top10:         newer.Stats.Top10 - older.Stats.Top10,
		Top12:         newer.Stats.Top12 - older.Stats.Top12,
		Top25:         newer.Stats.Top25 - older.Stats.Top25,
		Kills:         newer.Stats.Kills - older.Stats.Kills,
		Deaths:        newer.Stats.Deaths - older.Stats.Deaths,
		Matches:       newer.Stats.Matches - older.Stats.Matches,
		MinutesPlayed: newer.Stats.MinutesPlayed - older.Stats.MinutesPlayed,
	}

	var kd float64
	if delta.Deaths > 0 {
		kd = float64(delta.Kills) / float64(delta.Deaths)
	}

	var winRate float64
	if delta.Matches > 0 {
		winRate = float64(delta.Wins) / float64(delta.Matches) * 100
	}

	var killsPerMatch float64
	if delta.Matches > 0 {
		killsPerMatch = float64(delta.Kills) / float64(delta.Matches)
	}

	return &sessionSummary{
		PlayerName:    playerName,
		Date:          newer.Date,
		Delta:         delta,
		KillsPerMatch: killsPerMatch,
		KD:            kd,
		WinRate:       winRate,
	}
}

func formatSession(s sessionSummary) string {
	lines := []string{
		fmt.Sprintf("%s - Session %s", s.PlayerName, s.Date),
		fmt.Sprintf("Matches: %d", s.Delta.Matches),
		fmt.Sprintf("Wins: %d", s.Delta.Wins),
		fmt.Sprintf("Kills: %d", s.Delta.Kills),
		fmt.Sprintf("Kills/match: %.2f", s.KillsPerMatch),
		fmt.Sprintf("Deaths: %d", s.Delta.Deaths),
		fmt.Sprintf("K/D: %.2f", s.KD),
		fmt.Sprintf("Win rate: %.2f%%", s.WinRate),
		fmt.Sprintf("Top 3: %d", s.Delta.Top3),
		fmt.Sprintf("Top 5: %d", s.Delta.Top5),
		fmt.Sprintf("Top 6: %d", s.Delta.Top6),
		fmt.Sprintf("Top 10: %d", s.Delta.Top10),
		fmt.Sprintf("Top 12: %d", s.Delta.Top12),
		fmt.Sprintf("Top 25: %d", s.Delta.Top25),
		fmt.Sprintf("Time played: %.1fh", float64(s.Delta.MinutesPlayed)/60),
	}
	return strings.Join(lines, "\n")
}

func formatSessionCompact(s sessionSummary) string {
	return fmt.Sprintf(
		"%s: %d matches, %d wins, %d kills, %.2f K/D, %.0f%% WR",
		s.Date, s.Delta.Matches, s.Delta.Wins, s.Delta.Kills, s.KD, s.WinRate,
	)
}

func snapshotText(provider statsProvider, store snapshotStore) string {
	if store == nil {
		return "Session tracking is not configured."
	}
	if provider.Count() == 0 {
		return "No players are configured."
	}
	return collectSnapshotsReport(provider, store)
}

func collectSnapshotsReport(provider statsProvider, store snapshotStore) string {
	today := time.Now().UTC().Format("2006-01-02")
	lines := []string{fmt.Sprintf("Collecting snapshots for %s", today)}

	for _, entry := range provider.Entries() {
		snapshot, err := provider.FetchFresh(entry)
		if err != nil {
			lines = append(lines, fmt.Sprintf("%s: failed to fetch (%v)", entry.Name, err))
			continue
		}
		daily := dailySnapshot{
			AccountID: entry.AccountID,
			Name:      entry.Name,
			Date:      today,
			Stats:     extractDailyStats(snapshot.stats),
			CreatedAt: time.Now().UTC(),
		}
		if err := store.UpsertSnapshot(daily); err != nil {
			lines = append(lines, fmt.Sprintf("%s: failed to store (%v)", entry.Name, err))
			continue
		}
		lines = append(lines, fmt.Sprintf("%s: done", entry.Name))
	}
	return strings.Join(lines, "\n")
}

func leaderLabel(players []playerSnapshot, valueFn func(statLine) float64, lowerIsBetter bool) string {
	if len(players) == 0 {
		return ""
	}

	bestValue := valueFn(players[0].stats)
	winners := []string{players[0].entry.Name}

	for _, player := range players[1:] {
		value := valueFn(player.stats)

		switch {
		case value == bestValue:
			winners = append(winners, player.entry.Name)
		case lowerIsBetter && value < bestValue:
			bestValue = value
			winners = []string{player.entry.Name}
		case !lowerIsBetter && value > bestValue:
			bestValue = value
			winners = []string{player.entry.Name}
		}
	}

	if len(winners) == 1 {
		return winners[0]
	}

	slices.Sort(winners)
	return "Tie (" + strings.Join(winners, " / ") + ")"
}
