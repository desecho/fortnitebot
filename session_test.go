package main

import (
	"fmt"
	"strings"
	"testing"
)

type stubSnapshotStore struct {
	snapshots map[string][]dailySnapshot
	upserted  []dailySnapshot
	err       error
}

func newStubSnapshotStore() *stubSnapshotStore {
	return &stubSnapshotStore{
		snapshots: make(map[string][]dailySnapshot),
	}
}

func (s *stubSnapshotStore) UpsertSnapshot(snapshot dailySnapshot) error {
	if s.err != nil {
		return s.err
	}
	s.upserted = append(s.upserted, snapshot)
	return nil
}

func (s *stubSnapshotStore) RecentSnapshots(accountID string, limit int) ([]dailySnapshot, error) {
	if s.err != nil {
		return nil, s.err
	}
	snaps := s.snapshots[accountID]
	if len(snaps) > limit {
		snaps = snaps[:limit]
	}
	return snaps, nil
}

func TestDetectSession(t *testing.T) {
	t.Run("session detected", func(t *testing.T) {
		newer := dailySnapshot{
			Date:  "2026-03-03",
			Stats: dailyStatLine{Wins: 10, Kills: 50, Deaths: 20, Matches: 30, MinutesPlayed: 600, Top3: 15},
		}
		older := dailySnapshot{
			Date:  "2026-03-02",
			Stats: dailyStatLine{Wins: 8, Kills: 40, Deaths: 15, Matches: 25, MinutesPlayed: 500, Top3: 12},
		}

		session := detectSession("Alice", newer, older)
		if session == nil {
			t.Fatal("expected session, got nil")
		}
		if session.Delta.Matches != 5 {
			t.Fatalf("Delta.Matches = %d, want 5", session.Delta.Matches)
		}
		if session.Delta.Wins != 2 {
			t.Fatalf("Delta.Wins = %d, want 2", session.Delta.Wins)
		}
		if session.Delta.Kills != 10 {
			t.Fatalf("Delta.Kills = %d, want 10", session.Delta.Kills)
		}
		if session.Delta.Deaths != 5 {
			t.Fatalf("Delta.Deaths = %d, want 5", session.Delta.Deaths)
		}
		if session.Delta.Top3 != 3 {
			t.Fatalf("Delta.Top3 = %d, want 3", session.Delta.Top3)
		}
		if session.Delta.MinutesPlayed != 100 {
			t.Fatalf("Delta.MinutesPlayed = %d, want 100", session.Delta.MinutesPlayed)
		}
		if session.Date != "2026-03-03" {
			t.Fatalf("Date = %q, want 2026-03-03", session.Date)
		}
		if session.PlayerName != "Alice" {
			t.Fatalf("PlayerName = %q, want Alice", session.PlayerName)
		}

		// KD = 10/5 = 2.0
		if session.KD != 2.0 {
			t.Fatalf("KD = %f, want 2.0", session.KD)
		}

		// WinRate = 2/5 * 100 = 40.0
		if session.WinRate != 40.0 {
			t.Fatalf("WinRate = %f, want 40.0", session.WinRate)
		}

		// KillsPerMatch = 10/5 = 2.0
		if session.KillsPerMatch != 2.0 {
			t.Fatalf("KillsPerMatch = %f, want 2.0", session.KillsPerMatch)
		}
	})

	t.Run("no session when matches equal", func(t *testing.T) {
		newer := dailySnapshot{Stats: dailyStatLine{Matches: 25}}
		older := dailySnapshot{Stats: dailyStatLine{Matches: 25}}

		session := detectSession("Alice", newer, older)
		if session != nil {
			t.Fatal("expected nil session")
		}
	})

	t.Run("no session when matches decrease", func(t *testing.T) {
		newer := dailySnapshot{Stats: dailyStatLine{Matches: 20}}
		older := dailySnapshot{Stats: dailyStatLine{Matches: 25}}

		session := detectSession("Alice", newer, older)
		if session != nil {
			t.Fatal("expected nil session")
		}
	})

	t.Run("zero deaths gives zero kd", func(t *testing.T) {
		newer := dailySnapshot{Stats: dailyStatLine{Wins: 5, Kills: 10, Deaths: 0, Matches: 5}}
		older := dailySnapshot{Stats: dailyStatLine{Wins: 0, Kills: 0, Deaths: 0, Matches: 0}}

		session := detectSession("Alice", newer, older)
		if session == nil {
			t.Fatal("expected session, got nil")
		}
		if session.KD != 0 {
			t.Fatalf("KD = %f, want 0 (zero deaths)", session.KD)
		}
	})
}

