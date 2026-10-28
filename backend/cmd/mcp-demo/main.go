// Command mcp-demo 是一个最小 MCP 服务器（stdio + JSON-RPC 2.0），
// 供"学情分析"应用演示外部工具接入：暴露 word_count 与 text_stats 两个工具。
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"unicode"
)

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type response struct {
	JSONRPC string `json:"jsonrpc"`
	ID      any    `json:"id,omitempty"`
	Result  any    `json:"result,omitempty"`
	Error   any    `json:"error,omitempty"`
}

var tools = []map[string]any{
	{
		"name":        "word_count",
		"description": "统计一段中文文本的字数与段落数",
		"inputSchema": map[string]any{
			"type":       "object",
			"properties": map[string]any{"text": map[string]any{"type": "string", "description": "待统计的文本"}},
			"required":   []string{"text"},
		},
	},
	{
		"name":        "text_stats",
		"description": "统计文本的字数、句数与平均句长，用于学情分析",
		"inputSchema": map[string]any{
			"type":       "object",
			"properties": map[string]any{"text": map[string]any{"type": "string", "description": "待统计的文本"}},
			"required":   []string{"text"},
		},
	},
}

func main() {
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 1024*1024), 4*1024*1024)
	enc := json.NewEncoder(os.Stdout)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var req request
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			continue
		}
		var id any
		if len(req.ID) > 0 {
			_ = json.Unmarshal(req.ID, &id)
		}
		switch req.Method {
		case "initialize":
			_ = enc.Encode(response{JSONRPC: "2.0", ID: id, Result: map[string]any{
				"protocolVersion": "2024-11-05",
				"capabilities":    map[string]any{"tools": map[string]any{}},
				"serverInfo":      map[string]any{"name": "campusclaw-demo-tools", "version": "1.0.0"},
			}})
		case "notifications/initialized":
			// 通知不需要响应
		case "tools/list":
			_ = enc.Encode(response{JSONRPC: "2.0", ID: id, Result: map[string]any{"tools": tools}})
		case "tools/call":
			var p struct {
				Name      string         `json:"name"`
				Arguments map[string]any `json:"arguments"`
			}
			_ = json.Unmarshal(req.Params, &p)
			text, _ := p.Arguments["text"].(string)
			out, isErr := run(p.Name, text)
			_ = enc.Encode(response{JSONRPC: "2.0", ID: id, Result: map[string]any{
				"content": []map[string]any{{"type": "text", "text": out}},
				"isError": isErr,
			}})
		default:
			_ = enc.Encode(response{JSONRPC: "2.0", ID: id, Error: map[string]any{
				"code": -32601, "message": "method not found: " + req.Method,
			}})
		}
	}
}

func run(name, text string) (string, bool) {
	// 字数只统计非空白字符，避免把换行与缩进算进去。
	chars := 0
	for _, r := range text {
		if !unicode.IsSpace(r) {
			chars++
		}
	}
	switch name {
	case "word_count":
		paragraphs := 0
		for _, p := range strings.Split(text, "\n") {
			if strings.TrimSpace(p) != "" {
				paragraphs++
			}
		}
		return fmt.Sprintf("字数：%d；非空段落：%d", chars, paragraphs), false
	case "text_stats":
		sentences := 0
		for _, r := range text {
			if r == '。' || r == '！' || r == '？' || r == '.' || r == '!' || r == '?' {
				sentences++
			}
		}
		if sentences == 0 && chars > 0 {
			sentences = 1
		}
		avg := 0
		if sentences > 0 {
			avg = chars / sentences
		}
		return fmt.Sprintf("字数：%d；句数：%d；平均句长：%d 字", chars, sentences, avg), false
	}
	return "未知工具", true
}
