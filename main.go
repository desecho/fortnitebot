package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultPlayersFile = "players.json"
	defaultPollTimeout = 30
	fortniteAPIBaseURL = "https://fortnite-api.com/v2/stats/br/v2"
	statsCacheTTL      = 1 * time.Hour
)

type appConfig struct {
	botToken         string
	fortniteAPIToken string
	playersFile      string
	pollTimeoutSecs  int
}

type playerCatalogEntry struct {
	Name      string `json:"name"`
	AccountID string `json:"accountId"`
}

type playerSnapshot struct {
	entry playerCatalogEntry
	stats statLine
}

type fetchResult struct {
	entry    playerCatalogEntry
	snapshot playerSnapshot
	err      error
}

type cachedSnapshot struct {
	snapshot  playerSnapshot
	expiresAt time.Time
}

type statsProvider interface {
	Names() []string
	Entries() []playerCatalogEntry
	Count() int
	Lookup(name string) (playerCatalogEntry, bool)
	Fetch(entry playerCatalogEntry) (playerSnapshot, error)
	FetchSeason(entry playerCatalogEntry) (playerSnapshot, error)
}

type fortniteAPIStatsProvider struct {
	order   []string
	players map[string]playerCatalogEntry
	token   string
	client  *http.Client
	cache   map[string]cachedSnapshot
	cacheMu sync.RWMutex
}

type fortniteStatsResponse struct {
	Status int `json:"status"`
	Data   struct {
		Stats struct {
			All struct {
				Overall statLine `json:"overall"`
			} `json:"all"`
		} `json:"stats"`
	} `json:"data"`
}

type statLine struct {
	Wins          int64   `json:"wins"`
	Kills         int64   `json:"kills"`
	KillsPerMatch float64 `json:"killsPerMatch"`
	Deaths        int64   `json:"deaths"`
	KD            float64 `json:"kd"`
	Matches       int64   `json:"matches"`
	WinRate       float64 `json:"winRate"`
	MinutesPlayed int64   `json:"minutesPlayed"`
}

type telegramClient struct {
	baseURL    string
	httpClient *http.Client
}

type telegramUpdateEnvelope struct {
	OK          bool             `json:"ok"`
	Result      []telegramUpdate `json:"result"`
	Description string           `json:"description"`
}

type telegramResultEnvelope struct {
	OK          bool   `json:"ok"`
	Description string `json:"description"`
}

type telegramUpdate struct {
	UpdateID int64            `json:"update_id"`
	Message  *telegramMessage `json:"message"`
}

type telegramMessage struct {
	MessageID int64        `json:"message_id"`
	Text      string       `json:"text"`
	Chat      telegramChat `json:"chat"`
}

type telegramChat struct {
	ID int64 `json:"id"`
}

type telegramSendMessageRequest struct {
	ChatID string `json:"chat_id"`
	Text   string `json:"text"`
}

func main() {
	log.SetFlags(0)

	cfg, err := loadConfig()
	if err != nil {
		log.Fatal(err)
	}

	provider, err := newFortniteAPIStatsProvider(cfg.playersFile, cfg.fortniteAPIToken)
	if err != nil {
		log.Fatal(err)
	}

	client := newTelegramClient(cfg.botToken)
	if err := runBot(client, provider, cfg.pollTimeoutSecs); err != nil {
		log.Fatal(err)
	}
}

func loadConfig() (appConfig, error) {
	token := strings.TrimSpace(os.Getenv("TELEGRAM_BOT_TOKEN"))
	if token == "" {
		return appConfig{}, errors.New("TELEGRAM_BOT_TOKEN is required")
	}

	fortniteToken := strings.TrimSpace(os.Getenv("FORTNITE_API_TOKEN"))
	if fortniteToken == "" {
		return appConfig{}, errors.New("FORTNITE_API_TOKEN is required")
	}

	playersFile := strings.TrimSpace(os.Getenv("PLAYERS_FILE"))
	if playersFile == "" {
		playersFile = defaultPlayersFile
	}

	pollTimeout := defaultPollTimeout
	if raw := strings.TrimSpace(os.Getenv("POLL_TIMEOUT_SECS")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > 60 {
			return appConfig{}, fmt.Errorf("POLL_TIMEOUT_SECS must be an integer between 1 and 60")
		}
		pollTimeout = value
	}

	return appConfig{
		botToken:         token,
		fortniteAPIToken: fortniteToken,
		playersFile:      playersFile,
		pollTimeoutSecs:  pollTimeout,
	}, nil
}