func TestFormatSession(t *testing.T) {
	s := sessionSummary{
		PlayerName:    "Alice",
		Date:          "2026-03-03",
		Delta:         dailyStatLine{Matches: 10, Wins: 3, Kills: 25, Deaths: 7, MinutesPlayed: 120, Top3: 4, Top5: 5, Top6: 6, Top10: 7, Top12: 8, Top25: 9},
		KillsPerMatch: 2.50,
		KD:            3.57,
		WinRate:       30.0,
	}

	got := formatSession(s)

	expectations := []string{
		"Alice - Session 2026-03-03",
		"Matches: 10",
		"Wins: 3",
		"Kills: 25",
		"Kills/match: 2.50",
		"Deaths: 7",
		"K/D: 3.57",
		"Win rate: 30.00%",
		"Top 3: 4",
		"Top 5: 5",
		"Top 6: 6",
		"Top 10: 7",
		"Top 12: 8",
		"Top 25: 9",
		"Time played: 2.0h",
	}

	for _, expected := range expectations {
		if !strings.Contains(got, expected) {
			t.Fatalf("formatSession() missing %q in:\n%s", expected, got)
		}
	}
}

func TestFormatSessionCompact(t *testing.T) {
	s := sessionSummary{
		Date:    "2026-03-03",
		Delta:   dailyStatLine{Matches: 10, Wins: 3, Kills: 25},
		KD:      3.57,
		WinRate: 30.0,
	}

	got := formatSessionCompact(s)
	if !strings.Contains(got, "2026-03-03") {
		t.Fatalf("missing date in: %s", got)
	}
	if !strings.Contains(got, "10 matches") {
		t.Fatalf("missing matches in: %s", got)
	}
	if !strings.Contains(got, "3 wins") {
		t.Fatalf("missing wins in: %s", got)
	}
}

