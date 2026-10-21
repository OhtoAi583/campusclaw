// Package mcp 实现一个最小的 MCP 客户端（stdio + JSON-RPC 2.0）。
//
// 只覆盖本平台需要的三段：initialize → tools/list → tools/call。
// 每次对话打开一个会话，结束后关闭；工具 schema 会原样放进 LLM 请求的 tools 字段。
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// Tool 是 MCP 工具定义（与 OpenAI function schema 兼容）。
type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id,omitempty"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *rpcError       `json:"error"`
}

// Session 是一个已初始化的 MCP 会话。
type Session struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
	mu     sync.Mutex
	nextID int
}

// Start 启动 MCP 服务器进程并完成 initialize 握手。
func Start(ctx context.Context, command string, args ...string) (*Session, error) {
	cmd := exec.CommandContext(ctx, command, args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("启动 MCP 服务器失败: %w", err)
	}
	s := &Session{cmd: cmd, stdin: stdin, stdout: bufio.NewReader(stdout), nextID: 1}
	if _, err := s.call(ctx, "initialize", map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "campusclaw", "version": "1.0.0"},
	}); err != nil {
		s.Close()
		return nil, fmt.Errorf("MCP initialize 失败: %w", err)
	}
	if err := s.notify("notifications/initialized", map[string]any{}); err != nil {
		s.Close()
		return nil, err
	}
	return s, nil
}

// Close 关闭会话并回收子进程。
func (s *Session) Close() {
	_ = s.stdin.Close()
	if s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
	}
	_, _ = s.cmd.Process.Wait()
}

// ListTools 拉取服务器暴露的工具。
func (s *Session) ListTools(ctx context.Context) ([]Tool, error) {
	raw, err := s.call(ctx, "tools/list", map[string]any{})
	if err != nil {
		return nil, err
	}
	var out struct {
		Tools []Tool `json:"tools"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("解析 tools/list 失败: %w", err)
	}
	return out.Tools, nil
}

// CallTool 调用一个工具，返回文本结果。
func (s *Session) CallTool(ctx context.Context, name string, args map[string]any) (string, error) {
	raw, err := s.call(ctx, "tools/call", map[string]any{"name": name, "arguments": args})
	if err != nil {
		return "", err
	}
	var out struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("解析 tools/call 失败: %w", err)
	}
	var b strings.Builder
	for _, c := range out.Content {
		if c.Type == "text" {
			b.WriteString(c.Text)
		}
	}
	if out.IsError {
		return b.String(), fmt.Errorf("工具返回错误: %s", b.String())
	}
	return b.String(), nil
}

func (s *Session) notify(method string, params any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	payload, _ := json.Marshal(rpcRequest{JSONRPC: "2.0", Method: method, Params: params})
	_, err := s.stdin.Write(append(payload, '\n'))
	return err
}

func (s *Session) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := s.nextID
	s.nextID++
	payload, err := json.Marshal(rpcRequest{JSONRPC: "2.0", ID: id, Method: method, Params: params})
	if err != nil {
		return nil, err
	}
	if _, err := s.stdin.Write(append(payload, '\n')); err != nil {
		return nil, err
	}

	done := make(chan struct{})
	var resp rpcResponse
	var readErr error
	go func() {
		line, err := s.stdout.ReadBytes('\n')
		if err != nil {
			readErr = err
		} else if err := json.Unmarshal(line, &resp); err != nil {
			readErr = err
		}
		close(done)
	}()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(20 * time.Second):
		return nil, fmt.Errorf("MCP 调用超时: %s", method)
	case <-done:
	}
	if readErr != nil {
		return nil, readErr
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("MCP 错误 %d: %s", resp.Error.Code, resp.Error.Message)
	}
	return resp.Result, nil
}
