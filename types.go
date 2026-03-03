package main

import (
	"net/http"
	"sync"
	"time"
)

const (
	defaultPlayersFile   = "players.json"
	defaultPollTimeout   = 30
	fortniteAPIBaseURL   = "https://fortnite-api.com/v2/stats/br/v2"
	fortniteAPISeasonURL = "https://prod.api-fortnite.com/api/v1/season"
	epicStatusSummaryURL = "https://status.epicgames.com/api/v2/summary.json"
	statsCacheTTL        = 1 * time.Hour
)

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

type seasonProvider interface {
	DaysLeft() (int, error)
}

type statusProvider interface {
	Summary() (fortniteStatusSummary, error)
}

type fortniteAPIStatsProvider struct {
	order   []string
	players map[string]playerCatalogEntry
	token   string
	client  *http.Client
	cache   map[string]cachedSnapshot
	cacheMu sync.Mutex
}

type fortniteAPISeasonProvider struct {
	token  string
	client *http.Client
	url    string
	now    func() time.Time
}

type epicStatusProvider struct {
	client *http.Client
	url    string
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

type fortniteSeasonResponse struct {
	SeasonDateEnd string `json:"seasonDateEnd"`
}

type fortniteStatusSummary struct {
	Epic     string
	Fortnite string
	Services []fortniteServiceStatus
}

type fortniteServiceStatus struct {
	Name   string
	Status string
}

type epicStatusSummaryResponse struct {
	Components []epicStatusComponent `json:"components"`
	Status     struct {
		Description string `json:"description"`
	} `json:"status"`
}

type epicStatusComponent struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Status     string   `json:"status"`
	Group      bool     `json:"group"`
	GroupID    string   `json:"group_id"`
	Components []string `json:"components"`
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
