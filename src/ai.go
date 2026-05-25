// ai.go
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

var aiTimeout = 120 * time.Second
var aiMaxTokens = 4096
var maxRetries = 3

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatRequest struct {
	Model       string        `json:"model"`
	Messages    []ChatMessage `json:"messages"`
	MaxTokens   int           `json:"max_tokens"`
	Temperature float64       `json:"temperature"`
}

func CallAI(apiURL, apiKey, model, systemPrompt, userPrompt string) (string, error) {
	messages := []ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}
	return CallAIMessages(apiURL, apiKey, model, messages)
}

func CallAIMessages(apiURL, apiKey, model string, messages []ChatMessage) (string, error) {
	client := &http.Client{Timeout: aiTimeout}

	reqBody := ChatRequest{
		Model:       model,
		Messages:    messages,
		MaxTokens:   aiMaxTokens,
		Temperature: 0.3,
	}

	for attempt := 1; attempt <= maxRetries; attempt++ {
		result, err := doAIRequest(client, apiURL, apiKey, reqBody)
		if err == nil {
			return result, nil
		}
		if attempt < maxRetries {
			time.Sleep(time.Duration(attempt*2) * time.Second)
		}
	}
	return "", fmt.Errorf("AI 请求失败，已重试 %d 次", maxRetries)
}

func doAIRequest(client *http.Client, apiURL, apiKey string, reqBody ChatRequest) (string, error) {
	body, _ := json.Marshal(reqBody)

	url := buildChatURL(apiURL)

	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	if len(result.Choices) == 0 {
		return "", fmt.Errorf("AI 返回空响应")
	}

	return result.Choices[0].Message.Content, nil
}

func buildChatURL(apiURL string) string {
	// 去掉末尾斜杠
	apiURL = strings.TrimRight(apiURL, "/")

	// 已经包含 /chat/completions，直接返回
	if strings.HasSuffix(apiURL, "/chat/completions") {
		return apiURL
	}

	// 已经以 /v1 结尾，补 chat/completions
	if strings.HasSuffix(apiURL, "/v1") {
		return apiURL + "/chat/completions"
	}

	// 都不包含，补 /v1/chat/completions
	return apiURL + "/v1/chat/completions"
}
