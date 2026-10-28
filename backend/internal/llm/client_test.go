package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// 校验发给网关的请求体，并确认原始请求/响应字节被完整带回（供 llm_calls 落库）。
func TestChatSendsExpectedPayload(t *testing.T) {
	var gotBody map[string]any
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"x","model":"deepseek-chat","choices":[{"index":0,"message":{"role":"assistant","content":"回答 [1]"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":3,"total_tokens":13}}`)
	}))
	defer srv.Close()

	c := New(srv.URL, "test-key", 5*time.Second)
	req := Request{
		Model: "deepseek-chat", Temperature: 0.2, MaxTokens: 900, Stream: false,
		Messages: []Message{{Role: "system", Content: "系统提示"}, {Role: "user", Content: "问题"}},
		Tools: []Tool{func() Tool {
			var tl Tool
			tl.Type = "function"
			tl.Function.Name = "word_count"
			tl.Function.Description = "统计字数"
			tl.Function.Parameters = map[string]any{"type": "object"}
			return tl
		}()},
	}
	resp, rawReq, rawResp, err := c.Chat(context.Background(), req)
	if err != nil {
		t.Fatalf("Chat 失败: %v", err)
	}
	if gotAuth != "Bearer test-key" {
		t.Fatalf("Authorization 头不符: %q", gotAuth)
	}
	if gotBody["model"] != "deepseek-chat" || gotBody["max_tokens"].(float64) != 900 {
		t.Fatalf("请求体字段不符: %+v", gotBody)
	}
	if len(gotBody["messages"].([]any)) != 2 {
		t.Fatalf("messages 数量不符: %+v", gotBody["messages"])
	}
	if len(gotBody["tools"].([]any)) != 1 {
		t.Fatalf("tools 应原样发送: %+v", gotBody["tools"])
	}
	if !strings.Contains(string(rawReq), "系统提示") {
		t.Fatal("返回的原始请求体应包含完整消息文本")
	}
	if !strings.Contains(string(rawResp), "回答 [1]") {
		t.Fatal("返回的原始响应体应保持原样")
	}
	if len(resp.Choices) != 1 || resp.Choices[0].Message.Content != "回答 [1]" {
		t.Fatalf("解析结果不符: %+v", resp)
	}
	if resp.Usage.TotalTokens != 13 {
		t.Fatalf("usage 未解析: %+v", resp.Usage)
	}
}

func TestChatGatewayError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":{"message":"bad key"}}`)
	}))
	defer srv.Close()

	_, _, rawResp, err := New(srv.URL, "bad", time.Second).Chat(context.Background(), Request{Model: "m"})
	if err == nil {
		t.Fatal("非 200 应返回错误")
	}
	if !strings.Contains(err.Error(), "401") || !strings.Contains(string(rawResp), "bad key") {
		t.Fatalf("错误信息应包含状态码与响应体，实际 err=%v raw=%s", err, rawResp)
	}
}
