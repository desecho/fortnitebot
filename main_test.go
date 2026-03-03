package main

import (
	"strings"
	"testing"
)

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

func TestLoadConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		envs    map[string]string
		wantErr string
	}{
		{
			name: "missing TELEGRAM_BOT_TOKEN",
			envs: map[string]string{
				"TELEGRAM_BOT_TOKEN":  "",
				"FORTNITE_API_TOKEN":  "ft",
				"FORTNITE_API2_TOKEN": "ft2",
			},
			wantErr: "TELEGRAM_BOT_TOKEN is required",
		},
		{
			name: "missing FORTNITE_API_TOKEN",
			envs: map[string]string{
				"TELEGRAM_BOT_TOKEN":  "tg",
				"FORTNITE_API_TOKEN":  "",
				"FORTNITE_API2_TOKEN": "ft2",
			},
			wantErr: "FORTNITE_API_TOKEN is required",
		},
		{
			name: "POLL_TIMEOUT_SECS too low",
			envs: map[string]string{
				"TELEGRAM_BOT_TOKEN":  "tg",
				"FORTNITE_API_TOKEN":  "ft",
				"FORTNITE_API2_TOKEN": "ft2",
				"POLL_TIMEOUT_SECS":   "0",
			},
			wantErr: "POLL_TIMEOUT_SECS must be an integer between 1 and 60",
		},
		{
			name: "POLL_TIMEOUT_SECS too high",
			envs: map[string]string{
				"TELEGRAM_BOT_TOKEN":  "tg",
				"FORTNITE_API_TOKEN":  "ft",
				"FORTNITE_API2_TOKEN": "ft2",
				"POLL_TIMEOUT_SECS":   "61",
			},
			wantErr: "POLL_TIMEOUT_SECS must be an integer between 1 and 60",
		},
		{
			name: "POLL_TIMEOUT_SECS not a number",
			envs: map[string]string{
				"TELEGRAM_BOT_TOKEN":  "tg",
				"FORTNITE_API_TOKEN":  "ft",
				"FORTNITE_API2_TOKEN": "ft2",
				"POLL_TIMEOUT_SECS":   "abc",
			},
			wantErr: "POLL_TIMEOUT_SECS must be an integer between 1 and 60",
		},
		{
			name: "POLL_TIMEOUT_SECS negative",
			envs: map[string]string{
				"TELEGRAM_BOT_TOKEN":  "tg",
				"FORTNITE_API_TOKEN":  "ft",
				"FORTNITE_API2_TOKEN": "ft2",
				"POLL_TIMEOUT_SECS":   "-5",
			},
			wantErr: "POLL_TIMEOUT_SECS must be an integer between 1 and 60",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.envs {
				t.Setenv(k, v)
			}
			t.Setenv("PLAYERS_FILE", "")

			_, err := loadConfig()
			if err == nil {
				t.Fatal("loadConfig() error = nil, want error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %q, want substring %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestLoadConfigCustomPollTimeout(t *testing.T) {
	t.Setenv("TELEGRAM_BOT_TOKEN", "tg")
	t.Setenv("FORTNITE_API_TOKEN", "ft")
	t.Setenv("FORTNITE_API2_TOKEN", "ft2")
	t.Setenv("PLAYERS_FILE", "")
	t.Setenv("POLL_TIMEOUT_SECS", "15")

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig() error = %v", err)
	}
	if cfg.pollTimeoutSecs != 15 {
		t.Fatalf("pollTimeoutSecs = %d, want 15", cfg.pollTimeoutSecs)
	}
}

func TestLoadConfigCustomPlayersFile(t *testing.T) {
	t.Setenv("TELEGRAM_BOT_TOKEN", "tg")
	t.Setenv("FORTNITE_API_TOKEN", "ft")
	t.Setenv("FORTNITE_API2_TOKEN", "ft2")
	t.Setenv("PLAYERS_FILE", "custom.json")
	t.Setenv("POLL_TIMEOUT_SECS", "")

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig() error = %v", err)
	}
	if cfg.playersFile != "custom.json" {
		t.Fatalf("playersFile = %q, want custom.json", cfg.playersFile)
	}
}