func newFortniteAPIStatsProvider(playersFile, apiToken string) (*fortniteAPIStatsProvider, error) {
	data, err := os.ReadFile(playersFile)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", playersFile, err)
	}

	var entries []playerCatalogEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("parse %s: %w", playersFile, err)
	}

	if len(entries) == 0 {
		return nil, fmt.Errorf("%s must contain at least one player", playersFile)
	}

	provider := &fortniteAPIStatsProvider{
		order:   make([]string, 0, len(entries)),
		players: make(map[string]playerCatalogEntry, len(entries)),
		token:   apiToken,
		client: &http.Client{
			Timeout: 15 * time.Second,
		},
		cache: make(map[string]cachedSnapshot, len(entries)*2),
	}

	for _, entry := range entries {
		displayName := strings.TrimSpace(entry.Name)
		nameKey := strings.ToLower(displayName)
		if nameKey == "" {
			return nil, fmt.Errorf("%s contains a player entry without a name", playersFile)
		}
		if _, exists := provider.players[nameKey]; exists {
			return nil, fmt.Errorf("duplicate player name %q in %s", displayName, playersFile)
		}
		accountID := strings.TrimSpace(entry.AccountID)
		if accountID == "" {
			return nil, fmt.Errorf("player %q is missing an accountId", displayName)
		}

		entry.Name = displayName
		entry.AccountID = accountID
		provider.order = append(provider.order, nameKey)
		provider.players[nameKey] = entry
	}

	return provider, nil
}

func (p *fortniteAPIStatsProvider) Names() []string {
	names := make([]string, 0, len(p.order))
	for _, name := range p.order {
		player, ok := p.players[name]
		if !ok {
			continue
		}
		names = append(names, player.Name)
	}
	return names
}

func (p *fortniteAPIStatsProvider) Entries() []playerCatalogEntry {
	entries := make([]playerCatalogEntry, 0, len(p.order))
	for _, name := range p.order {
		entry, ok := p.players[name]
		if ok {
			entries = append(entries, entry)
		}
	}
	return entries
}

func (p *fortniteAPIStatsProvider) Count() int {
	return len(p.order)
}

func (p *fortniteAPIStatsProvider) Lookup(name string) (playerCatalogEntry, bool) {
	player, ok := p.players[strings.ToLower(strings.TrimSpace(name))]
	return player, ok
}

func (p *fortniteAPIStatsProvider) Fetch(entry playerCatalogEntry) (playerSnapshot, error) {
	return p.fetch(entry, "")
}

func (p *fortniteAPIStatsProvider) FetchSeason(entry playerCatalogEntry) (playerSnapshot, error) {
	return p.fetch(entry, "season")
}

func (p *fortniteAPIStatsProvider) fetch(entry playerCatalogEntry, timeWindow string) (playerSnapshot, error) {
	cacheKey := p.cacheKey(entry, timeWindow)
	if snapshot, ok := p.cachedSnapshot(cacheKey); ok {
		return snapshot, nil
	}

	requestURL := fortniteAPIBaseURL + "/" + url.PathEscape(entry.AccountID)
	if strings.TrimSpace(timeWindow) != "" {
		values := url.Values{}
		values.Set("timeWindow", timeWindow)
		requestURL += "?" + values.Encode()
	}

	req, err := http.NewRequest(http.MethodGet, requestURL, nil)
	if err != nil {
		return playerSnapshot{}, err
	}
	req.Header.Set("Authorization", p.token)

	resp, err := p.client.Do(req)
	if err != nil {
		return playerSnapshot{}, fmt.Errorf("request stats for %s: %w", entry.Name, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return playerSnapshot{}, fmt.Errorf("fortnite api returned %s for %s", resp.Status, entry.Name)
	}

	var payload fortniteStatsResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return playerSnapshot{}, fmt.Errorf("decode stats for %s: %w", entry.Name, err)
	}
	if payload.Status != 200 {
		return playerSnapshot{}, fmt.Errorf("fortnite api payload status %d for %s", payload.Status, entry.Name)
	}

	snapshot := playerSnapshot{
		entry: entry,
		stats: payload.Data.Stats.All.Overall,
	}
	p.storeCachedSnapshot(cacheKey, snapshot)

	return snapshot, nil
}

