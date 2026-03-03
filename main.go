package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
)

type appConfig struct {
	botToken          string
	fortniteAPIToken  string
	fortniteAPI2Token string
	playersFile       string
	pollTimeoutSecs   int
	mongodbURI        string
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

	var store snapshotStore
	if cfg.mongodbURI != "" {
		s, err := newMongoSnapshotStore(cfg.mongodbURI)
		if err != nil {
			log.Fatal(err)
		}
		store = s
		log.Println("MongoDB connected, session tracking enabled.")

		startCron(provider, store)
	} else {
		log.Println("MONGODB_URI not set, session tracking disabled.")
	}

	seasonProvider := newFortniteAPISeasonProvider(cfg.fortniteAPI2Token)
	statusSource := newEpicStatusProvider()
	client, err := newTelegramBotClient(cfg.botToken)
	if err != nil {
		log.Fatal(err)
	}
	if err := runBot(client, provider, seasonProvider, statusSource, store, cfg.pollTimeoutSecs); err != nil {
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

	mongodbURI := strings.TrimSpace(os.Getenv("MONGODB_URI"))

	return appConfig{
		botToken:          token,
		fortniteAPIToken:  fortniteToken,
		fortniteAPI2Token: fortniteToken2,
		playersFile:       playersFile,
		pollTimeoutSecs:   pollTimeout,
		mongodbURI:        mongodbURI,
	}, nil
}

func startCron(provider statsProvider, store snapshotStore) {
	c := cron.New()
	_, err := c.AddFunc("0 0 * * *", func() {
		collectSnapshots(provider, store)
	})
	if err != nil {
		log.Fatalf("failed to schedule cron: %v", err)
	}
	c.Start()
	log.Println("Cron scheduler started, daily snapshots at 00:00.")
}

func collectSnapshots(provider statsProvider, store snapshotStore) {
	today := time.Now().UTC().Format("2006-01-02")
	log.Printf("Collecting daily snapshots for %s", today)

	for _, entry := range provider.Entries() {
		snapshot, err := provider.FetchFresh(entry)
		if err != nil {
			log.Printf("Failed to fetch stats for %s: %v", entry.Name, err)
			continue
		}

		daily := dailySnapshot{
			AccountID: entry.AccountID,
			Name:      entry.Name,
			Date:      today,
			Stats:     extractDailyStats(snapshot.stats),
			CreatedAt: time.Now().UTC(),
		}

		if err := store.UpsertSnapshot(daily); err != nil {
			log.Printf("Failed to store snapshot for %s: %v", entry.Name, err)
			continue
		}
		log.Printf("Stored snapshot for %s on %s", entry.Name, today)
	}
}

func runBot(client botClient, provider statsProvider, season seasonProvider, status statusProvider, store snapshotStore, pollTimeout int) error {
	var offset int

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
			msg := update.Message
			if msg == nil {
				continue
			}

			response := handleMessage(provider, season, status, store, msg.Text)
			if strings.TrimSpace(response) == "" {
				continue
			}

			if err := client.sendMessage(msg.Chat.ID, response); err != nil {
				log.Printf("send message failed: %v", err)
				time.Sleep(1 * time.Second)
				if retryErr := client.sendMessage(msg.Chat.ID, response); retryErr != nil {
					log.Printf("send message retry failed: %v", retryErr)
				}
			}
		}
	}
}
