package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

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

func newFortniteAPISeasonProvider(apiToken string) *fortniteAPISeasonProvider {
	return &fortniteAPISeasonProvider{
		token: apiToken,
		client: &http.Client{
			Timeout: 15 * time.Second,
		},
		url: fortniteAPISeasonURL,
		now: func() time.Time {
			return time.Now().UTC()
		},
	}
}

func newEpicStatusProvider() *epicStatusProvider {
	return &epicStatusProvider{
		client: &http.Client{
			Timeout: 15 * time.Second,
		},
		url: epicStatusSummaryURL,
	}
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
	p.cacheMu.Lock()
	defer p.cacheMu.Unlock()

	cached, ok := p.cache[cacheKey]
	if !ok {
		return playerSnapshot{}, false
	}
	if time.Now().Before(cached.expiresAt) {
		return cached.snapshot, true
	}

	delete(p.cache, cacheKey)
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

func (p *fortniteAPISeasonProvider) DaysLeft() (int, error) {
	req, err := http.NewRequest(http.MethodGet, p.url, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("x-api-key", p.token)

	resp, err := p.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("request season data: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("fortnite api 2 returned %s", resp.Status)
	}

	var payload fortniteSeasonResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return 0, fmt.Errorf("decode season data: %w", err)
	}

	seasonDateEnd := strings.TrimSpace(payload.SeasonDateEnd)
	if seasonDateEnd == "" {
		return 0, errors.New("seasonDateEnd is missing")
	}

	endTime, err := time.Parse(time.RFC3339, seasonDateEnd)
	if err != nil {
		return 0, fmt.Errorf("parse seasonDateEnd: %w", err)
	}

	return daysLeftUntil(p.now(), endTime), nil
}

func (p *epicStatusProvider) Summary() (fortniteStatusSummary, error) {
	req, err := http.NewRequest(http.MethodGet, p.url, nil)
	if err != nil {
		return fortniteStatusSummary{}, err
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return fortniteStatusSummary{}, fmt.Errorf("request status data: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fortniteStatusSummary{}, fmt.Errorf("epic status returned %s", resp.Status)
	}

	var payload epicStatusSummaryResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return fortniteStatusSummary{}, fmt.Errorf("decode status data: %w", err)
	}

	summary, err := extractFortniteStatusSummary(payload)
	if err != nil {
		return fortniteStatusSummary{}, fmt.Errorf("parse status data: %w", err)
	}

	return summary, nil
}

func extractFortniteStatusSummary(payload epicStatusSummaryResponse) (fortniteStatusSummary, error) {
	componentsByID := make(map[string]epicStatusComponent, len(payload.Components))
	var (
		fortnite epicStatusComponent
		found    bool
	)

	for _, component := range payload.Components {
		componentsByID[component.ID] = component
		if strings.EqualFold(strings.TrimSpace(component.Name), "Fortnite") {
			if component.Group || !found {
				fortnite = component
				found = true
			}
		}
	}

	if !found {
		return fortniteStatusSummary{}, errors.New("fortnite component is missing")
	}

	services := make([]fortniteServiceStatus, 0, len(fortnite.Components))
	seen := make(map[string]struct{}, len(fortnite.Components))
	for _, componentID := range fortnite.Components {
		component, ok := componentsByID[componentID]
		if !ok {
			continue
		}

		services = append(services, fortniteServiceStatus{
			Name:   strings.TrimSpace(component.Name),
			Status: humanizeStatus(component.Status),
		})
		seen[componentID] = struct{}{}
	}

	if len(services) == 0 {
		for _, component := range payload.Components {
			if component.GroupID != fortnite.ID {
				continue
			}
			if _, exists := seen[component.ID]; exists {
				continue
			}

			services = append(services, fortniteServiceStatus{
				Name:   strings.TrimSpace(component.Name),
				Status: humanizeStatus(component.Status),
			})
		}
	}

	return fortniteStatusSummary{
		Epic:     strings.TrimSpace(payload.Status.Description),
		Fortnite: humanizeStatus(fortnite.Status),
		Services: services,
	}, nil
}

func daysLeftUntil(now, seasonDateEnd time.Time) int {
	now = now.UTC()
	seasonDateEnd = seasonDateEnd.UTC()
	if !seasonDateEnd.After(now) {
		return 0
	}

	remaining := seasonDateEnd.Sub(now)
	days := int(remaining / (24 * time.Hour))
	if remaining%(24*time.Hour) != 0 {
		days++
	}
	if days < 1 {
		return 1
	}
	return days
}

func extractDailyStats(s statLine) dailyStatLine {
	return dailyStatLine{
		Wins:          s.Wins,
		Top3:          s.Top3,
		Top5:          s.Top5,
		Top6:          s.Top6,
		Top10:         s.Top10,
		Top12:         s.Top12,
		Top25:         s.Top25,
		Kills:         s.Kills,
		Deaths:        s.Deaths,
		Matches:       s.Matches,
		MinutesPlayed: s.MinutesPlayed,
	}
}

func humanizeStatus(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}

	words := strings.Fields(strings.ReplaceAll(value, "_", " "))
	for i, word := range words {
		words[i] = strings.ToUpper(word[:1]) + strings.ToLower(word[1:])
	}

	return strings.Join(words, " ")
}