func (p *fortniteAPIStatsProvider) cacheKey(entry playerCatalogEntry, timeWindow string) string {
	return entry.AccountID + "|" + strings.TrimSpace(timeWindow)
}

func (p *fortniteAPIStatsProvider) cachedSnapshot(cacheKey string) (playerSnapshot, bool) {
	now := time.Now()

	p.cacheMu.RLock()
	cached, ok := p.cache[cacheKey]
	p.cacheMu.RUnlock()
	if !ok {
		return playerSnapshot{}, false
	}
	if now.Before(cached.expiresAt) {
		return cached.snapshot, true
	}

	p.cacheMu.Lock()
	cached, ok = p.cache[cacheKey]
	if ok && !now.Before(cached.expiresAt) {
		delete(p.cache, cacheKey)
	}
	p.cacheMu.Unlock()

	return playerSnapshot{}, false
}

func (p *fortniteAPIStatsProvider) storeCachedSnapshot(cacheKey string, snapshot playerSnapshot) {
	p.cacheMu.Lock()
	p.cache[cacheKey] = cachedSnapshot{
		snapshot:  snapshot,
		expiresAt: time.Now().Add(statsCacheTTL),
	}
	p.cacheMu.Unlock()
}

func newTelegramClient(token string) *telegramClient {
	return &telegramClient{
		baseURL: "https://api.telegram.org/bot" + token,
		httpClient: &http.Client{
			Timeout: 90 * time.Second,
		},
	}
}

func runBot(client *telegramClient, provider statsProvider, pollTimeout int) error {
	var offset int64

	log.Printf("Bot is running with %d configured player(s).", provider.Count())

	for {
		updates, err := client.getUpdates(offset, pollTimeout)
		if err != nil {
			log.Printf("poll failed: %v", err)
			time.Sleep(3 * time.Second)
			continue
		}

		for _, update := range updates {
			offset = update.UpdateID + 1
			if update.Message == nil {
				continue
			}

			response := handleMessage(provider, update.Message.Text)
			if strings.TrimSpace(response) == "" {
				continue
			}

			if err := client.sendMessage(update.Message.Chat.ID, response); err != nil {
				log.Printf("send message failed: %v", err)
			}
		}
	}
}

func (c *telegramClient) getUpdates(offset int64, timeoutSecs int) ([]telegramUpdate, error) {
	query := url.Values{}
	query.Set("offset", strconv.FormatInt(offset, 10))
	query.Set("timeout", strconv.Itoa(timeoutSecs))

	req, err := http.NewRequest(http.MethodGet, c.baseURL+"/getUpdates?"+query.Encode(), nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("telegram returned %s", resp.Status)
	}

	var payload telegramUpdateEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	if !payload.OK {
		return nil, fmt.Errorf("telegram error: %s", payload.Description)
	}

	return payload.Result, nil
}

func (c *telegramClient) sendMessage(chatID int64, text string) error {
	body, err := json.Marshal(telegramSendMessageRequest{
		ChatID: strconv.FormatInt(chatID, 10),
		Text:   text,
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, c.baseURL+"/sendMessage", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("telegram returned %s", resp.Status)
	}

	var payload telegramResultEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return err
	}
	if !payload.OK {
		return fmt.Errorf("telegram error: %s", payload.Description)
	}

	return nil
}

