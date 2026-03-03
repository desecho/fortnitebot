package main

import (
	"fmt"
	"io"
	"net/http"
	"strings"
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

// --- Configurable stats provider for testing ---

type testPlayer struct {
	entry    playerCatalogEntry
	snapshot playerSnapshot
}

type configurableStatsProvider struct {
	names     []string
	entries   []playerCatalogEntry
	players   map[string]playerCatalogEntry
	snapshots map[string]playerSnapshot
	fetchErr  error
}

func newTestStatsProvider(players ...testPlayer) *configurableStatsProvider {
	p := &configurableStatsProvider{
		players:   make(map[string]playerCatalogEntry),
		snapshots: make(map[string]playerSnapshot),
	}
	for _, pl := range players {
		p.names = append(p.names, pl.entry.Name)
		p.entries = append(p.entries, pl.entry)
		p.players[strings.ToLower(pl.entry.Name)] = pl.entry
		p.snapshots[pl.entry.AccountID] = pl.snapshot
	}
	return p
}

func (p *configurableStatsProvider) Names() []string              { return p.names }
func (p *configurableStatsProvider) Entries() []playerCatalogEntry { return p.entries }
func (p *configurableStatsProvider) Count() int                   { return len(p.entries) }

func (p *configurableStatsProvider) Lookup(name string) (playerCatalogEntry, bool) {
	e, ok := p.players[strings.ToLower(name)]
	return e, ok
}

func (p *configurableStatsProvider) Fetch(entry playerCatalogEntry) (playerSnapshot, error) {
	if p.fetchErr != nil {
		return playerSnapshot{}, p.fetchErr
	}
	s, ok := p.snapshots[entry.AccountID]
	if !ok {
		return playerSnapshot{}, fmt.Errorf("no snapshot for %s", entry.Name)
	}
	return s, nil
}

func (p *configurableStatsProvider) FetchSeason(entry playerCatalogEntry) (playerSnapshot, error) {
	return p.Fetch(entry)
}

// Test data

var (
	aliceEntry   = playerCatalogEntry{Name: "Alice", AccountID: "a1"}
	bobEntry     = playerCatalogEntry{Name: "Bob", AccountID: "b2"}
	charlieEntry = playerCatalogEntry{Name: "Charlie", AccountID: "c3"}

	aliceStats   = statLine{Wins: 100, Kills: 500, KillsPerMatch: 2.50, Deaths: 200, KD: 2.50, Matches: 200, WinRate: 50.0, MinutesPlayed: 6000}
	bobStats     = statLine{Wins: 80, Kills: 600, KillsPerMatch: 3.00, Deaths: 150, KD: 4.00, Matches: 200, WinRate: 40.0, MinutesPlayed: 5000}
	charlieStats = statLine{Wins: 100, Kills: 400, KillsPerMatch: 2.00, Deaths: 200, KD: 2.00, Matches: 200, WinRate: 50.0, MinutesPlayed: 7000}

	aliceSnapshot   = playerSnapshot{entry: aliceEntry, stats: aliceStats}
	bobSnapshot     = playerSnapshot{entry: bobEntry, stats: bobStats}
	charlieSnapshot = playerSnapshot{entry: charlieEntry, stats: charlieStats}
)

func twoPlayerProvider() *configurableStatsProvider {
	return newTestStatsProvider(
		testPlayer{entry: aliceEntry, snapshot: aliceSnapshot},
		testPlayer{entry: bobEntry, snapshot: bobSnapshot},
	)
}

func threePlayerProvider() *configurableStatsProvider {
	return newTestStatsProvider(
		testPlayer{entry: aliceEntry, snapshot: aliceSnapshot},
		testPlayer{entry: bobEntry, snapshot: bobSnapshot},
		testPlayer{entry: charlieEntry, snapshot: charlieSnapshot},
	)
}
