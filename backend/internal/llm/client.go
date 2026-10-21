// Package llm 封装与兼容 OpenAI 的对话网关的交互。
// 它只负责"发什么、收到什么"，不掺业务逻辑——这样请求体可以被原样记录下来做验收。
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// ToolCall 是模型返回的工具调用。
type ToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// Message 是对话消息。工具结果用 Role=tool + ToolCallID 回填。
type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	Name       string     `json:"name,omitempty"`
}

// Tool 是提供给模型的工具定义（function calling schema）。
type Tool struct {
	Type     string `json:"type"`
	Function struct {
		Name        string         `json:"name"`
		Description string         `json:"description"`
		Parameters  map[string]any `json:"parameters"`
	} `json:"function"`
}

// Request 是发给 /chat/completions 的完整请求体。
type Request struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Temperature float64   `json:"temperature"`
	MaxTokens   int       `json:"max_tokens"`
	Tools       []Tool    `json:"tools,omitempty"`
	Stream      bool      `json:"stream"`
}

// Usage 是 token 用量。
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// Response 是网关返回体（只保留本平台需要的字段）。
type Response struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Choices []struct {
		Index   int     `json:"index"`
		Message Message `json:"message"`
		Finish  string  `json:"finish_reason"`
	} `json:"choices"`
	Usage Usage `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// Client 是对话网关客户端。
type Client struct {
	BaseURL string
	APIKey  string
	HTTP    *http.Client
}

// New 构造客户端。
func New(baseURL, apiKey string, timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	return &Client{BaseURL: baseURL, APIKey: apiKey, HTTP: &http.Client{Timeout: timeout}}
}

// Chat 发送一次请求，同时返回原始请求体与原始响应体，供上层落库。
func (c *Client) Chat(ctx context.Context, req Request) (Response, []byte, []byte, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return Response{}, nil, nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return Response{}, body, nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)

	resp, err := c.HTTP.Do(httpReq)
	if err != nil {
		return Response{}, body, nil, fmt.Errorf("调用对话网关失败: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return Response{}, body, raw, err
	}
	if resp.StatusCode != http.StatusOK {
		return Response{}, body, raw, fmt.Errorf("对话网关返回 %d: %s", resp.StatusCode, truncate(string(raw), 300))
	}
	var parsed Response
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return Response{}, body, raw, fmt.Errorf("解析网关响应失败: %w", err)
	}
	if parsed.Error != nil {
		return Response{}, body, raw, fmt.Errorf("对话网关报错: %s", parsed.Error.Message)
	}
	if len(parsed.Choices) == 0 {
		return Response{}, body, raw, fmt.Errorf("对话网关未返回候选")
	}
	return parsed, body, raw, nil
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
