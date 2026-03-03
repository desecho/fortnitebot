package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestNewTelegramClient(t *testing.T) {
	client := newTelegramClient("test-token")
	want := "https://api.telegram.org/bottest-token"
	if client.baseURL != want {
		t.Fatalf("baseURL = %q, want %q", client.baseURL, want)
	}
}

func TestTelegramGetUpdates(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		var receivedURL string
		client := newTestHTTPClient(func(r *http.Request) (*http.Response, error) {
			receivedURL = r.URL.String()
			body := `{"ok":true,"result":[{"update_id":123,"message":{"message_id":1,"text":"/help","chat":{"id":456}}}]}`
			return newTestResponse(r, http.StatusOK, body), nil
		})

		tc := &telegramClient{
			baseURL:    "http://example.invalid/bot123",
			httpClient: client,
		}

		updates, err := tc.getUpdates(10, 30)
		if err != nil {
			t.Fatalf("getUpdates() error = %v", err)
		}
		if len(updates) != 1 {
			t.Fatalf("len(updates) = %d, want 1", len(updates))
		}
		if updates[0].UpdateID != 123 {
			t.Fatalf("UpdateID = %d, want 123", updates[0].UpdateID)
		}
		if updates[0].Message.Text != "/help" {
			t.Fatalf("Text = %q, want /help", updates[0].Message.Text)
		}
		if updates[0].Message.Chat.ID != 456 {
			t.Fatalf("Chat.ID = %d, want 456", updates[0].Message.Chat.ID)
		}
		if !strings.Contains(receivedURL, "offset=10") {
			t.Fatalf("URL = %q, want substring offset=10", receivedURL)
		}
		if !strings.Contains(receivedURL, "timeout=30") {
			t.Fatalf("URL = %q, want substring timeout=30", receivedURL)
		}
	})

	t.Run("empty result", func(t *testing.T) {
		client := newTestHTTPClient(func(r *http.Request) (*http.Response, error) {
			return newTestResponse(r, http.StatusOK, `{"ok":true,"result":[]}`), nil
		})

		tc := &telegramClient{baseURL: "http://example.invalid/bot123", httpClient: client}
		updates, err := tc.getUpdates(0, 30)
		if err != nil {
			t.Fatalf("getUpdates() error = %v", err)
		}
		if len(updates) != 0 {
			t.Fatalf("len(updates) = %d, want 0", len(updates))
		}
	})

	t.Run("non-200 response", func(t *testing.T) {
		client := newTestHTTPClient(func(r *http.Request) (*http.Response, error) {
			return newTestResponse(r, http.StatusBadGateway, ""), nil
		})

		tc := &telegramClient{baseURL: "http://example.invalid/bot123", httpClient: client}
		_, err := tc.getUpdates(0, 30)
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "telegram returned 502") {
			t.Fatalf("error = %q, want substring 'telegram returned 502'", err.Error())
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		client := newTestHTTPClient(func(r *http.Request) (*http.Response, error) {
			return newTestResponse(r, http.StatusOK, `{invalid`), nil
		})

		tc := &telegramClient{baseURL: "http://example.invalid/bot123", httpClient: client}
		_, err := tc.getUpdates(0, 30)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("telegram error response", func(t *testing.T) {
		client := newTestHTTPClient(func(r *http.Request) (*http.Response, error) {
			return newTestResponse(r, http.StatusOK, `{"ok":false,"description":"Unauthorized"}`), nil
		})

		tc := &telegramClient{baseURL: "http://example.invalid/bot123", httpClient: client}
		_, err := tc.getUpdates(0, 30)
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "Unauthorized") {
			t.Fatalf("error = %q, want substring 'Unauthorized'", err.Error())
		}
	})
}

func TestTelegramSendMessage(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		var receivedBody telegramSendMessageRequest
		var receivedContentType string
		client := newTestHTTPClient(func(r *http.Request) (*http.Response, error) {
			receivedContentType = r.Header.Get("Content-Type")
			if err := json.NewDecoder(r.Body).Decode(&receivedBody); err != nil {
				t.Fatalf("decode request body: %v", err)
			}
			return newTestResponse(r, http.StatusOK, `{"ok":true}`), nil
		})

		tc := &telegramClient{baseURL: "http://example.invalid/bot123", httpClient: client}
		err := tc.sendMessage(456, "Hello, world!")
		if err != nil {
			t.Fatalf("sendMessage() error = %v", err)
		}

		if receivedContentType != "application/json" {
			t.Fatalf("Content-Type = %q, want application/json", receivedContentType)
		}
		if receivedBody.ChatID != "456" {
			t.Fatalf("ChatID = %q, want 456", receivedBody.ChatID)
		}
		if receivedBody.Text != "Hello, world!" {
			t.Fatalf("Text = %q, want Hello, world!", receivedBody.Text)
		}
	})

	t.Run("non-200 response", func(t *testing.T) {
		client := newTestHTTPClient(func(r *http.Request) (*http.Response, error) {
			return newTestResponse(r, http.StatusTooManyRequests, ""), nil
		})

		tc := &telegramClient{baseURL: "http://example.invalid/bot123", httpClient: client}
		err := tc.sendMessage(456, "Hello")
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "telegram returned 429") {
			t.Fatalf("error = %q, want substring 'telegram returned 429'", err.Error())
		}
	})

	t.Run("invalid json response", func(t *testing.T) {
		client := newTestHTTPClient(func(r *http.Request) (*http.Response, error) {
			return newTestResponse(r, http.StatusOK, `{invalid`), nil
		})

		tc := &telegramClient{baseURL: "http://example.invalid/bot123", httpClient: client}
		err := tc.sendMessage(456, "Hello")
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("telegram error response", func(t *testing.T) {
		client := newTestHTTPClient(func(r *http.Request) (*http.Response, error) {
			return newTestResponse(r, http.StatusOK, `{"ok":false,"description":"Bad Request: chat not found"}`), nil
		})

		tc := &telegramClient{baseURL: "http://example.invalid/bot123", httpClient: client}
		err := tc.sendMessage(456, "Hello")
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "Bad Request: chat not found") {
			t.Fatalf("error = %q, want substring 'Bad Request: chat not found'", err.Error())
		}
	})

	t.Run("uses POST method", func(t *testing.T) {
		var receivedMethod string
		client := newTestHTTPClient(func(r *http.Request) (*http.Response, error) {
			receivedMethod = r.Method
			return newTestResponse(r, http.StatusOK, `{"ok":true}`), nil
		})

		tc := &telegramClient{baseURL: "http://example.invalid/bot123", httpClient: client}
		_ = tc.sendMessage(456, "Hello")
		if receivedMethod != http.MethodPost {
			t.Fatalf("method = %q, want POST", receivedMethod)
		}
	})
}
