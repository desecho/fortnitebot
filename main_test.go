package main

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type stubStatsProvider struct{}

func (stubStatsProvider) Names() []string {
	return nil
}

func (stubStatsProvider) Entries() []playerCatalogEntry {
	return nil
}

func (stubStatsProvider) Count() int {
	return 0
}

func (stubStatsProvider) Lookup(name string) (playerCatalogEntry, bool) {
	return playerCatalogEntry{}, false
}

func (stubStatsProvider) Fetch(entry playerCatalogEntry) (playerSnapshot, error) {
	return playerSnapshot{}, nil
}

func (stubStatsProvider) FetchSeason(entry playerCatalogEntry) (playerSnapshot, error) {
	return playerSnapshot{}, nil
}

type stubSeasonProvider struct {
	days int
	err  error
}

func (p stubSeasonProvider) DaysLeft() (int, error) {
	return p.days, p.err
}

type stubStatusProvider struct {
	summary fortniteStatusSummary
	err     error
}

func (p stubStatusProvider) Summary() (fortniteStatusSummary, error) {
	return p.summary, p.err
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func newTestHTTPClient(fn roundTripFunc) *http.Client {
	return &http.Client{Transport: fn}
}

func newTestResponse(req *http.Request, status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Status:     fmt.Sprintf("%d %s", status, http.StatusText(status)),
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    req,
	}
}

func TestLoadConfigIncludesSecondToken(t *testing.T) {
	t.Setenv("TELEGRAM_BOT_TOKEN", "telegram-token")
	t.Setenv("FORTNITE_API_TOKEN", "fortnite-token")
	t.Setenv("FORTNITE_API2_TOKEN", "fortnite-token-2")
	t.Setenv("PLAYERS_FILE", "")
	t.Setenv("POLL_TIMEOUT_SECS", "")

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig() error = %v", err)
	}

	if cfg.botToken != "telegram-token" {
		t.Fatalf("cfg.botToken = %q, want %q", cfg.botToken, "telegram-token")
	}
	if cfg.fortniteAPIToken != "fortnite-token" {
		t.Fatalf("cfg.fortniteAPIToken = %q, want %q", cfg.fortniteAPIToken, "fortnite-token")
	}
	if cfg.fortniteAPI2Token != "fortnite-token-2" {
		t.Fatalf("cfg.fortniteAPI2Token = %q, want %q", cfg.fortniteAPI2Token, "fortnite-token-2")
	}
	if cfg.playersFile != defaultPlayersFile {
		t.Fatalf("cfg.playersFile = %q, want %q", cfg.playersFile, defaultPlayersFile)
	}
	if cfg.pollTimeoutSecs != defaultPollTimeout {
		t.Fatalf("cfg.pollTimeoutSecs = %d, want %d", cfg.pollTimeoutSecs, defaultPollTimeout)
	}
}

func TestLoadConfigRequiresSecondToken(t *testing.T) {
	t.Setenv("TELEGRAM_BOT_TOKEN", "telegram-token")
	t.Setenv("FORTNITE_API_TOKEN", "fortnite-token")
	t.Setenv("FORTNITE_API2_TOKEN", "")

	_, err := loadConfig()
	if err == nil {
		t.Fatal("loadConfig() error = nil, want error")
	}
	if err.Error() != "FORTNITE_API2_TOKEN is required" {
		t.Fatalf("loadConfig() error = %q, want %q", err.Error(), "FORTNITE_API2_TOKEN is required")
	}
}

func TestHandleMessageSeasonRoute(t *testing.T) {
	got := handleMessage(stubStatsProvider{}, stubSeasonProvider{days: 5}, stubStatusProvider{}, "/season")
	want := "Season ends in 5 days."
	if got != want {
		t.Fatalf("handleMessage() = %q, want %q", got, want)
	}
}

func TestHandleMessageStatusRoute(t *testing.T) {
	got := handleMessage(
		stubStatsProvider{},
		stubSeasonProvider{},
		stubStatusProvider{
			summary: fortniteStatusSummary{
				Epic:     "All Systems Operational",
				Fortnite: "Operational",
				Services: []fortniteServiceStatus{
					{Name: "Game Services", Status: "Operational"},
					{Name: "Matchmaking", Status: "Operational"},
				},
			},
		},
		"/status",
	)

	want := strings.Join([]string{
		"Fortnite status",
		"Epic overall: All Systems Operational",
		"Fortnite overall: Operational",
		"Services:",
		"Game Services: Operational",
		"Matchmaking: Operational",
	}, "\n")

	if got != want {
		t.Fatalf("handleMessage() = %q, want %q", got, want)
	}
}

