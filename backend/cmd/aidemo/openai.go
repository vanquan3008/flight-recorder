package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"
)

const openAIResponsesEndpoint = "https://api.openai.com/v1/responses"

var (
	errModelTimeout = errors.New("model provider timed out")
	errModelFailed  = errors.New("model provider failed")
)

type openAIModel struct {
	name     string
	apiKey   string
	endpoint string
	client   *http.Client
}

type modelResult struct {
	answer string
	model  string
	usage  *modelUsage
}

type modelUsage struct {
	InputTokens  *int64 `json:"input_tokens"`
	OutputTokens *int64 `json:"output_tokens"`
}

func configuredOpenAIModel(name, key string) (*openAIModel, error) {
	name, key = strings.TrimSpace(name), strings.TrimSpace(key)
	if name == "" {
		return nil, nil
	}
	if key == "" {
		return nil, errors.New("set TRACEPILOT_AI_DEMO_OPENAI_API_KEY for the configured sandbox model")
	}
	return &openAIModel{
		name: name, apiKey: key, endpoint: openAIResponsesEndpoint,
		client: &http.Client{
			Timeout:       6 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}, nil
}

func (model *openAIModel) generate(ctx context.Context, instructions, input string) (modelResult, error) {
	requestBody, err := json.Marshal(struct {
		Model           string `json:"model"`
		Instructions    string `json:"instructions"`
		Input           string `json:"input"`
		MaxOutputTokens int    `json:"max_output_tokens"`
		Store           bool   `json:"store"`
	}{Model: model.name, Instructions: instructions, Input: input, MaxOutputTokens: 256, Store: false})
	if err != nil {
		return modelResult{}, errModelFailed
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, model.endpoint, bytes.NewReader(requestBody))
	if err != nil {
		return modelResult{}, errModelFailed
	}
	request.Header.Set("Authorization", "Bearer "+model.apiKey)
	request.Header.Set("Content-Type", "application/json")
	response, err := model.client.Do(request)
	if err != nil {
		return modelResult{}, modelRequestError(err)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusRequestTimeout || response.StatusCode == http.StatusGatewayTimeout {
		return modelResult{}, errModelTimeout
	}
	if response.StatusCode != http.StatusOK {
		return modelResult{}, errModelFailed
	}

	const maxResponseBytes = 64 << 10
	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return modelResult{}, modelRequestError(err)
	}
	var payload struct {
		Model  string      `json:"model"`
		Status string      `json:"status"`
		Usage  *modelUsage `json:"usage"`
		Output []struct {
			Type    string `json:"type"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
	}
	if len(body) > maxResponseBytes || json.Unmarshal(body, &payload) != nil {
		return modelResult{}, errModelFailed
	}
	result := modelResult{model: payload.Model, usage: payload.Usage}
	if payload.Status != "completed" {
		return result, errModelFailed
	}
	var answer strings.Builder
	for _, item := range payload.Output {
		if item.Type != "message" {
			continue
		}
		for _, content := range item.Content {
			switch content.Type {
			case "output_text":
				answer.WriteString(content.Text)
			case "refusal":
				return result, errModelFailed
			}
		}
	}
	result.answer = strings.TrimSpace(answer.String())
	if result.answer == "" {
		return result, errModelFailed
	}
	return result, nil
}

func modelRequestError(err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return errModelTimeout
	}
	return errModelFailed
}
