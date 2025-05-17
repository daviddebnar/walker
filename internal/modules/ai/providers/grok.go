package providers

import (
	"bytes"
	"encoding/json"
	"log"
	"log/slog"
	"net/http"
	"os"

	"github.com/abenz1267/walker/internal/config"
	"github.com/abenz1267/walker/internal/util"

	"github.com/diamondburned/gotk4/pkg/core/gioutil"
)

const (
	GROK_API_URL = "https://api.x.ai/v1/chat/completions"
	GROK_API_KEY = "GROK_API_KEY"
)

type GrokProvider struct {
	config      config.AI
	key         string
	specialFunc func(args ...interface{})
}

func NewGrokProvider(config config.AI, specialFunc func(...interface{})) Provider {
	key := os.Getenv(GROK_API_KEY)
	if key == "" {
		log.Println("grok: no api key set")
		return nil
	}
	return &GrokProvider{
		config:      config,
		key:         os.Getenv(GROK_API_KEY),
		specialFunc: specialFunc,
	}
}

func (p *GrokProvider) SetupData() []util.Entry {
	var entries []util.Entry

	for _, v := range p.config.Grok.Prompts {
		entries = append(entries, util.Entry{
			Label:            v.Label,
			Sub:              "Grok",
			Exec:             "",
			RecalculateScore: true,
			Matching:         util.Fuzzy,
			SpecialFunc:      p.specialFunc,
			SpecialFuncArgs:  []interface{}{"grok", &v},
			SingleModuleOnly: v.SingleModuleOnly,
		})
	}

	if len(p.config.Grok.Prompts) == 0 {
		log.Println("grok: no prompts set.")
	}

	return entries
}

type GrokRequest struct {
	Model       string    `json:"model"`
	MaxTokens   int       `json:"max_tokens"`
	Temperature float64   `json:"temperature"`
	Messages    []Message `json:"messages"`
}

type GrokResponse struct {
	Id                string `json:"id"`
	Model             string `json:"model"`
	Object            string `json:"object"`
	SystemFingerprint string `json:"system_fingerprint"`
	Created           int    `json:"created"`
	Choices           []struct {
		Index         int    `json:"index"`
		FinishSection string `json:"finish_reason"`
		Message       struct {
			Role             string `json:"role"`
			Content          string `json:"content"`
			ReasoningContent string `json:"reasoning_content"`
			Refusal          string `json:"refusal"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens        int `json:"prompt_tokens"`
		CompletionTokens    int `json:"completion_tokens"`
		TotalTokens         int `json:"total_tokens"`
		PromptTokensDetails struct {
			TextTokens   int `json:"text_tokens"`
			AudioTokens  int `json:"audio_tokens"`
			ImageTokens  int `json:"image_tokens"`
			CachedTokens int `json:"cached_tokens"`
		} `json:"prompt_tokens_details"`
		CompletionTokensDetails struct {
			ReasoningTokens          int `json:"reasoning_tokens"`
			AudioTokens              int `json:"audio_tokens"`
			AcceptedPredictionTokens int `json:"accepted_prediction_tokens"`
			RejectedPredictionTokens int `json:"rejected_prediction_tokens"`
		} `json:"completion_tokens_details"`
	} `json:"usage"`
}

func (p *GrokProvider) Query(query string, currentMessages *[]Message, currentPrompt *config.AIPrompt, items *gioutil.ListModel[Message]) {
	queryMsg := Message{
		Role:    "user",
		Content: query,
	}

	messages := []Message{}

	if currentMessages != nil && len(*currentMessages) > 0 {
		messages = *currentMessages
	}

	messages = append(messages, queryMsg)

	items.Splice(0, int(items.NItems()), messages...)

	req := GrokRequest{
		Model:       currentPrompt.Model,
		MaxTokens:   currentPrompt.MaxTokens,
		Temperature: currentPrompt.Temperature,
		Messages:    messages,
	}

	b, err := json.Marshal(req)
	if err != nil {
		log.Panicln(err)
	}

	request, err := http.NewRequest("POST", GROK_API_URL, bytes.NewBuffer(b))
	if err != nil {
		log.Panicln(err)
	}
	request.Header.Set("Authorization", "Bearer "+p.key)
	request.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(request)
	if err != nil {
		slog.Error("Error making request: %v", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		slog.Error("Grok API returned unexpected status code %d", resp.StatusCode)
		return
	}

	var grokResp GrokResponse

	err = json.NewDecoder(resp.Body).Decode(&grokResp)
	if err != nil {
		slog.Error("Error decoding response: %v", err)
		return
	}

	responseMessages := []Message{}

	for _, v := range grokResp.Choices {
		responseMessages = append(responseMessages, Message{
			Role:    "assistant",
			Content: v.Message.Content,
		})
	}

	messages = append(messages, responseMessages...)
	*currentMessages = messages

}
