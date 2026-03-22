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
	openAIToken       string
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

	var ranker rankingProvider
	if cfg.openAIToken != "" {
		ranker = newOpenAIRankingProvider(cfg.openAIToken)
		log.Println("OpenAI configured, AI ranking enabled.")
	} else {
		log.Println("OPENAI_TOKEN not set, AI ranking disabled.")
	}

	seasonProvider := newFortniteAPISeasonProvider(cfg.fortniteAPI2Token)
	statusSource := newEpicStatusProvider()
	client, err := newTelegramBotClient(cfg.botToken)
	if err != nil {
		log.Fatal(err)
	}
	if err := runBot(client, provider, seasonProvider, statusSource, store, ranker, cfg.pollTimeoutSecs); err != nil {
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
	openAIToken := strings.TrimSpace(os.Getenv("OPENAI_TOKEN"))

	return appConfig{
		botToken:          token,
		fortniteAPIToken:  fortniteToken,
		fortniteAPI2Token: fortniteToken2,
		playersFile:       playersFile,
		pollTimeoutSecs:   pollTimeout,
		mongodbURI:        mongodbURI,
		openAIToken:       openAIToken,
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
	log.Print(collectSnapshotsReport(provider, store))
}

func runBot(client botClient, provider statsProvider, season seasonProvider, status statusProvider, store snapshotStore, ranker rankingProvider, pollTimeout int) error {
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

			if msg.ReplyToMessage != nil && msg.ReplyToMessage.From != nil && msg.ReplyToMessage.From.ID == client.botUserID() {
				continue
			}

			response := handleMessage(provider, season, status, store, ranker, msg.Text)
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
