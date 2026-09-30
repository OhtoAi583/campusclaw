package answer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"campusclaw/backend/internal/search"
)

// GatewayClient 调用兼容 OpenAI 的对话网关（本课为 DeepSeek）。
//
// 只把检索到的切片交给模型：消息体由服务端自己组装，客户端无法注入 system 消息。
// 密钥只存在于服务端环境变量中，不下发浏览器。
type GatewayClient struct {
	BaseURL string
	APIKey  string
	Model   string
	HTTP    *http.Client
}

// NewGatewayClient 构造对话网关客户端。timeout 是单次生成的整体超时。
func NewGatewayClient(baseURL, apiKey, model string, timeout time.Duration) *GatewayClient {
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	return &GatewayClient{
		BaseURL: strings.TrimRight(baseURL, "/"),
		APIKey:  apiKey,
		Model:   model,
		HTTP:    &http.Client{Timeout: timeout},
	}
}

// Name 返回引擎名，便于前端与日志区分回答来源。
func (g *GatewayClient) Name() string { return "gateway:" + g.Model }

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
	MaxTokens   int           `json:"max_tokens"`
	Stream      bool          `json:"stream"`
}

type chatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// Answer 组装提示词并请求生成。提示词里只有材料标题、切片序号、切片正文与用户提问。
func (g *GatewayClient) Answer(ctx context.Context, question string, chunks []search.Result) (string, error) {
	system, user := BuildPrompt(question, chunks)
	payload, err := json.Marshal(chatRequest{
		Model: g.Model,
		Messages: []chatMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
		// 温度压低：本课要的是"依据片段回答"，不是创作。
		Temperature: 0.2,
		MaxTokens:   500,
		Stream:      false,
	})
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.BaseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+g.APIKey)

	resp, err := g.HTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("调用对话网关失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("读取网关响应失败: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("对话网关返回 %d: %s", resp.StatusCode, truncate(string(body), 200))
	}

	var parsed chatResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("解析网关响应失败: %w", err)
	}
	if parsed.Error != nil {
		return "", fmt.Errorf("对话网关报错: %s", parsed.Error.Message)
	}
	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("对话网关未返回任何候选")
	}
	answer := strings.TrimSpace(parsed.Choices[0].Message.Content)
	if answer == "" {
		return "", fmt.Errorf("对话网关返回了空回答")
	}
	return answer, nil
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
