// Package server 组装路由与中间件。
package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"campusclaw/backend/internal/answer"
	"campusclaw/backend/internal/auth"
	"campusclaw/backend/internal/config"
	"campusclaw/backend/internal/httpx"
	"campusclaw/backend/internal/materials"
	"campusclaw/backend/internal/search"
)

// Deps 是服务器需要的外部依赖。
type Deps struct {
	Config    config.Config
	DB        *sql.DB
	Sessions  *auth.SessionStore
	Tokens    *auth.TokenService
	Hasher    *auth.Hasher
	Limiter   *auth.Limiter
	Materials *materials.Handler
	Search    *search.Service
	Answer    *answer.Service
}

type server struct {
	deps Deps
}

// New 返回装配好中间件的 http.Handler。
func New(deps Deps) http.Handler {
	s := &server{deps: deps}
	mux := http.NewServeMux()

	// 存活探针：公开、不查库，只回答"进程还在不在"。
	mux.HandleFunc("GET /health", s.health)
	// 认证入口。
	mux.HandleFunc("POST /api/login", s.login)
	mux.HandleFunc("POST /api/logout", s.logout)

	// token 方案：换取访问令牌、刷新、撤销（与 Cookie 方案并存，互不影响）。
	mux.HandleFunc("POST /api/token", s.tokenLogin)
	mux.HandleFunc("POST /api/token/refresh", s.tokenRefresh)
	mux.HandleFunc("POST /api/token/logout", s.tokenLogout)

	// 受保护接口：默认拒绝，未登录一律 401。
	mux.Handle("GET /api/me", httpx.RequireAuth(http.HandlerFunc(s.me), deps.Sessions, deps.Tokens))
	mux.Handle("GET /api/materials", httpx.RequireAuth(http.HandlerFunc(deps.Materials.List), deps.Sessions, deps.Tokens))
	mux.Handle("GET /api/materials/{id}", httpx.RequireAuth(http.HandlerFunc(deps.Materials.Detail), deps.Sessions, deps.Tokens))
	mux.Handle("GET /api/materials/{id}/file", httpx.RequireAuth(http.HandlerFunc(deps.Materials.File), deps.Sessions, deps.Tokens))
	// 上传是教师专属：先认证，再按会话角色授权（垂直权限）。
	mux.Handle("POST /api/materials", httpx.RequireAuth(
		httpx.RequireRole("teacher", http.HandlerFunc(deps.Materials.Upload)), deps.Sessions, deps.Tokens))
	// 本班范围内的知识库检索：与列表、详情同一条班级边界，班级只取自会话。
	mux.Handle("POST /api/search", httpx.RequireAuth(http.HandlerFunc(s.search), deps.Sessions, deps.Tokens))
	// 基于知识库的问答：先在本班检索，取得切片后才生成回答并标注出处。
	mux.Handle("POST /api/ask", httpx.RequireAuth(http.HandlerFunc(s.ask), deps.Sessions, deps.Tokens))

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		httpx.Fail(w, http.StatusNotFound, "not_found")
	})

	var handler http.Handler = mux
	handler = httpx.Recover(handler)
	handler = httpx.Log(handler)
	handler = httpx.SecurityHeaders(handler)
	return handler
}

func (s *server) health(w http.ResponseWriter, r *http.Request) {
	// 故意不查数据库：数据库故障应表现为受保护接口不可用，
	// 而不是把容器判死重启，也不是把已登录用户判为会话失效。
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type identityResponse struct {
	UserID    int64  `json:"user_id"`
	Username  string `json:"username"`
	Role      string `json:"role"`
	ClassID   int64  `json:"class_id"`
	ClassName string `json:"class_name"`
}

func (s *server) login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10))
	if err := decoder.Decode(&req); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid_request")
		return
	}
	username := strings.TrimSpace(req.Username)
	key := username + "|" + clientIP(r)
	allowed := s.deps.Limiter.Allowed(key)

	var (
		userID int64
		hash   string
		found  bool
	)
	err := s.deps.DB.QueryRowContext(r.Context(),
		`SELECT id, password_hash FROM users WHERE username = ?`, username).Scan(&userID, &hash)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		found = false
	case err != nil:
		slog.Error("查询用户失败", "error", err)
		httpx.PermanentFailure(w)
		return
	default:
		found = true
	}

	// 未知用户、已锁定、口令错误三种情况对外必须完全一致：
	// 同样的状态码、同样的响应体，并且都执行一次同成本的哈希比较。
	switch {
	case !found || !allowed:
		s.deps.Hasher.VerifyDummy(req.Password)
		s.failLogin(w, key)
		return
	case s.deps.Hasher.Verify(hash, req.Password) != nil:
		s.failLogin(w, key)
		return
	}

	s.deps.Limiter.Reset(key)

	// 登录成功换发新的会话标识：先作废客户端带来的旧会话，再签发新会话（防会话固定）。
	if cookie, err := r.Cookie(httpx.CookieName); err == nil {
		if err := s.deps.Sessions.Invalidate(r.Context(), cookie.Value); err != nil {
			slog.Error("作废旧会话失败", "error", err)
		}
	}
	token, expires, err := s.deps.Sessions.Create(r.Context(), userID)
	if err != nil {
		slog.Error("签发会话失败", "error", err)
		httpx.PermanentFailure(w)
		return
	}
	_ = expires
	httpx.SetSessionCookie(w, token, s.deps.Sessions.TTL())

	session, err := s.deps.Sessions.Lookup(r.Context(), token)
	if err != nil {
		slog.Error("读取新会话失败", "error", err)
		httpx.PermanentFailure(w)
		return
	}
	slog.Info("登录成功", "username", session.Username, "role", session.Role, "class_id", session.ClassID)
	httpx.JSON(w, http.StatusOK, toIdentity(session))
}

