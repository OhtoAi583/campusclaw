// Package chat 负责一次"应用对话"的编排：
// 解析应用 → 渲染提示词模板 → 检索本班知识库 → 注入技能 → 挂载 MCP 工具 → 调用模型（含工具循环）→ 落库取证。
package chat

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"campusclaw/backend/internal/llm"
	"campusclaw/backend/internal/mcp"
	"campusclaw/backend/internal/platform"
	"campusclaw/backend/internal/search"
)

// MaxSkillChars 是注入 system prompt 的技能正文总长度上限。
const MaxSkillChars = 12000

// MaxToolRounds 是工具调用循环的最大轮数。
const MaxToolRounds = 3

// Citation 是一条出处。
type Citation struct {
	Index         int     `json:"index"`
	MaterialID    int64   `json:"material_id"`
	MaterialTitle string  `json:"material_title"`
	OriginalName  string  `json:"original_name"`
	ChunkIndex    int     `json:"chunk_index"`
	StartOffset   int     `json:"start_offset"`
	EndOffset     int     `json:"end_offset"`
	Snippet       string  `json:"snippet"`
	Score         float64 `json:"score"`
}

// Meta 描述本次请求"到底带了什么"，直接对应验收问题。
type Meta struct {
	AppID         string   `json:"app_id"`
	AppName       string   `json:"app_name"`
	Model         string   `json:"model"`
	Temperature   float64  `json:"temperature"`
	MaxTokens     int      `json:"max_tokens"`
	TemplateID    string   `json:"template_id"`
	TemplateUsed  bool     `json:"template_used"`
	KBEnabled     bool     `json:"kb_enabled"`
	KBChunks      int      `json:"kb_chunks"`
	KBChars       int      `json:"kb_chars"`
	KBTotalChunks int      `json:"kb_total_chunks"`
	SkillIDs      []string `json:"skill_ids"`
	SkillChars    int      `json:"skill_chars"`
	ToolNames     []string `json:"tool_names"`
	LLMCalls      []int64  `json:"llm_call_ids"`
	ToolRounds    int      `json:"tool_rounds"`
	LatencyMS     int64    `json:"latency_ms"`
}

// Result 是一次对话的结果。
type Result struct {
	Answer         string     `json:"answer"`
	Citations      []Citation `json:"citations"`
	ConversationID string     `json:"conversation_id"`
	Meta           Meta       `json:"meta"`
}

type mcpSession struct {
	server string
	conn   *mcp.Session
}

// Service 编排一次对话。
type Service struct {
	Platform *platform.Store
	Search   *search.Service
	LLM      *llm.Client
}

