package main

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

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

func TestNewFortniteAPIStatsProvider(t *testing.T) {
	tests := []struct {
		name    string
		json    string
		wantErr string
	}{
		{
			name: "valid file",
			json: `[{"name":"Alice","accountId":"a1"}]`,
		},
		{
			name:    "empty array",
			json:    `[]`,
			wantErr: "must contain at least one player",
		},
		{
			name:    "invalid json",
			json:    `[{"name":}]`,
			wantErr: "parse",
		},
		{
			name:    "missing name",
			json:    `[{"name":"","accountId":"a1"}]`,
			wantErr: "without a name",
		},
		{
			name:    "whitespace only name",
			json:    `[{"name":"  ","accountId":"a1"}]`,
			wantErr: "without a name",
		},
		{
			name:    "missing accountId",
			json:    `[{"name":"Alice","accountId":""}]`,
			wantErr: "missing an accountId",
		},
		{
			name:    "duplicate names",
			json:    `[{"name":"Alice","accountId":"a1"},{"name":"Alice","accountId":"a2"}]`,
			wantErr: "duplicate player name",
		},
		{
			name:    "case insensitive duplicate",
			json:    `[{"name":"Alice","accountId":"a1"},{"name":"alice","accountId":"a2"}]`,
			wantErr: "duplicate player name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "players.json")
			if err := os.WriteFile(path, []byte(tt.json), 0644); err != nil {
				t.Fatal(err)
			}

			provider, err := newFortniteAPIStatsProvider(path, "test-token")
			if tt.wantErr != "" {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %q, want substring %q", err.Error(), tt.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if provider == nil {
				t.Fatal("provider is nil")
			}
		})
	}
}

func TestNewFortniteAPIStatsProviderFileNotFound(t *testing.T) {
	_, err := newFortniteAPIStatsProvider("/nonexistent/players.json", "test-token")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "read") {
		t.Fatalf("error = %q, want substring %q", err.Error(), "read")
	}
}

func TestFortniteAPIStatsProviderMethods(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "players.json")
	data := `[{"name":"Alice","accountId":"a1"},{"name":"Bob","accountId":"b2"}]`
	if err := os.WriteFile(path, []byte(data), 0644); err != nil {
		t.Fatal(err)
	}

	provider, err := newFortniteAPIStatsProvider(path, "test-token")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("Names", func(t *testing.T) {
		names := provider.Names()
		if len(names) != 2 {
			t.Fatalf("len(Names()) = %d, want 2", len(names))
		}
		if names[0] != "Alice" || names[1] != "Bob" {
			t.Fatalf("Names() = %v, want [Alice Bob]", names)
		}
	})

	t.Run("Entries", func(t *testing.T) {
		entries := provider.Entries()
		if len(entries) != 2 {
			t.Fatalf("len(Entries()) = %d, want 2", len(entries))
		}
		if entries[0].Name != "Alice" || entries[1].Name != "Bob" {
			t.Fatalf("Entries() names = [%s, %s], want [Alice, Bob]", entries[0].Name, entries[1].Name)
		}
	})

	t.Run("Count", func(t *testing.T) {
		if provider.Count() != 2 {
			t.Fatalf("Count() = %d, want 2", provider.Count())
		}
	})

	t.Run("Lookup found", func(t *testing.T) {
		entry, ok := provider.Lookup("Alice")
		if !ok {
			t.Fatal("Lookup(Alice) not found")
		}
		if entry.Name != "Alice" {
			t.Fatalf("entry.Name = %q, want Alice", entry.Name)
		}
	})

	t.Run("Lookup case insensitive", func(t *testing.T) {
		entry, ok := provider.Lookup("alice")
		if !ok {
			t.Fatal("Lookup(alice) not found")
		}
		if entry.Name != "Alice" {
			t.Fatalf("entry.Name = %q, want Alice", entry.Name)
		}
	})

	t.Run("Lookup not found", func(t *testing.T) {
		_, ok := provider.Lookup("Charlie")
		if ok {
			t.Fatal("Lookup(Charlie) found, want not found")
		}
	})

	t.Run("Lookup with whitespace", func(t *testing.T) {
		entry, ok := provider.Lookup("  Alice  ")
		if !ok {
			t.Fatal("Lookup(  Alice  ) not found")
		}
		if entry.Name != "Alice" {
			t.Fatalf("entry.Name = %q, want Alice", entry.Name)
		}
	})
}

