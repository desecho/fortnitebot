package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"
)

type appConfig struct {
	botToken          string
	fortniteAPIToken  string
	fortniteAPI2Token string
	playersFile       string
	pollTimeoutSecs   int
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

	seasonProvider := newFortniteAPISeasonProvider(cfg.fortniteAPI2Token)
	statusSource := newEpicStatusProvider()
	client := newTelegramClient(cfg.botToken)
	if err := runBot(client, provider, seasonProvider, statusSource, cfg.pollTimeoutSecs); err != nil {
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

	fortniteToken2 := strings.TrimSpace(os.Getenv("FORTNITE_API2_TOKEN"))
	if fortniteToken2 == "" {
		return appConfig{}, errors.New("FORTNITE_API2_TOKEN is required")
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
		botToken:          token,
		fortniteAPIToken:  fortniteToken,
		fortniteAPI2Token: fortniteToken2,
		playersFile:       playersFile,
		pollTimeoutSecs:   pollTimeout,
	}, nil
}

func runBot(client *telegramClient, provider statsProvider, season seasonProvider, status statusProvider, pollTimeout int) error {
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

			response := handleMessage(provider, season, status, update.Message.Text)
			if strings.TrimSpace(response) == "" {
				continue
			}

			if err := client.sendMessage(update.Message.Chat.ID, response); err != nil {
				log.Printf("send message failed: %v", err)
				time.Sleep(1 * time.Second)
				if retryErr := client.sendMessage(update.Message.Chat.ID, response); retryErr != nil {
					log.Printf("send message retry failed: %v", retryErr)
				}
			}
		}
	}
}
