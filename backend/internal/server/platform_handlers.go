package server

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"campusclaw/backend/internal/httpx"
	"campusclaw/backend/internal/mcp"
	"campusclaw/backend/internal/platform"
)

// listApps 返回内置 AI 应用及其绑定关系。
func (s *server) listApps(w http.ResponseWriter, r *http.Request) {
	apps, err := s.deps.Platform.ListApps(r.Context())
	if err != nil {
		slog.Error("读取应用失败", "error", err)
		httpx.PermanentFailure(w)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": apps})
}

// getApp 返回单个应用。
func (s *server) getApp(w http.ResponseWriter, r *http.Request) {
	app, err := s.deps.Platform.GetApp(r.Context(), r.PathValue("id"))
	if errors.Is(err, platform.ErrNotFound) {
		httpx.Fail(w, http.StatusNotFound, "app_not_found")
		return
	}
	if err != nil {
		httpx.PermanentFailure(w)
		return
	}
	httpx.JSON(w, http.StatusOK, app)
}

// listTemplates 返回提示词模板。
func (s *server) listTemplates(w http.ResponseWriter, r *http.Request) {
	items, err := s.deps.Platform.ListTemplates(r.Context())
	if err != nil {
		httpx.PermanentFailure(w)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": items})
}

// listSkills 返回内置与上传的技能。
func (s *server) listSkills(w http.ResponseWriter, r *http.Request) {
	items, err := s.deps.Platform.ListSkills(r.Context())
	if err != nil {
		httpx.PermanentFailure(w)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": items})
}

// uploadSkill 接收一个 SKILL.md 并保存（支持 .md 文件或 JSON 正文）。
func (s *server) uploadSkill(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 256<<10)
	var (
		filename string
		body     string
	)
	contentType := r.Header.Get("Content-Type")
	if strings.HasPrefix(contentType, "multipart/form-data") {
		if err := r.ParseMultipartForm(256 << 10); err != nil {
			httpx.Fail(w, http.StatusBadRequest, "invalid_multipart")
			return
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			httpx.Fail(w, http.StatusBadRequest, "missing_file")
			return
		}
		defer file.Close()
		if !strings.HasSuffix(strings.ToLower(header.Filename), ".md") {
			httpx.Fail(w, http.StatusBadRequest, "only_markdown_skill")
			return
		}
		raw, err := io.ReadAll(io.LimitReader(file, 256<<10))
		if err != nil || len(raw) == 0 {
			httpx.Fail(w, http.StatusBadRequest, "empty_skill")
			return
		}
		filename, body = filepath.Base(header.Filename), string(raw)
	} else {
		var req struct {
			Name string `json:"name"`
			Body string `json:"body"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Body) == "" {
			httpx.Fail(w, http.StatusBadRequest, "invalid_skill")
			return
		}
		filename, body = req.Name, req.Body
		if filename == "" {
			filename = "skill.md"
		}
	}

	name, description := parseSkillMeta(filename, body)
	id := slug(strings.TrimSuffix(filename, filepath.Ext(filename)))
	if id == "" {
		id = "skill"
	}
	skill := platform.Skill{
		ID: id, Name: name, Description: description, Body: body,
		SourceFilename: filename, SizeChars: len([]rune(body)), Enabled: true,
	}
	if err := s.deps.Platform.UpsertSkill(r.Context(), skill); err != nil {
		slog.Error("保存技能失败", "error", err)
		httpx.PermanentFailure(w)
		return
	}
	httpx.JSON(w, http.StatusCreated, skill)
}

// listMCPServers 返回 MCP 服务器配置。
func (s *server) listMCPServers(w http.ResponseWriter, r *http.Request) {
	items, err := s.deps.Platform.ListMCPServers(r.Context())
	if err != nil {
		httpx.PermanentFailure(w)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": items})
}

// mcpTools 现场连接 MCP 服务器，返回其暴露的工具 schema。
func (s *server) mcpTools(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	servers, err := s.deps.Platform.ListMCPServers(r.Context())
	if err != nil {
		httpx.PermanentFailure(w)
		return
	}
	for _, srv := range servers {
		if srv.ID != id {
			continue
		}
		conn, err := mcp.Start(r.Context(), srv.Command, strings.Fields(srv.Args)...)
		if err != nil {
			httpx.Fail(w, http.StatusBadGateway, "mcp_unreachable")
			return
		}
		defer conn.Close()
		tools, err := conn.ListTools(r.Context())
		if err != nil {
			httpx.Fail(w, http.StatusBadGateway, "mcp_list_failed")
			return
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"server": srv, "tools": tools})
		return
	}
	httpx.Fail(w, http.StatusNotFound, "mcp_server_not_found")
}

// chatHandler 执行一次应用对话。
func (s *server) chatHandler(w http.ResponseWriter, r *http.Request) {
	session, ok := httpx.SessionFrom(r.Context())
	if !ok {
		httpx.Fail(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var req struct {
		AppID          string `json:"app_id"`
		Question       string `json:"question"`
		ConversationID string `json:"conversation_id"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10)).Decode(&req); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid_request")
		return
	}
	if strings.TrimSpace(req.Question) == "" || req.AppID == "" {
		httpx.Fail(w, http.StatusBadRequest, "missing_fields")
		return
	}
	if req.ConversationID == "" {
		req.ConversationID = fmt.Sprintf("c%d-%d", session.UserID, timeNowUnixNano())
	}
	result, err := s.deps.Chat.Ask(r.Context(), req.AppID, session.ClassID, session.UserID,
		session.ClassName, strings.TrimSpace(req.Question), req.ConversationID)
	if errors.Is(err, platform.ErrNotFound) {
		httpx.Fail(w, http.StatusNotFound, "app_not_found")
		return
	}
	if err != nil {
		slog.Error("对话失败", "error", err)
		httpx.PermanentFailure(w)
		return
	}
	slog.Info("应用对话完成", "app", req.AppID, "user_id", session.UserID, "class_id", session.ClassID,
		"kb_chunks", result.Meta.KBChunks, "skills", result.Meta.SkillIDs,
		"tools", result.Meta.ToolNames, "tool_rounds", result.Meta.ToolRounds,
		"llm_calls", result.Meta.LLMCalls, "latency_ms", result.Meta.LatencyMS)
	httpx.JSON(w, http.StatusOK, result)
}

// listLLMCalls 按会话或应用返回调用记录（不含完整报文）。
func (s *server) listLLMCalls(w http.ResponseWriter, r *http.Request) {
	conversation := r.URL.Query().Get("conversation_id")
	appID := r.URL.Query().Get("app_id")
	rows, err := s.deps.DB.QueryContext(r.Context(), `
		SELECT id, app_id, conversation_id, model, status, latency_ms, round_index,
		       kb_chunks, kb_chars, skill_chars, tool_rounds, created_at
		FROM llm_calls
		WHERE (? = '' OR conversation_id = ?) AND (? = '' OR app_id = ?)
		ORDER BY id DESC LIMIT 50`, conversation, conversation, appID, appID)
	if err != nil {
		httpx.PermanentFailure(w)
		return
	}
	defer rows.Close()
	type row struct {
		ID             int64  `json:"id"`
		AppID          string `json:"app_id"`
		ConversationID string `json:"conversation_id"`
		Model          string `json:"model"`
		Status         string `json:"status"`
		LatencyMS      int    `json:"latency_ms"`
		RoundIndex     int    `json:"round_index"`
		KBChunks       int    `json:"kb_chunks"`
		KBChars        int    `json:"kb_chars"`
		SkillChars     int    `json:"skill_chars"`
		ToolRounds     int    `json:"tool_rounds"`
		CreatedAt      string `json:"created_at"`
	}
	var out []row
	for rows.Next() {
		var x row
		if err := rows.Scan(&x.ID, &x.AppID, &x.ConversationID, &x.Model, &x.Status, &x.LatencyMS,
			&x.RoundIndex, &x.KBChunks, &x.KBChars, &x.SkillChars, &x.ToolRounds, &x.CreatedAt); err != nil {
			httpx.PermanentFailure(w)
			return
		}
		out = append(out, x)
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": out})
}

// getLLMCall 返回一次调用的完整请求体与响应体（验收证据）。
func (s *server) getLLMCall(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid_id")
		return
	}
	var (
		appID, conversation, model, status                        string
		requestJSON, responseJSON, toolsJSON                      string
		latency, round, kbChunks, kbChars, skillChars, toolRounds int
		createdAt                                                 string
	)
	err = s.deps.DB.QueryRowContext(r.Context(), `
		SELECT app_id, conversation_id, model, status, request_json, response_json, COALESCE(tools_json,''),
		       latency_ms, round_index, kb_chunks, kb_chars, skill_chars, tool_rounds, created_at
		FROM llm_calls WHERE id = ?`, id).
		Scan(&appID, &conversation, &model, &status, &requestJSON, &responseJSON, &toolsJSON,
			&latency, &round, &kbChunks, &kbChars, &skillChars, &toolRounds, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.Fail(w, http.StatusNotFound, "call_not_found")
		return
	}
	if err != nil {
		httpx.PermanentFailure(w)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"id": id, "app_id": appID, "conversation_id": conversation, "model": model, "status": status,
		"latency_ms": latency, "round_index": round, "kb_chunks": kbChunks, "kb_chars": kbChars,
		"skill_chars": skillChars, "tool_rounds": toolRounds, "created_at": createdAt,
		"tools":    json.RawMessage(toolsJSON),
		"request":  json.RawMessage(requestJSON),
		"response": json.RawMessage(responseJSON),
	})
}

