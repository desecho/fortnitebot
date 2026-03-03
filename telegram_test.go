package main

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type fakeBotClient struct {
	batches   [][]tgbotapi.Update
	batchIdx  int
	sent      []sentMessage
	sendCalls int
	sendErrs  []error // consumed in order; nil or empty means success
	done      chan struct{}
	mu        sync.Mutex
}

type sentMessage struct {
	chatID int64
	text   string
}

func newFakeBotClient(batches ...[]tgbotapi.Update) *fakeBotClient {
	return &fakeBotClient{
		batches: batches,
		done:    make(chan struct{}),
	}
}

func (f *fakeBotClient) getUpdates(offset int, timeoutSecs int) ([]tgbotapi.Update, error) {
	f.mu.Lock()
	idx := f.batchIdx
	f.batchIdx++
	f.mu.Unlock()

	if idx >= len(f.batches) {
		if idx == len(f.batches) {
			close(f.done)
		}
		time.Sleep(time.Hour) // block to prevent busy loop
		return nil, fmt.Errorf("stopped")
	}
	return f.batches[idx], nil
}

func (f *fakeBotClient) sendMessage(chatID int64, text string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.sendCalls++
	if len(f.sendErrs) > 0 {
		err := f.sendErrs[0]
		f.sendErrs = f.sendErrs[1:]
		if err != nil {
			return err
		}
	}
	f.sent = append(f.sent, sentMessage{chatID: chatID, text: text})
	return nil
}

func newMessageUpdate(updateID int, chatID int64, text string) tgbotapi.Update {
	return tgbotapi.Update{
		UpdateID: updateID,
		Message: &tgbotapi.Message{
			Text: text,
			Chat: &tgbotapi.Chat{ID: chatID},
		},
	}
}

func waitForDone(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for runBot to process all batches")
	}
}

func TestRunBotForwardsMessages(t *testing.T) {
	fake := newFakeBotClient(
		[]tgbotapi.Update{newMessageUpdate(1, 100, "/help")},
	)
	go runBot(fake, stubStatsProvider{}, stubSeasonProvider{}, stubStatusProvider{}, 1)
	waitForDone(t, fake.done)

	fake.mu.Lock()
	defer fake.mu.Unlock()

	if len(fake.sent) != 1 {
		t.Fatalf("sent %d messages, want 1", len(fake.sent))
	}
	if fake.sent[0].chatID != 100 {
		t.Fatalf("chatID = %d, want 100", fake.sent[0].chatID)
	}
	if !strings.Contains(fake.sent[0].text, "/stats") {
		t.Fatalf("response = %q, want help text containing /stats", fake.sent[0].text)
	}
}

func TestRunBotSkipsNilMessage(t *testing.T) {
	fake := newFakeBotClient(
		[]tgbotapi.Update{{UpdateID: 1, Message: nil}},
	)
	go runBot(fake, stubStatsProvider{}, stubSeasonProvider{}, stubStatusProvider{}, 1)
	waitForDone(t, fake.done)

	fake.mu.Lock()
	defer fake.mu.Unlock()

	if len(fake.sent) != 0 {
		t.Fatalf("sent %d messages, want 0", len(fake.sent))
	}
}

func TestRunBotRetriesOnSendError(t *testing.T) {
	fake := newFakeBotClient(
		[]tgbotapi.Update{newMessageUpdate(1, 100, "/help")},
	)
	fake.sendErrs = []error{fmt.Errorf("network error")} // first call fails, retry succeeds

	go runBot(fake, stubStatsProvider{}, stubSeasonProvider{}, stubStatusProvider{}, 1)
	waitForDone(t, fake.done)

	fake.mu.Lock()
	defer fake.mu.Unlock()

	if fake.sendCalls != 2 {
		t.Fatalf("sendCalls = %d, want 2 (initial + retry)", fake.sendCalls)
	}
	if len(fake.sent) != 1 {
		t.Fatalf("sent %d messages, want 1 (retry should succeed)", len(fake.sent))
	}
}