// Ask 执行一次应用对话。
func (s *Service) Ask(ctx context.Context, appID string, classID, userID int64, classSlug, question, conversationID string) (Result, error) {
	started := time.Now()
	app, err := s.Platform.GetApp(ctx, appID)
	if err != nil {
		return Result{}, err
	}

	// 1) 知识库：只取命中的 top-k 切片，不把整库塞给模型。
	var hits []search.Result
	var totalChunks int
	if app.KBEnabled {
		hits, totalChunks, err = s.Search.Search(ctx, classID, question, app.KBTopK)
		if err != nil {
			return Result{}, err
		}
	}
	contextText, citations := buildContext(hits)

	// 2) 提示词模板：渲染后**原样**作为 system / user 消息发出去。
	system := render(app.Template.SystemTemplate, map[string]string{
		"app_name": app.Name, "class_name": classSlug, "question": question, "context": contextText,
	})
	user := render(app.Template.UserTemplate, map[string]string{
		"app_name": app.Name, "class_name": classSlug, "question": question, "context": contextText,
	})

	// 3) 技能：启用技能的**完整正文**拼进 system prompt（有总长度上限）。
	skillIDs := make([]string, 0, len(app.Skills))
	skillChars := 0
	var skillBuf strings.Builder
	for _, sk := range app.Skills {
		body := strings.TrimSpace(sk.Body)
		if skillChars+len([]rune(body)) > MaxSkillChars {
			body = string([]rune(body)[:maxInt(0, MaxSkillChars-skillChars)])
		}
		skillIDs = append(skillIDs, sk.ID)
		skillChars += len([]rune(body))
		fmt.Fprintf(&skillBuf, "\n\n## 已启用技能：%s\n\n%s", sk.Name, body)
	}
	if skillBuf.Len() > 0 {
		system += skillBuf.String()
	}

	messages := []llm.Message{
		{Role: "system", Content: system},
		{Role: "user", Content: user},
	}

	// 4) MCP：建立会话、拉取工具 schema，并原样放进请求的 tools 字段。
	var sessions []mcpSession
	var tools []llm.Tool
	toolOwner := map[string]string{}
	for _, srv := range app.MCPServers {
		conn, err := mcp.Start(ctx, srv.Command, splitArgs(srv.Args)...)
		if err != nil {
			slog.Error("连接 MCP 服务器失败", "server", srv.ID, "error", err)
			continue
		}
		sessions = append(sessions, mcpSession{server: srv.ID, conn: conn})
		list, err := conn.ListTools(ctx)
		if err != nil {
			slog.Error("拉取 MCP 工具失败", "server", srv.ID, "error", err)
			continue
		}
		for _, t := range list {
			var tool llm.Tool
			tool.Type = "function"
			tool.Function.Name = t.Name
			tool.Function.Description = t.Description
			tool.Function.Parameters = t.InputSchema
			tools = append(tools, tool)
			toolOwner[t.Name] = srv.ID
		}
	}
	defer func() {
		for _, s := range sessions {
			s.conn.Close()
		}
	}()
	toolNames := make([]string, 0, len(tools))
	for _, t := range tools {
		toolNames = append(toolNames, t.Function.Name)
	}
	sort.Strings(toolNames)

	// 5) 调模型：有 tool_calls 就执行工具并把结果回填，最多 MaxToolRounds 轮。
	var callIDs []int64
	var answer string
	toolRounds := 0
	for round := 0; round <= MaxToolRounds; round++ {
		req := llm.Request{
			Model: app.Model, Messages: messages,
			Temperature: app.Temperature, MaxTokens: app.MaxTokens,
			Tools: tools, Stream: false,
		}
		startedCall := time.Now()
		resp, reqBody, respBody, err := s.LLM.Chat(ctx, req)
		latency := time.Since(startedCall).Milliseconds()
		if err != nil {
			_, _ = s.Platform.DB.ExecContext(ctx, `
				INSERT INTO llm_calls (app_id, user_id, class_id, conversation_id, model, request_json, response_json,
					status, latency_ms, tool_rounds, round_index, kb_chunks, kb_chars, skill_chars, tools_json)
				VALUES (?, ?, ?, ?, ?, ?, ?, 'error', ?, ?, ?, ?, ?, ?, ?)`,
				app.ID, userID, classID, conversationID, app.Model, string(reqBody), string(respBody),
				latency, toolRounds, round, len(hits), len([]rune(contextText)), skillChars, mustJSON(tools))
			return Result{}, err
		}
		id, _ := saveCall(ctx, s.Platform.DB, app, userID, classID, conversationID,
			reqBody, respBody, latency, toolRounds, round, len(hits), len([]rune(contextText)), skillChars, tools)
		if id > 0 {
			callIDs = append(callIDs, id)
		}
		msg := resp.Choices[0].Message
		if len(msg.ToolCalls) == 0 {
			answer = strings.TrimSpace(msg.Content)
			break
		}
		// 记录 assistant 的工具请求，然后执行工具、回填结果。
		messages = append(messages, msg)
		for _, tc := range msg.ToolCalls {
			output, callErr := s.callTool(ctx, sessions, toolOwner, tc)
			if callErr != nil {
				output = "工具调用失败: " + callErr.Error()
			}
			messages = append(messages, llm.Message{
				Role: "tool", ToolCallID: tc.ID, Name: tc.Function.Name, Content: output,
			})
		}
		toolRounds++
	}
	if answer == "" {
		answer = "（模型未返回文本内容）"
	}

	return Result{
		Answer: answer, Citations: citations, ConversationID: conversationID,
		Meta: Meta{
			AppID: app.ID, AppName: app.Name, Model: app.Model,
			Temperature: app.Temperature, MaxTokens: app.MaxTokens,
			TemplateID: app.Template.ID, TemplateUsed: true,
			KBEnabled: app.KBEnabled, KBChunks: len(hits), KBChars: len([]rune(contextText)),
			KBTotalChunks: totalChunks,
			SkillIDs:      skillIDs, SkillChars: skillChars, ToolNames: toolNames,
			LLMCalls: callIDs, ToolRounds: toolRounds,
			LatencyMS: time.Since(started).Milliseconds(),
		},
	}, nil
}

func (s *Service) callTool(ctx context.Context, sessions []mcpSession, owner map[string]string, tc llm.ToolCall) (string, error) {
	var args map[string]any
	if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
		return "", fmt.Errorf("解析工具参数失败: %w", err)
	}
	serverID := owner[tc.Function.Name]
	for _, sess := range sessions {
		if sess.server == serverID {
			return sess.conn.CallTool(ctx, tc.Function.Name, args)
		}
	}
	return "", fmt.Errorf("未找到提供工具 %s 的 MCP 服务器", tc.Function.Name)
}

func buildContext(hits []search.Result) (string, []Citation) {
	var b strings.Builder
	citations := make([]Citation, 0, len(hits))
	for i, h := range hits {
		fmt.Fprintf(&b, "[%d] 《%s》（第 %d 块，字符 %d-%d）\n%s\n\n",
			i+1, h.MaterialTitle, h.ChunkIndex+1, h.StartOffset, h.EndOffset, strings.TrimSpace(h.Content))
		citations = append(citations, Citation{
			Index: i + 1, MaterialID: h.MaterialID, MaterialTitle: h.MaterialTitle,
			OriginalName: h.OriginalName, ChunkIndex: h.ChunkIndex,
			StartOffset: h.StartOffset, EndOffset: h.EndOffset, Snippet: h.Content, Score: h.Score,
		})
	}
	if b.Len() == 0 {
		return "（本班材料中没有检索到相关片段）", citations
	}
	return b.String(), citations
}

func render(tpl string, vars map[string]string) string {
	out := tpl
	for k, v := range vars {
		out = strings.ReplaceAll(out, "{{"+k+"}}", v)
	}
	return out
}

func splitArgs(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return strings.Fields(s)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func saveCall(ctx context.Context, db *sql.DB, app platform.App, userID, classID int64, conversationID string,
	reqBody, respBody []byte, latency int64, toolRounds, round, kbChunks, kbChars, skillChars int, tools []llm.Tool) (int64, error) {
	res, err := db.ExecContext(ctx, `
		INSERT INTO llm_calls (app_id, user_id, class_id, conversation_id, model, request_json, response_json,
			status, latency_ms, tool_rounds, round_index, kb_chunks, kb_chars, skill_chars, tools_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, 'ok', ?, ?, ?, ?, ?, ?, ?)`,
		app.ID, userID, classID, conversationID, app.Model, string(reqBody), string(respBody),
		latency, toolRounds, round, kbChunks, kbChars, skillChars, mustJSON(tools))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}