func TestSessionText(t *testing.T) {
	t.Run("nil store", func(t *testing.T) {
		provider := twoPlayerProvider()
		got := sessionText(provider, nil, nil)
		if got != "Session tracking is not configured." {
			t.Fatalf("got = %q", got)
		}
	})

	t.Run("no players", func(t *testing.T) {
		got := sessionText(stubStatsProvider{}, newStubSnapshotStore(), nil)
		if got != "No players are configured." {
			t.Fatalf("got = %q", got)
		}
	})

	t.Run("too many args", func(t *testing.T) {
		provider := twoPlayerProvider()
		got := sessionText(provider, newStubSnapshotStore(), []string{"a", "b"})
		if got != "Usage: /session [player]" {
			t.Fatalf("got = %q", got)
		}
	})

	t.Run("unknown player", func(t *testing.T) {
		provider := twoPlayerProvider()
		got := sessionText(provider, newStubSnapshotStore(), []string{"nobody"})
		if !strings.Contains(got, "Unknown player") {
			t.Fatalf("got = %q, want substring 'Unknown player'", got)
		}
	})

	t.Run("not enough data", func(t *testing.T) {
		provider := twoPlayerProvider()
		store := newStubSnapshotStore()
		store.snapshots["a1"] = []dailySnapshot{{Date: "2026-03-03"}}

		got := sessionText(provider, store, []string{"Alice"})
		if !strings.Contains(got, "Not enough data") {
			t.Fatalf("got = %q, want substring 'Not enough data'", got)
		}
	})

	t.Run("session detected for single player", func(t *testing.T) {
		provider := twoPlayerProvider()
		store := newStubSnapshotStore()
		store.snapshots["a1"] = []dailySnapshot{
			{Date: "2026-03-03", Stats: dailyStatLine{Wins: 12, Kills: 60, Deaths: 22, Matches: 32, MinutesPlayed: 700}},
			{Date: "2026-03-02", Stats: dailyStatLine{Wins: 10, Kills: 50, Deaths: 20, Matches: 30, MinutesPlayed: 600}},
		}

		got := sessionText(provider, store, []string{"Alice"})
		if !strings.Contains(got, "Session 2026-03-03") {
			t.Fatalf("got = %q, want substring 'Session 2026-03-03'", got)
		}
		if !strings.Contains(got, "Matches: 2") {
			t.Fatalf("got = %q, want substring 'Matches: 2'", got)
		}
	})

	t.Run("no session detected", func(t *testing.T) {
		provider := twoPlayerProvider()
		store := newStubSnapshotStore()
		store.snapshots["a1"] = []dailySnapshot{
			{Date: "2026-03-03", Stats: dailyStatLine{Matches: 30}},
			{Date: "2026-03-02", Stats: dailyStatLine{Matches: 30}},
		}

		got := sessionText(provider, store, []string{"Alice"})
		if !strings.Contains(got, "No recent gaming session") {
			t.Fatalf("got = %q, want substring 'No recent gaming session'", got)
		}
	})

	t.Run("all players", func(t *testing.T) {
		provider := twoPlayerProvider()
		store := newStubSnapshotStore()
		store.snapshots["a1"] = []dailySnapshot{
			{Date: "2026-03-03", Stats: dailyStatLine{Wins: 12, Kills: 60, Deaths: 22, Matches: 32}},
			{Date: "2026-03-02", Stats: dailyStatLine{Wins: 10, Kills: 50, Deaths: 20, Matches: 30}},
		}
		store.snapshots["b2"] = []dailySnapshot{
			{Date: "2026-03-03", Stats: dailyStatLine{Matches: 30}},
			{Date: "2026-03-02", Stats: dailyStatLine{Matches: 30}},
		}

		got := sessionText(provider, store, nil)
		if !strings.Contains(got, "Alice") {
			t.Fatalf("got = %q, want substring 'Alice'", got)
		}
		if !strings.Contains(got, "Bob") {
			t.Fatalf("got = %q, want substring 'Bob'", got)
		}
	})

	t.Run("store error", func(t *testing.T) {
		provider := twoPlayerProvider()
		store := newStubSnapshotStore()
		store.err = fmt.Errorf("db down")

		got := sessionText(provider, store, []string{"Alice"})
		if !strings.Contains(got, "Failed to fetch session data") {
			t.Fatalf("got = %q, want substring 'Failed to fetch session data'", got)
		}
	})
}