func handleMessage(provider statsProvider, text string) string {
	fields := strings.Fields(strings.TrimSpace(text))
	if len(fields) == 0 {
		return ""
	}

	command := normalizeCommand(fields[0])
	args := fields[1:]

	switch command {
	case "/start", "/help":
		return helpText(provider)
	case "/players":
		return playersText(provider)
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

func helpText(provider statsProvider) string {
	lines := []string{
		"Fortnite stats bot",
		"",
		"Commands:",
		"/players",
		"/stats [player]",
		"/seasonstats [player]",
		"/compare <player1> <player2>",
		"/seasoncompare <player1> <player2>",
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
	if len(args) != 2 {
		if season {
			return "Usage: /seasoncompare <player1> <player2>"
		}
		return "Usage: /compare <player1> <player2>"
	}

	leftPlayer, ok := provider.Lookup(args[0])
	if !ok {
		return fmt.Sprintf("Unknown player %q. Use /players to see the configured player names.", args[0])
	}

	rightPlayer, ok := provider.Lookup(args[1])
	if !ok {
		return fmt.Sprintf("Unknown player %q. Use /players to see the configured player names.", args[1])
	}

	results := fetchStatsBatch(provider, []playerCatalogEntry{leftPlayer, rightPlayer}, season)
	if results[0].err != nil {
		return fmt.Sprintf("Failed to fetch stats for %s: %v", leftPlayer.Name, results[0].err)
	}
	if results[1].err != nil {
		return fmt.Sprintf("Failed to fetch stats for %s: %v", rightPlayer.Name, results[1].err)
	}

	leftStats := results[0].snapshot.stats
	rightStats := results[1].snapshot.stats
	leftName := playerLabel(results[0].snapshot)
	rightName := playerLabel(results[1].snapshot)

	lines := []string{
		compareTitle(season),
		fmt.Sprintf("%s: %s", leftName, formatInlineStats(leftStats)),
		fmt.Sprintf("%s: %s", rightName, formatInlineStats(rightStats)),
		fmt.Sprintf("Wins leader: %s", winnerLabel(leftName, rightName, float64(leftStats.Wins), float64(rightStats.Wins), false)),
		fmt.Sprintf("Kills leader: %s", winnerLabel(leftName, rightName, float64(leftStats.Kills), float64(rightStats.Kills), false)),
		fmt.Sprintf("Kills/match leader: %s", winnerLabel(leftName, rightName, leftStats.KillsPerMatch, rightStats.KillsPerMatch, false)),
		fmt.Sprintf("Lower deaths: %s", winnerLabel(leftName, rightName, float64(leftStats.Deaths), float64(rightStats.Deaths), true)),
		fmt.Sprintf("KD leader: %s", winnerLabel(leftName, rightName, leftStats.KD, rightStats.KD, false)),
		fmt.Sprintf("Matches leader: %s", winnerLabel(leftName, rightName, float64(leftStats.Matches), float64(rightStats.Matches), false)),
		fmt.Sprintf("Win rate leader: %s", winnerLabel(leftName, rightName, leftStats.WinRate, rightStats.WinRate, false)),
		fmt.Sprintf("Hours played leader: %s", winnerLabel(leftName, rightName, float64(leftStats.MinutesPlayed), float64(rightStats.MinutesPlayed), false)),
	}

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

func formatInlineStats(line statLine) string {
	return fmt.Sprintf(
		"%d wins, %d kills, %.2f kills/match, %d deaths, %.2f KD, %d matches, %.2f%% win rate, %.2f hours",
		line.Wins,
		line.Kills,
		line.KillsPerMatch,
		line.Deaths,
		line.KD,
		line.Matches,
		line.WinRate,
		hoursPlayed(line.MinutesPlayed),
	)
}

func hoursPlayed(minutes int64) float64 {
	return float64(minutes) / 60
}

func playerLabel(player playerSnapshot) string {
	return player.entry.Name
}

func winnerLabel(leftName, rightName string, leftValue, rightValue float64, lowerIsBetter bool) string {
	switch {
	case leftValue == rightValue:
		pair := []string{leftName, rightName}
		slices.Sort(pair)
		return "Tie (" + strings.Join(pair, " / ") + ")"
	case lowerIsBetter && leftValue < rightValue:
		return leftName
	case lowerIsBetter && rightValue < leftValue:
		return rightName
	case leftValue > rightValue:
		return leftName
	case rightValue > leftValue:
		return rightName
	default:
		pair := []string{leftName, rightName}
		slices.Sort(pair)
		return "Tie (" + strings.Join(pair, " / ") + ")"
	}
}
