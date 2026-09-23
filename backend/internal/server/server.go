// Package server 组装路由与中间件。
package server

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strings"

	"campusclaw/backend/internal/auth"
	"campusclaw/backend/internal/config"
	"campusclaw/backend/internal/httpx"
	"campusclaw/backend/internal/materials"
)

// Deps 是服务器需要的外部依赖。
type Deps struct {
	Config    config.Config
	DB        *sql.DB
	Sessions  *auth.SessionStore
	Hasher    *auth.Hasher
	Limiter   *auth.Limiter
	Materials *materials.Handler
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

	// 受保护接口：默认拒绝，未登录一律 401。
	mux.Handle("GET /api/me", httpx.RequireAuth(http.HandlerFunc(s.me), deps.Sessions))
	mux.Handle("GET /api/materials", httpx.RequireAuth(http.HandlerFunc(deps.Materials.List), deps.Sessions))
	mux.Handle("GET /api/materials/{id}", httpx.RequireAuth(http.HandlerFunc(deps.Materials.Detail), deps.Sessions))
	mux.Handle("GET /api/materials/{id}/file", httpx.RequireAuth(http.HandlerFunc(deps.Materials.File), deps.Sessions))
	// 上传是教师专属：先认证，再按会话角色授权（垂直权限）。
	mux.Handle("POST /api/materials", httpx.RequireAuth(
		httpx.RequireRole("teacher", http.HandlerFunc(deps.Materials.Upload)), deps.Sessions))

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