func TestSessionsText(t *testing.T) {
	t.Run("nil store", func(t *testing.T) {
		provider := twoPlayerProvider()
		got := sessionsText(provider, nil, nil)
		if got != "Session tracking is not configured." {
			t.Fatalf("got = %q", got)
		}
	})

	t.Run("no players", func(t *testing.T) {
		got := sessionsText(stubStatsProvider{}, newStubSnapshotStore(), nil)
		if got != "No players are configured." {
			t.Fatalf("got = %q", got)
		}
	})

	t.Run("too many args", func(t *testing.T) {
		provider := twoPlayerProvider()
		got := sessionsText(provider, newStubSnapshotStore(), []string{"a", "b"})
		if got != "Usage: /sessions [player]" {
			t.Fatalf("got = %q", got)
		}
	})

	t.Run("unknown player", func(t *testing.T) {
		provider := twoPlayerProvider()
		got := sessionsText(provider, newStubSnapshotStore(), []string{"nobody"})
		if !strings.Contains(got, "Unknown player") {
			t.Fatalf("got = %q, want substring 'Unknown player'", got)
		}
	})

	t.Run("multiple sessions for one player", func(t *testing.T) {
		provider := twoPlayerProvider()
		store := newStubSnapshotStore()
		store.snapshots["a1"] = []dailySnapshot{
			{Date: "2026-03-03", Stats: dailyStatLine{Wins: 15, Kills: 70, Deaths: 25, Matches: 35}},
			{Date: "2026-03-02", Stats: dailyStatLine{Wins: 12, Kills: 60, Deaths: 22, Matches: 32}},
			{Date: "2026-03-01", Stats: dailyStatLine{Wins: 10, Kills: 50, Deaths: 20, Matches: 30}},
		}

		got := sessionsText(provider, store, []string{"Alice"})
		if !strings.Contains(got, "Recent sessions") {
			t.Fatalf("got = %q, want substring 'Recent sessions'", got)
		}
		if !strings.Contains(got, "2026-03-03") {
			t.Fatalf("got = %q, want substring '2026-03-03'", got)
		}
		if !strings.Contains(got, "2026-03-02") {
			t.Fatalf("got = %q, want substring '2026-03-02'", got)
		}
	})

	t.Run("no sessions detected", func(t *testing.T) {
		provider := twoPlayerProvider()
		store := newStubSnapshotStore()
		store.snapshots["a1"] = []dailySnapshot{
			{Date: "2026-03-03", Stats: dailyStatLine{Matches: 30}},
			{Date: "2026-03-02", Stats: dailyStatLine{Matches: 30}},
			{Date: "2026-03-01", Stats: dailyStatLine{Matches: 30}},
		}

		got := sessionsText(provider, store, []string{"Alice"})
		if !strings.Contains(got, "No recent gaming sessions") {
			t.Fatalf("got = %q, want substring 'No recent gaming sessions'", got)
		}
	})

	t.Run("skips days without activity", func(t *testing.T) {
		provider := twoPlayerProvider()
		store := newStubSnapshotStore()
		store.snapshots["a1"] = []dailySnapshot{
			{Date: "2026-03-04", Stats: dailyStatLine{Wins: 15, Kills: 70, Deaths: 25, Matches: 35}},
			{Date: "2026-03-03", Stats: dailyStatLine{Wins: 12, Kills: 60, Deaths: 22, Matches: 32}},
			{Date: "2026-03-02", Stats: dailyStatLine{Wins: 12, Kills: 60, Deaths: 22, Matches: 32}}, // same as above, no activity
			{Date: "2026-03-01", Stats: dailyStatLine{Wins: 10, Kills: 50, Deaths: 20, Matches: 30}},
		}

		got := sessionsText(provider, store, []string{"Alice"})
		// Should have sessions for 2026-03-04 (vs 03-03) and 2026-03-02 (vs 03-01)
		// 2026-03-03 vs 2026-03-02 has no delta, so no session for 2026-03-03
		if !strings.Contains(got, "2026-03-04") {
			t.Fatalf("got = %q, want substring '2026-03-04'", got)
		}
		if !strings.Contains(got, "2026-03-02") {
			t.Fatalf("got = %q, want substring '2026-03-02' (session when 03-02 vs 03-01 has delta)", got)
		}
		if strings.Contains(got, "2026-03-03") && !strings.Contains(got, "2026-03-04") {
			t.Fatalf("got = %q, should not have session for 2026-03-03 (no activity)", got)
		}
	})
}

func TestExtractDailyStats(t *testing.T) {
	s := statLine{
		Wins: 10, Kills: 50, Deaths: 20, Matches: 30, MinutesPlayed: 600,
		KillsPerMatch: 1.67, KD: 2.5, WinRate: 33.33,
		Top3: 15, Top5: 5, Top6: 20, Top10: 8, Top12: 12, Top25: 25,
	}

	d := extractDailyStats(s)
	if d.Wins != 10 {
		t.Fatalf("Wins = %d, want 10", d.Wins)
	}
	if d.Kills != 50 {
		t.Fatalf("Kills = %d, want 50", d.Kills)
	}
	if d.Deaths != 20 {
		t.Fatalf("Deaths = %d, want 20", d.Deaths)
	}
	if d.Matches != 30 {
		t.Fatalf("Matches = %d, want 30", d.Matches)
	}
	if d.MinutesPlayed != 600 {
		t.Fatalf("MinutesPlayed = %d, want 600", d.MinutesPlayed)
	}
	if d.Top3 != 15 {
		t.Fatalf("Top3 = %d, want 15", d.Top3)
	}
	if d.Top5 != 5 {
		t.Fatalf("Top5 = %d, want 5", d.Top5)
	}
	if d.Top6 != 20 {
		t.Fatalf("Top6 = %d, want 20", d.Top6)
	}
	if d.Top10 != 8 {
		t.Fatalf("Top10 = %d, want 8", d.Top10)
	}
	if d.Top12 != 12 {
		t.Fatalf("Top12 = %d, want 12", d.Top12)
	}
	if d.Top25 != 25 {
		t.Fatalf("Top25 = %d, want 25", d.Top25)
	}
}