// failLogin 记录一次失败并返回统一文案；锁定期也走同一条路径。
func (s *server) failLogin(w http.ResponseWriter, key string) {
	s.deps.Limiter.Fail(key)
	httpx.Fail(w, http.StatusUnauthorized, "invalid_credentials")
}

func (s *server) logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(httpx.CookieName); err == nil {
		if err := s.deps.Sessions.Invalidate(r.Context(), cookie.Value); err != nil {
			slog.Error("删除会话失败", "error", err)
			httpx.PermanentFailure(w)
			return
		}
	}
	httpx.ClearSessionCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) me(w http.ResponseWriter, r *http.Request) {
	session, ok := httpx.SessionFrom(r.Context())
	if !ok {
		httpx.Fail(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	httpx.JSON(w, http.StatusOK, toIdentity(session))
}

type tokenRequest struct {
	Username     string `json:"username"`
	Password     string `json:"password"`
	RefreshToken string `json:"refresh_token"`
}

// tokenLogin 校验账号口令并签发 JWT 访问令牌 + 刷新令牌。
// 与 Cookie 方案共用同一套口令校验、失败文案与限流逻辑。
func (s *server) tokenLogin(w http.ResponseWriter, r *http.Request) {
	var req tokenRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&req); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid_request")
		return
	}
	username := strings.TrimSpace(req.Username)
	key := username + "|" + clientIP(r)

	var (
		userID int64
		hash   string
	)
	err := s.deps.DB.QueryRowContext(r.Context(),
		`SELECT id, password_hash FROM users WHERE username = ?`, username).Scan(&userID, &hash)
	allowed := s.deps.Limiter.Allowed(key)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		s.deps.Hasher.VerifyDummy(req.Password)
		s.failLogin(w, key)
		return
	case err != nil:
		slog.Error("查询用户失败", "error", err)
		httpx.PermanentFailure(w)
		return
	case !allowed:
		s.deps.Hasher.VerifyDummy(req.Password)
		s.failLogin(w, key)
		return
	case s.deps.Hasher.Verify(hash, req.Password) != nil:
		s.failLogin(w, key)
		return
	}
	s.deps.Limiter.Reset(key)

	pair, err := s.deps.Tokens.Issue(r.Context(), userID)
	if err != nil {
		slog.Error("签发令牌失败", "error", err)
		httpx.PermanentFailure(w)
		return
	}
	slog.Info("签发访问令牌", "username", username, "user_id", userID, "expires_in", pair.ExpiresIn)
	httpx.JSON(w, http.StatusOK, pair)
}

// tokenRefresh 用刷新令牌换新令牌对（旧刷新令牌一次性作废，即轮换）。
func (s *server) tokenRefresh(w http.ResponseWriter, r *http.Request) {
	var req tokenRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&req); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid_request")
		return
	}
	pair, err := s.deps.Tokens.Refresh(r.Context(), strings.TrimSpace(req.RefreshToken))
	if err != nil {
		httpx.Fail(w, http.StatusUnauthorized, "invalid_token")
		return
	}
	httpx.JSON(w, http.StatusOK, pair)
}