var (
	nonSlug   = regexp.MustCompile(`[^a-zA-Z0-9_-]+`)
	multiDash = regexp.MustCompile(`-{2,}`)
)

func slug(s string) string {
	out := nonSlug.ReplaceAllString(strings.ToLower(s), "-")
	out = multiDash.ReplaceAllString(out, "-")
	return strings.Trim(out, "-")
}

// parseSkillMeta 从 SKILL.md 里取名称与描述：
// 优先 frontmatter 的 name/description，其次一级标题与首段。
func parseSkillMeta(filename, body string) (string, string) {
	fallbackName := strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))
	name := fallbackName
	named := false
	description := ""
	lines := strings.Split(body, "\n")
	inFront := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "---" && i == 0 {
			inFront = true
			continue
		}
		if inFront && trimmed == "---" {
			inFront = false
			continue
		}
		if inFront {
			if v, ok := strings.CutPrefix(trimmed, "name:"); ok {
				if candidate := strings.Trim(strings.TrimSpace(v), `"'`); candidate != "" {
					name = candidate
					named = true
				}
			}
			if v, ok := strings.CutPrefix(trimmed, "description:"); ok {
				description = strings.Trim(strings.TrimSpace(v), `"'`)
			}
			continue
		}
		if !named && strings.HasPrefix(trimmed, "# ") {
			if candidate := strings.TrimSpace(strings.TrimPrefix(trimmed, "# ")); candidate != "" {
				name = candidate
				named = true
			}
		}
		if description == "" && trimmed != "" && !strings.HasPrefix(trimmed, "#") && !strings.HasPrefix(trimmed, "---") {
			description = trimmed
		}
	}
	if !named {
		name = fallbackName
	}
	if description == "" {
		description = "用户上传的技能"
	}
	if len([]rune(description)) > 120 {
		description = string([]rune(description)[:120])
	}
	return name, description
}

func timeNowUnixNano() int64 { return time.Now().UnixNano() }