func TestCollectSnapshots(t *testing.T) {
	provider := twoPlayerProvider()
	store := newStubSnapshotStore()

	collectSnapshots(provider, store)

	if len(store.upserted) != 2 {
		t.Fatalf("upserted %d snapshots, want 2", len(store.upserted))
	}

	names := map[string]bool{}
	for _, s := range store.upserted {
		names[s.Name] = true
		if s.Date == "" {
			t.Fatal("snapshot date is empty")
		}
		if s.AccountID == "" {
			t.Fatal("snapshot accountId is empty")
		}
	}

	if !names["Alice"] {
		t.Fatal("missing snapshot for Alice")
	}
	if !names["Bob"] {
		t.Fatal("missing snapshot for Bob")
	}
	if provider.fetchCount != 0 {
		t.Fatalf("Fetch() calls = %d, want 0", provider.fetchCount)
	}
	if provider.fetchFreshCount != 2 {
		t.Fatalf("FetchFresh() calls = %d, want 2", provider.fetchFreshCount)
	}
}

func TestSnapshotText(t *testing.T) {
	t.Run("nil store", func(t *testing.T) {
		provider := twoPlayerProvider()
		got := snapshotText(provider, nil)
		if got != "Session tracking is not configured." {
			t.Fatalf("got = %q", got)
		}
	})

	t.Run("no players", func(t *testing.T) {
		got := snapshotText(stubStatsProvider{}, newStubSnapshotStore())
		if got != "No players are configured." {
			t.Fatalf("got = %q", got)
		}
	})

	t.Run("successful collection", func(t *testing.T) {
		provider := twoPlayerProvider()
		store := newStubSnapshotStore()

		got := snapshotText(provider, store)
		if !strings.Contains(got, "Collecting snapshots for") {
			t.Fatalf("got = %q, want substring 'Collecting snapshots for'", got)
		}
		if !strings.Contains(got, "Alice: done") {
			t.Fatalf("got = %q, want substring 'Alice: done'", got)
		}
		if !strings.Contains(got, "Bob: done") {
			t.Fatalf("got = %q, want substring 'Bob: done'", got)
		}
		if len(store.upserted) != 2 {
			t.Fatalf("upserted %d snapshots, want 2", len(store.upserted))
		}
		if provider.fetchCount != 0 {
			t.Fatalf("Fetch() calls = %d, want 0", provider.fetchCount)
		}
		if provider.fetchFreshCount != 2 {
			t.Fatalf("FetchFresh() calls = %d, want 2", provider.fetchFreshCount)
		}
	})

	t.Run("fetch error", func(t *testing.T) {
		provider := twoPlayerProvider()
		provider.fetchErr = fmt.Errorf("api down")
		store := newStubSnapshotStore()

		got := snapshotText(provider, store)
		if !strings.Contains(got, "Alice: failed to fetch") {
			t.Fatalf("got = %q, want substring 'Alice: failed to fetch'", got)
		}
		if provider.fetchCount != 0 {
			t.Fatalf("Fetch() calls = %d, want 0", provider.fetchCount)
		}
		if provider.fetchFreshCount != 2 {
			t.Fatalf("FetchFresh() calls = %d, want 2", provider.fetchFreshCount)
		}
	})

	t.Run("store error", func(t *testing.T) {
		provider := twoPlayerProvider()
		store := newStubSnapshotStore()
		store.err = fmt.Errorf("db down")

		got := snapshotText(provider, store)
		if !strings.Contains(got, "Alice: failed to store") {
			t.Fatalf("got = %q, want substring 'Alice: failed to store'", got)
		}
	})
}

