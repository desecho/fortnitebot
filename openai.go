package main

import (
	"context"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

const aiSystemPrompt = "Given the fortnite stats, output the ranking of the players based on performance. The output should be like this:\n1. Player1\n2. Player2\n3. Player3\nIt should also include a short explanation in Russian (в блатном безбашенном стиле) of the ranking results. Do not include headers"

type openAIRankingProvider struct {
	client *openai.Client
}

func newOpenAIRankingProvider(token string) *openAIRankingProvider {
	client := openai.NewClient(option.WithAPIKey(token))
	return &openAIRankingProvider{client: &client}
}

func (p *openAIRankingProvider) Rank(statsText string) (string, error) {
	resp, err := p.client.Chat.Completions.New(context.Background(), openai.ChatCompletionNewParams{
		Model: "gpt-5",
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(aiSystemPrompt),
			openai.UserMessage(statsText),
		},
	})
	if err != nil {
		return "", err
	}
	if len(resp.Choices) == 0 {
		return "No response from AI.", nil
	}
	return resp.Choices[0].Message.Content, nil
}
