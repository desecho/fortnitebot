package main

import (
	"net/http"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
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
	FetchFresh(entry playerCatalogEntry) (playerSnapshot, error)
	FetchSeason(entry playerCatalogEntry) (playerSnapshot, error)
}

type seasonProvider interface {
	DaysLeft() (int, error)
}

type statusProvider interface {
	Summary() (fortniteStatusSummary, error)
}

type botClient interface {
	getUpdates(offset int, timeoutSecs int) ([]tgbotapi.Update, error)
	sendMessage(chatID int64, text string) error
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
	Top3          int64   `json:"top3"`
	Top5          int64   `json:"top5"`
	Top6          int64   `json:"top6"`
	Top10         int64   `json:"top10"`
	Top12         int64   `json:"top12"`
	Top25         int64   `json:"top25"`
}

type dailyStatLine struct {
	Wins          int64 `bson:"wins"          json:"wins"`
	Top3          int64 `bson:"top3"          json:"top3"`
	Top5          int64 `bson:"top5"          json:"top5"`
	Top6          int64 `bson:"top6"          json:"top6"`
	Top10         int64 `bson:"top10"         json:"top10"`
	Top12         int64 `bson:"top12"         json:"top12"`
	Top25         int64 `bson:"top25"         json:"top25"`
	Kills         int64 `bson:"kills"         json:"kills"`
	Deaths        int64 `bson:"deaths"        json:"deaths"`
	Matches       int64 `bson:"matches"       json:"matches"`
	MinutesPlayed int64 `bson:"minutesPlayed" json:"minutesPlayed"`
}

type dailySnapshot struct {
	AccountID string        `bson:"accountId"  json:"accountId"`
	Name      string        `bson:"name"       json:"name"`
	Date      string        `bson:"date"       json:"date"`
	Stats     dailyStatLine `bson:"stats"      json:"stats"`
	CreatedAt time.Time     `bson:"createdAt"  json:"createdAt"`
}

type sessionSummary struct {
	PlayerName    string
	Date          string
	Delta         dailyStatLine
	KillsPerMatch float64
	KD            float64
	WinRate       float64
}

type snapshotStore interface {
	UpsertSnapshot(snapshot dailySnapshot) error
	RecentSnapshots(accountID string, limit int) ([]dailySnapshot, error)
}