func TestSeasonText(t *testing.T) {
	tests := []struct {
		name     string
		provider stubSeasonProvider
		want     string
	}{
		{
			name:     "singular",
			provider: stubSeasonProvider{days: 1},
			want:     "Season ends in 1 day.",
		},
		{
			name:     "plural",
			provider: stubSeasonProvider{days: 3},
			want:     "Season ends in 3 days.",
		},
		{
			name:     "ended",
			provider: stubSeasonProvider{days: 0},
			want:     "The current season has ended.",
		},
		{
			name:     "error",
			provider: stubSeasonProvider{err: errors.New("boom")},
			want:     "Failed to fetch season info: boom",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := seasonText(tt.provider); got != tt.want {
				t.Fatalf("seasonText() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestStatusTextError(t *testing.T) {
	got := statusText(stubStatusProvider{err: errors.New("boom")})
	want := "Failed to fetch Fortnite status: boom"
	if got != want {
		t.Fatalf("statusText() = %q, want %q", got, want)
	}
}

func TestDaysLeftUntil(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name string
		end  time.Time
		want int
	}{
		{
			name: "partial day rounds up",
			end:  now.Add(23 * time.Hour),
			want: 1,
		},
		{
			name: "exact days stay exact",
			end:  now.Add(48 * time.Hour),
			want: 2,
		},
		{
			name: "past season returns zero",
			end:  now.Add(-1 * time.Hour),
			want: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := daysLeftUntil(now, tt.end); got != tt.want {
				t.Fatalf("daysLeftUntil() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestFortniteAPISeasonProviderDaysLeft(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	var (
		sawHeader     bool
		receivedToken string
	)

	client := newTestHTTPClient(func(r *http.Request) (*http.Response, error) {
		sawHeader = true
		receivedToken = r.Header.Get("x-api-key")
		return newTestResponse(r, http.StatusOK, `{"seasonDateEnd":"2026-01-02T12:00:00Z"}`), nil
	})

	provider := &fortniteAPISeasonProvider{
		token:  "secret-2",
		client: client,
		url:    "http://example.invalid/season",
		now: func() time.Time {
			return now
		},
	}

	days, err := provider.DaysLeft()
	if err != nil {
		t.Fatalf("DaysLeft() error = %v", err)
	}
	if !sawHeader {
		t.Fatal("server did not receive a request")
	}
	if receivedToken != "secret-2" {
		t.Fatalf("x-api-key = %q, want %q", receivedToken, "secret-2")
	}
	if days != 2 {
		t.Fatalf("DaysLeft() = %d, want %d", days, 2)
	}
}

func TestFortniteAPISeasonProviderDaysLeftErrors(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name    string
		status  int
		body    string
		wantErr string
	}{
		{
			name:    "non-200 response",
			status:  http.StatusBadGateway,
			body:    `{"error":"bad gateway"}`,
			wantErr: "fortnite api 2 returned 502 Bad Gateway",
		},
		{
			name:    "invalid json",
			status:  http.StatusOK,
			body:    `{"seasonDateEnd":`,
			wantErr: "decode season data:",
		},
		{
			name:    "missing end date",
			status:  http.StatusOK,
			body:    `{}`,
			wantErr: "seasonDateEnd is missing",
		},
		{
			name:    "invalid end date",
			status:  http.StatusOK,
			body:    `{"seasonDateEnd":"not-a-date"}`,
			wantErr: "parse seasonDateEnd:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := newTestHTTPClient(func(r *http.Request) (*http.Response, error) {
				return newTestResponse(r, tt.status, tt.body), nil
			})

			provider := &fortniteAPISeasonProvider{
				token:  "secret-2",
				client: client,
				url:    "http://example.invalid/season",
				now: func() time.Time {
					return now
				},
			}

			_, err := provider.DaysLeft()
			if err == nil {
				t.Fatal("DaysLeft() error = nil, want error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("DaysLeft() error = %q, want substring %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestEpicStatusProviderSummary(t *testing.T) {
	body := `{
		"status": {"description": "All Systems Operational"},
		"components": [
			{"id": "fn", "name": "Fortnite", "status": "operational", "group": true, "group_id": "", "components": ["fn-1","fn-2","fn-3","fn-4","fn-5","fn-6","fn-7","fn-8","fn-9"]},
			{"id": "fn-1", "name": "Website", "status": "operational", "group": false, "group_id": "fn", "components": []},
			{"id": "fn-2", "name": "Game Services", "status": "operational", "group": false, "group_id": "fn", "components": []},
			{"id": "fn-3", "name": "Login", "status": "operational", "group": false, "group_id": "fn", "components": []},
			{"id": "fn-4", "name": "Parties, Friends, and Messaging", "status": "operational", "group": false, "group_id": "fn", "components": []},
			{"id": "fn-5", "name": "Voice Chat", "status": "operational", "group": false, "group_id": "fn", "components": []},
			{"id": "fn-6", "name": "Matchmaking", "status": "operational", "group": false, "group_id": "fn", "components": []},
			{"id": "fn-7", "name": "Stats and Leaderboards", "status": "operational", "group": false, "group_id": "fn", "components": []},
			{"id": "fn-8", "name": "Item Shop", "status": "operational", "group": false, "group_id": "fn", "components": []},
			{"id": "fn-9", "name": "Fortnite Crew", "status": "operational", "group": false, "group_id": "fn", "components": []}
		]
	}`

	client := newTestHTTPClient(func(r *http.Request) (*http.Response, error) {
		return newTestResponse(r, http.StatusOK, body), nil
	})

	provider := &epicStatusProvider{
		client: client,
		url:    "http://example.invalid/status",
	}

	summary, err := provider.Summary()
	if err != nil {
		t.Fatalf("Summary() error = %v", err)
	}

	if summary.Epic != "All Systems Operational" {
		t.Fatalf("summary.Epic = %q, want %q", summary.Epic, "All Systems Operational")
	}
	if summary.Fortnite != "Operational" {
		t.Fatalf("summary.Fortnite = %q, want %q", summary.Fortnite, "Operational")
	}

	wantNames := []string{
		"Website",
		"Game Services",
		"Login",
		"Parties, Friends, and Messaging",
		"Voice Chat",
		"Matchmaking",
		"Stats and Leaderboards",
		"Item Shop",
		"Fortnite Crew",
	}

	if len(summary.Services) != len(wantNames) {
		t.Fatalf("len(summary.Services) = %d, want %d", len(summary.Services), len(wantNames))
	}

	for i, wantName := range wantNames {
		if summary.Services[i].Name != wantName {
			t.Fatalf("summary.Services[%d].Name = %q, want %q", i, summary.Services[i].Name, wantName)
		}
		if summary.Services[i].Status != "Operational" {
			t.Fatalf("summary.Services[%d].Status = %q, want %q", i, summary.Services[i].Status, "Operational")
		}
	}
}