func TestSessionCurrentText(t *testing.T) {
	t.Run("nil store", func(t *testing.T) {
		provider := twoPlayerProvider()
		got := sessionCurrentText(provider, nil, nil)
		if got != "Session tracking is not configured." {
			t.Fatalf("got = %q", got)
		}
	})

	t.Run("no players", func(t *testing.T) {
		got := sessionCurrentText(stubStatsProvider{}, newStubSnapshotStore(), nil)
		if got != "No players are configured." {
			t.Fatalf("got = %q", got)
		}
	})

	t.Run("too many args", func(t *testing.T) {
		provider := twoPlayerProvider()
		got := sessionCurrentText(provider, newStubSnapshotStore(), []string{"a", "b"})
		if got != "Usage: /sessioncurrent [player]" {
			t.Fatalf("got = %q", got)
		}
	})

	t.Run("unknown player", func(t *testing.T) {
		provider := twoPlayerProvider()
		got := sessionCurrentText(provider, newStubSnapshotStore(), []string{"nobody"})
		if !strings.Contains(got, "Unknown player") {
			t.Fatalf("got = %q, want substring 'Unknown player'", got)
		}
	})

	t.Run("no snapshots available", func(t *testing.T) {
		provider := twoPlayerProvider()
		store := newStubSnapshotStore()

		got := sessionCurrentText(provider, store, []string{"Alice"})
		if !strings.Contains(got, "No snapshot data available") {
			t.Fatalf("got = %q, want substring 'No snapshot data available'", got)
		}
	})

	t.Run("session detected with live data", func(t *testing.T) {
		provider := newTestStatsProvider(
			testPlayer{
				entry: aliceEntry,
				snapshot: playerSnapshot{
					entry: aliceEntry,
					stats: statLine{Wins: 12, Kills: 60, Deaths: 22, Matches: 32, MinutesPlayed: 700},
				},
			},
		)
		store := newStubSnapshotStore()
		store.snapshots["a1"] = []dailySnapshot{
			{Date: "2026-03-02", Stats: dailyStatLine{Wins: 10, Kills: 50, Deaths: 20, Matches: 30, MinutesPlayed: 600}},
		}

		got := sessionCurrentText(provider, store, []string{"Alice"})
		if !strings.Contains(got, "Matches: 2") {
			t.Fatalf("got = %q, want substring 'Matches: 2'", got)
		}
		if !strings.Contains(got, "Wins: 2") {
			t.Fatalf("got = %q, want substring 'Wins: 2'", got)
		}
		if !strings.Contains(got, "Kills: 10") {
			t.Fatalf("got = %q, want substring 'Kills: 10'", got)
		}
		if provider.fetchFreshCount != 1 {
			t.Fatalf("FetchFresh() calls = %d, want 1", provider.fetchFreshCount)
		}
	})

	t.Run("no activity since snapshot", func(t *testing.T) {
		provider := newTestStatsProvider(
			testPlayer{
				entry: aliceEntry,
				snapshot: playerSnapshot{
					entry: aliceEntry,
					stats: statLine{Wins: 10, Kills: 50, Deaths: 20, Matches: 30},
				},
			},
		)
		store := newStubSnapshotStore()
		store.snapshots["a1"] = []dailySnapshot{
			{Date: "2026-03-02", Stats: dailyStatLine{Wins: 10, Kills: 50, Deaths: 20, Matches: 30}},
		}

		got := sessionCurrentText(provider, store, []string{"Alice"})
		if !strings.Contains(got, "No activity since last snapshot") {
			t.Fatalf("got = %q, want substring 'No activity since last snapshot'", got)
		}
	})

	t.Run("store error", func(t *testing.T) {
		provider := twoPlayerProvider()
		store := newStubSnapshotStore()
		store.err = fmt.Errorf("db down")

		got := sessionCurrentText(provider, store, []string{"Alice"})
		if !strings.Contains(got, "Failed to fetch session data") {
			t.Fatalf("got = %q, want substring 'Failed to fetch session data'", got)
		}
	})

	t.Run("fetch fresh error", func(t *testing.T) {
		provider := twoPlayerProvider()
		provider.fetchFreshErr = fmt.Errorf("api down")
		store := newStubSnapshotStore()
		store.snapshots["a1"] = []dailySnapshot{
			{Date: "2026-03-02", Stats: dailyStatLine{Matches: 30}},
		}

		got := sessionCurrentText(provider, store, []string{"Alice"})
		if !strings.Contains(got, "Failed to fetch live stats") {
			t.Fatalf("got = %q, want substring 'Failed to fetch live stats'", got)
		}
	})

	t.Run("all players", func(t *testing.T) {
		provider := newTestStatsProvider(
			testPlayer{
				entry: aliceEntry,
				snapshot: playerSnapshot{
					entry: aliceEntry,
					stats: statLine{Wins: 12, Kills: 60, Deaths: 22, Matches: 32},
				},
			},
			testPlayer{
				entry: bobEntry,
				snapshot: playerSnapshot{
					entry: bobEntry,
					stats: statLine{Wins: 5, Kills: 20, Deaths: 10, Matches: 15},
				},
			},
		)
		store := newStubSnapshotStore()
		store.snapshots["a1"] = []dailySnapshot{
			{Date: "2026-03-02", Stats: dailyStatLine{Wins: 10, Kills: 50, Deaths: 20, Matches: 30}},
		}
		store.snapshots["b2"] = []dailySnapshot{
			{Date: "2026-03-02", Stats: dailyStatLine{Wins: 5, Kills: 20, Deaths: 10, Matches: 15}},
		}

		got := sessionCurrentText(provider, store, nil)
		if !strings.Contains(got, "Alice") {
			t.Fatalf("got = %q, want substring 'Alice'", got)
		}
		if !strings.Contains(got, "Bob") {
			t.Fatalf("got = %q, want substring 'Bob'", got)
		}
	})
}

