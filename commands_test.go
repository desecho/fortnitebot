package main

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestHandleMessageSeasonRoute(t *testing.T) {
	got := handleMessage(stubStatsProvider{}, stubSeasonProvider{days: 5}, stubStatusProvider{}, nil, "/season")
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
		nil,
		"/status",
	)

	want := strings.Join([]string{
		"Fortnite status",
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

func TestNormalizeCommand(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"/help", "/help"},
		{"/help@BotName", "/help"},
		{"/HELP", "/help"},
		{"/HELP@BotName", "/help"},
		{"  /help  ", "/help"},
		{"/compare@MyBot", "/compare"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := normalizeCommand(tt.input); got != tt.want {
				t.Fatalf("normalizeCommand(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestHelpText(t *testing.T) {
	got := helpText()
	for _, want := range []string{"/players", "/season", "/status", "/stats", "/seasonstats", "/compare", "/seasoncompare", "/session", "/sessioncurrent", "/sessions"} {
		if !strings.Contains(got, want) {
			t.Fatalf("helpText() missing %q", want)
		}
	}
}

func TestPlayersText(t *testing.T) {
	t.Run("no players", func(t *testing.T) {
		got := playersText(stubStatsProvider{})
		want := "No players are configured."
		if got != want {
			t.Fatalf("playersText() = %q, want %q", got, want)
		}
	})

	t.Run("with players", func(t *testing.T) {
		provider := newTestStatsProvider(
			testPlayer{entry: aliceEntry, snapshot: aliceSnapshot},
			testPlayer{entry: bobEntry, snapshot: bobSnapshot},
		)
		got := playersText(provider)
		want := "Configured players:\nAlice\nBob"
		if got != want {
			t.Fatalf("playersText() = %q, want %q", got, want)
		}
	})
}

func TestFormatStats(t *testing.T) {
	player := playerSnapshot{
		entry: playerCatalogEntry{Name: "TestPlayer"},
		stats: statLine{
			Wins: 10, Kills: 50, KillsPerMatch: 2.50, Deaths: 20,
			KD: 2.50, Matches: 20, WinRate: 50.00, MinutesPlayed: 600,
		},
	}

	got := formatStats(player)
	lines := strings.Split(got, "\n")
	if lines[0] != "TestPlayer" {
		t.Fatalf("first line = %q, want TestPlayer", lines[0])
	}

	expectations := []string{
		"Wins: 10",
		"Kills: 50",
		"Kills/match: 2.50",
		"Deaths: 20",
		"K/D: 2.50",
		"Matches: 20",
		"Win rate: 50.00%",
		"Hours played: 10.00",
	}

	for _, expected := range expectations {
		if !strings.Contains(got, expected) {
			t.Fatalf("formatStats() missing %q in:\n%s", expected, got)
		}
	}
}

func TestHoursPlayed(t *testing.T) {
	tests := []struct {
		minutes int64
		want    float64
	}{
		{60, 1.0},
		{0, 0.0},
		{90, 1.5},
		{30, 0.5},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%d minutes", tt.minutes), func(t *testing.T) {
			if got := hoursPlayed(tt.minutes); got != tt.want {
				t.Fatalf("hoursPlayed(%d) = %f, want %f", tt.minutes, got, tt.want)
			}
		})
	}
}

func TestFallbackText(t *testing.T) {
	tests := []struct {
		value    string
		fallback string
		want     string
	}{
		{"hello", "default", "hello"},
		{"", "default", "default"},
		{"  ", "default", "default"},
		{"  hello  ", "default", "hello"},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("value=%q", tt.value), func(t *testing.T) {
			if got := fallbackText(tt.value, tt.fallback); got != tt.want {
				t.Fatalf("fallbackText(%q, %q) = %q, want %q", tt.value, tt.fallback, got, tt.want)
			}
		})
	}
}

func TestCompareTitle(t *testing.T) {
	if got := compareTitle(false); got != "Compare (overall)" {
		t.Fatalf("compareTitle(false) = %q, want %q", got, "Compare (overall)")
	}
	if got := compareTitle(true); got != "Compare (season)" {
		t.Fatalf("compareTitle(true) = %q, want %q", got, "Compare (season)")
	}
}

func TestLeaderLabel(t *testing.T) {
	winsFn := func(s statLine) float64 { return float64(s.Wins) }
	deathsFn := func(s statLine) float64 { return float64(s.Deaths) }

	tests := []struct {
		name          string
		players       []playerSnapshot
		valueFn       func(statLine) float64
		lowerIsBetter bool
		want          string
	}{
		{
			name:    "empty",
			players: nil,
			valueFn: winsFn,
			want:    "",
		},
		{
			name:    "single player",
			players: []playerSnapshot{aliceSnapshot},
			valueFn: winsFn,
			want:    "Alice",
		},
		{
			name:    "clear winner higher is better",
			players: []playerSnapshot{aliceSnapshot, bobSnapshot},
			valueFn: winsFn,
			want:    "Alice",
		},
		{
			name:          "clear winner lower is better",
			players:       []playerSnapshot{aliceSnapshot, bobSnapshot},
			valueFn:       deathsFn,
			lowerIsBetter: true,
			want:          "Bob",
		},
		{
			name:    "tie between two",
			players: []playerSnapshot{aliceSnapshot, charlieSnapshot},
			valueFn: winsFn,
			want:    "Tie (Alice / Charlie)",
		},
		{
			name: "tie among three",
			players: []playerSnapshot{
				aliceSnapshot,
				{entry: playerCatalogEntry{Name: "Dan"}, stats: statLine{Wins: 100}},
				charlieSnapshot,
			},
			valueFn: winsFn,
			want:    "Tie (Alice / Charlie / Dan)",
		},
		{
			name:          "tie lower is better",
			players:       []playerSnapshot{aliceSnapshot, charlieSnapshot},
			valueFn:       deathsFn,
			lowerIsBetter: true,
			want:          "Tie (Alice / Charlie)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := leaderLabel(tt.players, tt.valueFn, tt.lowerIsBetter)
			if got != tt.want {
				t.Fatalf("leaderLabel() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestStatsText(t *testing.T) {
	t.Run("no players configured", func(t *testing.T) {
		got := statsText(stubStatsProvider{}, nil, false)
		want := "No players are configured."
		if got != want {
			t.Fatalf("got = %q, want %q", got, want)
		}
	})

	t.Run("too many args overall", func(t *testing.T) {
		provider := twoPlayerProvider()
		got := statsText(provider, []string{"a", "b"}, false)
		want := "Usage: /stats [player]"
		if got != want {
			t.Fatalf("got = %q, want %q", got, want)
		}
	})

	t.Run("too many args season", func(t *testing.T) {
		provider := twoPlayerProvider()
		got := statsText(provider, []string{"a", "b"}, true)
		want := "Usage: /seasonstats [player]"
		if got != want {
			t.Fatalf("got = %q, want %q", got, want)
		}
	})

	t.Run("unknown player", func(t *testing.T) {
		provider := twoPlayerProvider()
		got := statsText(provider, []string{"nobody"}, false)
		if !strings.Contains(got, "Unknown player") {
			t.Fatalf("got = %q, want substring 'Unknown player'", got)
		}
	})

	t.Run("single player", func(t *testing.T) {
		provider := twoPlayerProvider()
		got := statsText(provider, []string{"Alice"}, false)
		if !strings.Contains(got, "Alice") {
			t.Fatalf("got = %q, want substring 'Alice'", got)
		}
		if !strings.Contains(got, "Wins: 100") {
			t.Fatalf("got = %q, want substring 'Wins: 100'", got)
		}
	})

	t.Run("all players", func(t *testing.T) {
		provider := twoPlayerProvider()
		got := statsText(provider, nil, false)
		if !strings.Contains(got, "Alice") || !strings.Contains(got, "Bob") {
			t.Fatalf("got = %q, want both Alice and Bob", got)
		}
	})

	t.Run("single player fetch error", func(t *testing.T) {
		provider := twoPlayerProvider()
		provider.fetchErr = errors.New("api down")
		got := statsText(provider, []string{"Alice"}, false)
		if !strings.Contains(got, "Failed to fetch stats") {
			t.Fatalf("got = %q, want substring 'Failed to fetch stats'", got)
		}
	})

	t.Run("batch fetch error", func(t *testing.T) {
		provider := twoPlayerProvider()
		provider.fetchErr = errors.New("api down")
		got := statsText(provider, nil, false)
		if !strings.Contains(got, "Failed to fetch stats") {
			t.Fatalf("got = %q, want substring 'Failed to fetch stats'", got)
		}
	})
}

func TestCompareText(t *testing.T) {
	t.Run("not enough players configured", func(t *testing.T) {
		provider := newTestStatsProvider(testPlayer{entry: aliceEntry, snapshot: aliceSnapshot})
		got := compareText(provider, []string{"Alice", "Bob"}, false)
		if !strings.Contains(got, "Add at least two players") {
			t.Fatalf("got = %q, want substring 'Add at least two players'", got)
		}
	})

	t.Run("not enough players configured season", func(t *testing.T) {
		provider := newTestStatsProvider(testPlayer{entry: aliceEntry, snapshot: aliceSnapshot})
		got := compareText(provider, []string{"Alice", "Bob"}, true)
		if !strings.Contains(got, "/seasoncompare") {
			t.Fatalf("got = %q, want substring '/seasoncompare'", got)
		}
	})

	t.Run("too few args", func(t *testing.T) {
		provider := twoPlayerProvider()
		got := compareText(provider, []string{"Alice"}, false)
		if !strings.Contains(got, "Usage: /compare") {
			t.Fatalf("got = %q, want substring 'Usage: /compare'", got)
		}
	})

	t.Run("too few args season", func(t *testing.T) {
		provider := twoPlayerProvider()
		got := compareText(provider, []string{"Alice"}, true)
		if !strings.Contains(got, "Usage: /seasoncompare") {
			t.Fatalf("got = %q, want substring 'Usage: /seasoncompare'", got)
		}
	})

	t.Run("duplicate player", func(t *testing.T) {
		provider := twoPlayerProvider()
		got := compareText(provider, []string{"Alice", "Alice"}, false)
		if !strings.Contains(got, "only be listed once") {
			t.Fatalf("got = %q, want substring 'only be listed once'", got)
		}
	})

	t.Run("duplicate player case insensitive", func(t *testing.T) {
		provider := twoPlayerProvider()
		got := compareText(provider, []string{"Alice", "alice"}, false)
		if !strings.Contains(got, "only be listed once") {
			t.Fatalf("got = %q, want substring 'only be listed once'", got)
		}
	})

	t.Run("unknown player", func(t *testing.T) {
		provider := twoPlayerProvider()
		got := compareText(provider, []string{"Alice", "nobody"}, false)
		if !strings.Contains(got, "Unknown player") {
			t.Fatalf("got = %q, want substring 'Unknown player'", got)
		}
	})

	t.Run("valid two player compare", func(t *testing.T) {
		provider := twoPlayerProvider()
		got := compareText(provider, []string{"Alice", "Bob"}, false)
		if !strings.Contains(got, "Compare (overall)") {
			t.Fatalf("got = %q, want substring 'Compare (overall)'", got)
		}
		if !strings.Contains(got, "Wins leader: Alice") {
			t.Fatalf("got = %q, want substring 'Wins leader: Alice'", got)
		}
		if !strings.Contains(got, "Kills leader: Bob") {
			t.Fatalf("got = %q, want substring 'Kills leader: Bob'", got)
		}
	})

	t.Run("season compare title", func(t *testing.T) {
		provider := twoPlayerProvider()
		got := compareText(provider, []string{"Alice", "Bob"}, true)
		if !strings.Contains(got, "Compare (season)") {
			t.Fatalf("got = %q, want substring 'Compare (season)'", got)
		}
	})

	t.Run("three player compare with tie", func(t *testing.T) {
		provider := threePlayerProvider()
		got := compareText(provider, []string{"Alice", "Bob", "Charlie"}, false)
		if !strings.Contains(got, "Compare (overall)") {
			t.Fatalf("got = %q, want substring 'Compare (overall)'", got)
		}
		if !strings.Contains(got, "Tie (Alice / Charlie)") {
			t.Fatalf("got = %q, want substring 'Tie (Alice / Charlie)' for wins tie", got)
		}
	})

	t.Run("fetch error", func(t *testing.T) {
		provider := twoPlayerProvider()
		provider.fetchErr = errors.New("api down")
		got := compareText(provider, []string{"Alice", "Bob"}, false)
		if !strings.Contains(got, "Failed to fetch compare stats") {
			t.Fatalf("got = %q, want substring 'Failed to fetch compare stats'", got)
		}
	})
}

func TestStatusTextEdgeCases(t *testing.T) {
	t.Run("no services", func(t *testing.T) {
		got := statusText(stubStatusProvider{
			summary: fortniteStatusSummary{
				Epic:     "All Systems Operational",
				Fortnite: "Operational",
				Services: nil,
			},
		})
		if !strings.Contains(got, "No Fortnite services are listed.") {
			t.Fatalf("got = %q, want substring 'No Fortnite services are listed.'", got)
		}
	})

	t.Run("empty epic and fortnite values use fallback", func(t *testing.T) {
		got := statusText(stubStatusProvider{
			summary: fortniteStatusSummary{
				Epic:     "",
				Fortnite: "",
				Services: []fortniteServiceStatus{{Name: "Login", Status: "OK"}},
			},
		})
		if !strings.Contains(got, "Fortnite overall: Unknown") {
			t.Fatalf("got = %q, want substring 'Fortnite overall: Unknown'", got)
		}
	})
}

func TestHandleMessageRoutes(t *testing.T) {
	provider := twoPlayerProvider()
	season := stubSeasonProvider{days: 5}
	status := stubStatusProvider{
		summary: fortniteStatusSummary{
			Epic: "OK", Fortnite: "OK",
			Services: []fortniteServiceStatus{{Name: "Login", Status: "OK"}},
		},
	}

	tests := []struct {
		name         string
		text         string
		wantContains string
		wantEmpty    bool
	}{
		{"empty text", "", "", true},
		{"unknown command", "/foo", "Unknown command", false},
		{"/start", "/start", "Fortnite stats bot", false},
		{"/help", "/help", "Fortnite stats bot", false},
		{"/players", "/players", "Configured players:", false},
		{"/stats single", "/stats Alice", "Wins: 100", false},
		{"/seasonstats single", "/seasonstats Alice", "Wins: 100", false},
		{"/compare", "/compare Alice Bob", "Compare (overall)", false},
		{"/seasoncompare", "/seasoncompare Alice Bob", "Compare (season)", false},
		{"command with @mention", "/help@MyBot", "Fortnite stats bot", false},
		{"uppercase command", "/HELP", "Fortnite stats bot", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := handleMessage(provider, season, status, nil, tt.text)
			if tt.wantEmpty {
				if got != "" {
					t.Fatalf("got = %q, want empty", got)
				}
				return
			}
			if !strings.Contains(got, tt.wantContains) {
				t.Fatalf("got = %q, want substring %q", got, tt.wantContains)
			}
		})
	}
}
