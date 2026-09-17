// Пакет llm: клиент llama.cpp-сервера (OpenAI-совместимый
// /v1/chat/completions), сборка промта, парсинг ответа и добор невалидных
// комбинаций повторными запросами.
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"fortunata/internal/generate"
)

const (
	// requestTimeout — лимит одного запроса к LLM; должен оставаться меньше
	// WriteTimeout HTTP-сервера в cmd/server/main.go, иначе ответ
	// AI-генерации закоммитится, а до клиента не дойдёт.
	requestTimeout = 120 * time.Second
	// maxTokens — запас на <think>-рассуждения qwen3 и 20 комбинаций.
	maxTokens = 4096
	// temperature — умеренная случайность предложений.
	temperature = 0.8
	// maxRefills — число доборов после первого запроса.
	maxRefills = 2
)

// ErrUnavailable — LLM-сервер недоступен (сеть, таймаут, отказ в соединении).
var ErrUnavailable = errors.New("LLM-сервер недоступен")

// Client — клиент llama.cpp-сервера; http.Client подменяемый для тестов.
type Client struct {
	baseURL string // без пути, напр. http://192.168.1.128:18020
	model   string
	apiKey  string
	hc      *http.Client
}

// NewClient собирает клиент; hc == nil заменяется на клиент с requestTimeout.
func NewClient(baseURL, model, apiKey string, hc *http.Client) *Client {
	if hc == nil {
		hc = &http.Client{Timeout: requestTimeout}
	}
	return &Client{
		baseURL: strings.TrimSuffix(baseURL, "/"),
		model:   model,
		apiKey:  apiKey,
		hc:      hc,
	}
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
	MaxTokens   int           `json:"max_tokens"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// Complete отправляет system+user промт и возвращает текст ответа модели.
func (c *Client) Complete(ctx context.Context, userPrompt string) (string, error) {
	body, err := json.Marshal(chatRequest{
		Model:       c.model,
		Messages:    []chatMessage{{Role: "system", Content: systemPrompt}, {Role: "user", Content: userPrompt}},
		Temperature: temperature,
		MaxTokens:   maxTokens,
	})
	if err != nil {
		return "", fmt.Errorf("кодирование запроса: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("создание запроса: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("сервер LLM ответил %d", resp.StatusCode)
	}
	var cr chatResponse
	// Лимит тела — гигиена: ожидаемый ответ на 20 комбинаций — десятки КБ.
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&cr); err != nil {
		return "", fmt.Errorf("ответ LLM не JSON: %w", err)
	}
	if len(cr.Choices) == 0 || cr.Choices[0].Message.Content == "" {
		return "", errors.New("пустой ответ LLM")
	}
	return cr.Choices[0].Message.Content, nil
}

// Propose запрашивает count валидных комбинаций; если валидных меньше,
// добирает недостающее повторными запросами (максимум maxRefills) с
// exclude-списком уже найденных. Сетевые ошибки пробрасываются сразу.
// Может вернуть меньше count (вплоть до нуля) — решение об ошибке
// принимает вызывающий.
func (c *Client) Propose(ctx context.Context, count int, main, bonus []int, totalDraws int) ([]generate.Ticket, error) {
	seen := make(map[string]struct{}, count)
	tickets := make([]generate.Ticket, 0, count)
	for attempt := 0; len(tickets) < count && attempt <= maxRefills; attempt++ {
		var exclude []generate.Ticket
		if attempt > 0 {
			exclude = tickets
		}
		content, err := c.Complete(ctx, ComposePrompt(count-len(tickets), main, bonus, totalDraws, exclude))
		if err != nil {
			return nil, err
		}
		for _, t := range ExtractTickets(content) {
			key := generate.Key(t)
			if _, dup := seen[key]; dup {
				continue
			}
			seen[key] = struct{}{}
			tickets = append(tickets, t)
		}
	}
	return tickets, nil
}