func TestHumanizeStatus(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"operational", "Operational"},
		{"UNDER_MAINTENANCE", "Under Maintenance"},
		{"major_outage", "Major Outage"},
		{"", ""},
		{"  operational  ", "Operational"},
		{"PARTIAL_OUTAGE", "Partial Outage"},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("input=%q", tt.input), func(t *testing.T) {
			if got := humanizeStatus(tt.input); got != tt.want {
				t.Fatalf("humanizeStatus(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestExtractFortniteStatusSummaryMissingFortnite(t *testing.T) {
	payload := epicStatusSummaryResponse{
		Components: []epicStatusComponent{
			{ID: "other", Name: "Other Game", Status: "operational"},
		},
	}

	_, err := extractFortniteStatusSummary(payload)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "fortnite component is missing") {
		t.Fatalf("error = %q, want substring %q", err.Error(), "fortnite component is missing")
	}
}

func TestExtractFortniteStatusSummaryGroupIDFallback(t *testing.T) {
	payload := epicStatusSummaryResponse{
		Components: []epicStatusComponent{
			{ID: "fn", Name: "Fortnite", Status: "operational", Group: true, Components: nil},
			{ID: "fn-1", Name: "Login", Status: "operational", GroupID: "fn"},
			{ID: "fn-2", Name: "Matchmaking", Status: "under_maintenance", GroupID: "fn"},
		},
		Status: struct {
			Description string `json:"description"`
		}{Description: "Minor Disruption"},
	}

	summary, err := extractFortniteStatusSummary(payload)
	if err != nil {
		t.Fatalf("error = %v", err)
	}

	if summary.Epic != "Minor Disruption" {
		t.Fatalf("Epic = %q, want Minor Disruption", summary.Epic)
	}
	if len(summary.Services) != 2 {
		t.Fatalf("len(Services) = %d, want 2", len(summary.Services))
	}
	if summary.Services[0].Name != "Login" {
		t.Fatalf("Services[0].Name = %q, want Login", summary.Services[0].Name)
	}
	if summary.Services[1].Status != "Under Maintenance" {
		t.Fatalf("Services[1].Status = %q, want Under Maintenance", summary.Services[1].Status)
	}
}

func TestFortniteAPIStatsProviderCache(t *testing.T) {
	provider := &fortniteAPIStatsProvider{
		cache: make(map[string]cachedSnapshot),
	}

	entry := playerCatalogEntry{Name: "Alice", AccountID: "a1"}
	snapshot := playerSnapshot{entry: entry, stats: statLine{Wins: 10}}
	cacheKey := provider.cacheKey(entry, "")

	t.Run("miss on empty cache", func(t *testing.T) {
		_, ok := provider.cachedSnapshot(cacheKey)
		if ok {
			t.Fatal("expected cache miss")
		}
	})

	t.Run("hit after store", func(t *testing.T) {
		provider.storeCachedSnapshot(cacheKey, snapshot)
		got, ok := provider.cachedSnapshot(cacheKey)
		if !ok {
			t.Fatal("expected cache hit")
		}
		if got.stats.Wins != 10 {
			t.Fatalf("cached Wins = %d, want 10", got.stats.Wins)
		}
	})

	t.Run("miss after expiration", func(t *testing.T) {
		provider.cacheMu.Lock()
		provider.cache[cacheKey] = cachedSnapshot{
			snapshot:  snapshot,
			expiresAt: time.Now().Add(-1 * time.Second),
		}
		provider.cacheMu.Unlock()

		_, ok := provider.cachedSnapshot(cacheKey)
		if ok {
			t.Fatal("expected cache miss after expiration")
		}
	})

	t.Run("different cache keys", func(t *testing.T) {
		seasonKey := provider.cacheKey(entry, "season")
		provider.storeCachedSnapshot(seasonKey, playerSnapshot{entry: entry, stats: statLine{Wins: 5}})

		got, ok := provider.cachedSnapshot(seasonKey)
		if !ok {
			t.Fatal("expected cache hit for season key")
		}
		if got.stats.Wins != 5 {
			t.Fatalf("cached season Wins = %d, want 5", got.stats.Wins)
		}

		// Overall key should still be expired from the previous subtest
		_, ok = provider.cachedSnapshot(cacheKey)
		if ok {
			t.Fatal("expected cache miss for expired overall key")
		}
	})
}

func TestFortniteAPIStatsProviderFetch(t *testing.T) {
	entry := playerCatalogEntry{Name: "Alice", AccountID: "a1"}

	t.Run("success", func(t *testing.T) {
		body := `{"status":200,"data":{"stats":{"all":{"overall":{"wins":10,"kills":50,"killsPerMatch":2.5,"deaths":20,"kd":2.5,"matches":20,"winRate":50.0,"minutesPlayed":600}}}}}`
		var receivedAuth string
		client := newTestHTTPClient(func(r *http.Request) (*http.Response, error) {
			receivedAuth = r.Header.Get("Authorization")
			return newTestResponse(r, http.StatusOK, body), nil
		})

		provider := &fortniteAPIStatsProvider{
			order:   []string{"alice"},
			players: map[string]playerCatalogEntry{"alice": entry},
			token:   "test-token",
			client:  client,
			cache:   make(map[string]cachedSnapshot),
		}

		snapshot, err := provider.Fetch(entry)
		if err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}
		if receivedAuth != "test-token" {
			t.Fatalf("Authorization = %q, want test-token", receivedAuth)
		}
		if snapshot.stats.Wins != 10 {
			t.Fatalf("Wins = %d, want 10", snapshot.stats.Wins)
		}
		if snapshot.stats.KillsPerMatch != 2.5 {
			t.Fatalf("KillsPerMatch = %f, want 2.5", snapshot.stats.KillsPerMatch)
		}
	})

	t.Run("non-200 response", func(t *testing.T) {
		client := newTestHTTPClient(func(r *http.Request) (*http.Response, error) {
			return newTestResponse(r, http.StatusInternalServerError, ""), nil
		})

		provider := &fortniteAPIStatsProvider{
			token:  "test-token",
			client: client,
			cache:  make(map[string]cachedSnapshot),
		}

		_, err := provider.Fetch(entry)
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "500") {
			t.Fatalf("error = %q, want substring '500'", err.Error())
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		client := newTestHTTPClient(func(r *http.Request) (*http.Response, error) {
			return newTestResponse(r, http.StatusOK, `{invalid`), nil
		})

		provider := &fortniteAPIStatsProvider{
			token:  "test-token",
			client: client,
			cache:  make(map[string]cachedSnapshot),
		}

		_, err := provider.Fetch(entry)
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "decode stats") {
			t.Fatalf("error = %q, want substring 'decode stats'", err.Error())
		}
	})

	t.Run("payload status not 200", func(t *testing.T) {
		client := newTestHTTPClient(func(r *http.Request) (*http.Response, error) {
			return newTestResponse(r, http.StatusOK, `{"status":403}`), nil
		})

		provider := &fortniteAPIStatsProvider{
			token:  "test-token",
			client: client,
			cache:  make(map[string]cachedSnapshot),
		}

		_, err := provider.Fetch(entry)
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "payload status 403") {
			t.Fatalf("error = %q, want substring 'payload status 403'", err.Error())
		}
	})

	t.Run("season adds timeWindow query parameter", func(t *testing.T) {
		var requestURL string
		client := newTestHTTPClient(func(r *http.Request) (*http.Response, error) {
			requestURL = r.URL.String()
			return newTestResponse(r, http.StatusOK, `{"status":200,"data":{"stats":{"all":{"overall":{}}}}}`), nil
		})

		provider := &fortniteAPIStatsProvider{
			token:  "test-token",
			client: client,
			cache:  make(map[string]cachedSnapshot),
		}

		_, err := provider.FetchSeason(entry)
		if err != nil {
			t.Fatalf("FetchSeason() error = %v", err)
		}
		if !strings.Contains(requestURL, "timeWindow=season") {
			t.Fatalf("URL = %q, want substring 'timeWindow=season'", requestURL)
		}
	})

	t.Run("cache hit skips HTTP", func(t *testing.T) {
		called := false
		client := newTestHTTPClient(func(r *http.Request) (*http.Response, error) {
			called = true
			return newTestResponse(r, http.StatusOK, ""), nil
		})

		snapshot := playerSnapshot{entry: entry, stats: statLine{Wins: 42}}
		provider := &fortniteAPIStatsProvider{
			token:  "test-token",
			client: client,
			cache:  make(map[string]cachedSnapshot),
		}
		provider.storeCachedSnapshot(provider.cacheKey(entry, ""), snapshot)

		got, err := provider.Fetch(entry)
		if err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}
		if called {
			t.Fatal("HTTP client was called, expected cache hit")
		}
		if got.stats.Wins != 42 {
			t.Fatalf("Wins = %d, want 42", got.stats.Wins)
		}
	})

	t.Run("FetchFresh ignores cache and refreshes it", func(t *testing.T) {
		called := false
		body := `{"status":200,"data":{"stats":{"all":{"overall":{"wins":7}}}}}`
		client := newTestHTTPClient(func(r *http.Request) (*http.Response, error) {
			called = true
			return newTestResponse(r, http.StatusOK, body), nil
		})

		provider := &fortniteAPIStatsProvider{
			token:  "test-token",
			client: client,
			cache:  make(map[string]cachedSnapshot),
		}
		provider.storeCachedSnapshot(provider.cacheKey(entry, ""), playerSnapshot{entry: entry, stats: statLine{Wins: 42}})

		got, err := provider.FetchFresh(entry)
		if err != nil {
			t.Fatalf("FetchFresh() error = %v", err)
		}
		if !called {
			t.Fatal("HTTP client was not called, expected FetchFresh to bypass cache")
		}
		if got.stats.Wins != 7 {
			t.Fatalf("Wins = %d, want 7", got.stats.Wins)
		}

		cached, ok := provider.cachedSnapshot(provider.cacheKey(entry, ""))
		if !ok {
			t.Fatal("expected cache to be populated after FetchFresh")
		}
		if cached.stats.Wins != 7 {
			t.Fatalf("cached Wins = %d, want 7", cached.stats.Wins)
		}
	})

	t.Run("populates cache after fetch", func(t *testing.T) {
		body := `{"status":200,"data":{"stats":{"all":{"overall":{"wins":7}}}}}`
		client := newTestHTTPClient(func(r *http.Request) (*http.Response, error) {
			return newTestResponse(r, http.StatusOK, body), nil
		})

		provider := &fortniteAPIStatsProvider{
			token:  "test-token",
			client: client,
			cache:  make(map[string]cachedSnapshot),
		}

		_, err := provider.Fetch(entry)
		if err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}

		cached, ok := provider.cachedSnapshot(provider.cacheKey(entry, ""))
		if !ok {
			t.Fatal("expected cache to be populated after Fetch")
		}
		if cached.stats.Wins != 7 {
			t.Fatalf("cached Wins = %d, want 7", cached.stats.Wins)
		}
	})
}

func TestEpicStatusProviderErrors(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		wantErr string
	}{
		{
			name:    "non-200 response",
			status:  http.StatusServiceUnavailable,
			body:    "",
			wantErr: "epic status returned 503",
		},
		{
			name:    "invalid json",
			status:  http.StatusOK,
			body:    `{invalid`,
			wantErr: "decode status data",
		},
		{
			name:    "missing fortnite component",
			status:  http.StatusOK,
			body:    `{"components":[{"id":"other","name":"Other","status":"operational"}],"status":{"description":"OK"}}`,
			wantErr: "fortnite component is missing",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := newTestHTTPClient(func(r *http.Request) (*http.Response, error) {
				return newTestResponse(r, tt.status, tt.body), nil
			})

			provider := &epicStatusProvider{client: client, url: "http://example.invalid/status"}
			_, err := provider.Summary()
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %q, want substring %q", err.Error(), tt.wantErr)
			}
		})
	}
}