func TestHandleMessageSessionRoutes(t *testing.T) {
	provider := twoPlayerProvider()
	season := stubSeasonProvider{days: 5}
	status := stubStatusProvider{
		summary: fortniteStatusSummary{
			Epic: "OK", Fortnite: "OK",
			Services: []fortniteServiceStatus{{Name: "Login", Status: "OK"}},
		},
	}
	store := newStubSnapshotStore()

	tests := []struct {
		name         string
		text         string
		wantContains string
	}{
		{"/session no store", "/session", "Session tracking is not configured."},
		{"/sessioncurrent no store", "/sessioncurrent", "Session tracking is not configured."},
		{"/sessions no store", "/sessions", "Session tracking is not configured."},
		{"/snapshot no store", "/snapshot", "Session tracking is not configured."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := handleMessage(provider, season, status, nil, tt.text)
			if !strings.Contains(got, tt.wantContains) {
				t.Fatalf("got = %q, want substring %q", got, tt.wantContains)
			}
		})
	}

	t.Run("/session with store", func(t *testing.T) {
		got := handleMessage(provider, season, status, store, "/session Alice")
		if !strings.Contains(got, "Alice") {
			t.Fatalf("got = %q, want substring 'Alice'", got)
		}
	})

	t.Run("/sessions with store", func(t *testing.T) {
		got := handleMessage(provider, season, status, store, "/sessions Alice")
		if !strings.Contains(got, "Alice") {
			t.Fatalf("got = %q, want substring 'Alice'", got)
		}
	})

	t.Run("/snapshot with store", func(t *testing.T) {
		got := handleMessage(provider, season, status, store, "/snapshot")
		if !strings.Contains(got, "Collecting snapshots for") {
			t.Fatalf("got = %q, want substring 'Collecting snapshots for'", got)
		}
	})
}
