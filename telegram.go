package main

import (
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type telegramBotClient struct {
	api *tgbotapi.BotAPI
}

func newTelegramBotClient(token string) (*telegramBotClient, error) {
	api, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, err
	}
	return &telegramBotClient{api: api}, nil
}

func (c *telegramBotClient) getUpdates(offset int, timeoutSecs int) ([]tgbotapi.Update, error) {
	cfg := tgbotapi.NewUpdate(offset)
	cfg.Timeout = timeoutSecs
	return c.api.GetUpdates(cfg)
}

func (c *telegramBotClient) botUserID() int64 {
	return c.api.Self.ID
}

func (c *telegramBotClient) sendMessage(chatID int64, text string) error {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = tgbotapi.ModeHTML
	_, err := c.api.Send(msg)
	return err
}

func (c *telegramBotClient) setCommands(commands []tgbotapi.BotCommand) error {
	cfg := tgbotapi.NewSetMyCommands(commands...)
	_, err := c.api.Request(cfg)
	return err
}