// tokenLogout 撤销刷新令牌。访问令牌是无状态的，只能等它自然过期（写在 README 的已知限制里）。
func (s *server) tokenLogout(w http.ResponseWriter, r *http.Request) {
	var req tokenRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&req); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid_request")
		return
	}
	if err := s.deps.Tokens.Revoke(r.Context(), strings.TrimSpace(req.RefreshToken)); err != nil {
		slog.Error("撤销刷新令牌失败", "error", err)
		httpx.PermanentFailure(w)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type askRequest struct {
	Question string `json:"question"`
	TopK     int    `json:"top_k"`
}

// ask 执行"先检索、再生成"。无命中时不调用生成模型，直接返回固定文案与空 citations。
func (s *server) ask(w http.ResponseWriter, r *http.Request) {
	session, ok := httpx.SessionFrom(r.Context())
	if !ok {
		httpx.Fail(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var req askRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&req); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid_request")
		return
	}
	question := strings.TrimSpace(req.Question)
	if question == "" {
		httpx.Fail(w, http.StatusBadRequest, "empty_question")
		return
	}
	if len([]rune(question)) > s.deps.Config.Search.QueryMaxChars {
		httpx.Fail(w, http.StatusBadRequest, "question_too_long")
		return
	}

	started := time.Now()
	result, err := s.deps.Answer.Ask(r.Context(), session.ClassID, question, req.TopK)
	switch {
	case errors.Is(err, search.ErrInvalidQuery):
		httpx.Fail(w, http.StatusBadRequest, "invalid_question")
		return
	case err != nil:
		slog.Error("问答失败", "error", err)
		httpx.PermanentFailure(w)
		return
	}

	// 日志只记必要信息：不记问题原文与切片正文。
	slog.Info("问答",
		"user_id", session.UserID,
		"class_id", session.ClassID,
		"question_chars", len([]rune(question)),
		"citations", len(result.Citations),
		"model_called", result.ModelCalled,
		"engine", result.Engine,
		"duration_ms", time.Since(started).Milliseconds(),
	)
	httpx.JSON(w, http.StatusOK, result)
}

type searchRequest struct {
	Query string `json:"query"`
	TopK  int    `json:"top_k"`
}

type searchResponse struct {
	Query   string          `json:"query"`
	ClassID int64           `json:"class_id"`
	Count   int             `json:"count"`
	Items   []search.Result `json:"items"`
}

// search 执行本班检索。检索范围只来自会话：请求体里的 class_id 既不解析也不采信。
func (s *server) search(w http.ResponseWriter, r *http.Request) {
	session, ok := httpx.SessionFrom(r.Context())
	if !ok {
		httpx.Fail(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var req searchRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10))
	if err := decoder.Decode(&req); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid_request")
		return
	}

	timeout := s.deps.Config.Search.Timeout
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()

	started := time.Now()
	items, candidates, err := s.deps.Search.Search(ctx, session.ClassID, req.Query, req.TopK)
	switch {
	case errors.Is(err, search.ErrInvalidQuery):
		httpx.Fail(w, http.StatusBadRequest, "invalid_query")
		return
	case errors.Is(err, context.DeadlineExceeded):
		// 超时返回 503，不返回部分结果（spec R10.2）。
		slog.Warn("检索超时", "user_id", session.UserID, "class_id", session.ClassID)
		httpx.PermanentFailure(w)
		return
	case err != nil:
		slog.Error("检索失败", "error", err)
		httpx.PermanentFailure(w)
		return
	}

	// 日志只记录必要信息：不写查询原文，也不写片段正文（spec R9）。
	slog.Info("检索",
		"user_id", session.UserID,
		"class_id", session.ClassID,
		"query_chars", len([]rune(strings.TrimSpace(req.Query))),
		"candidates", candidates,
		"results", len(items),
		"duration_ms", time.Since(started).Milliseconds(),
	)

	if items == nil {
		items = []search.Result{}
	}
	httpx.JSON(w, http.StatusOK, searchResponse{
		Query:   strings.TrimSpace(req.Query),
		ClassID: session.ClassID,
		Count:   len(items),
		Items:   items,
	})
}

func toIdentity(session auth.Session) identityResponse {
	return identityResponse{
		UserID:    session.UserID,
		Username:  session.Username,
		Role:      session.Role,
		ClassID:   session.ClassID,
		ClassName: session.ClassName,
	}
}

// clientIP 取真实客户端 IP 用于限流键。生产链路里 Nginx 会带上 X-Real-IP。
func clientIP(r *http.Request) string {
	if v := strings.TrimSpace(r.Header.Get("X-Real-IP")); v != "" {
		return v
	}
	if v := r.Header.Get("X-Forwarded-For"); v != "" {
		if idx := strings.Index(v, ","); idx > 0 {
			return strings.TrimSpace(v[:idx])
		}
		return strings.TrimSpace(v)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
