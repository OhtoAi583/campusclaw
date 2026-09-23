// Package httpx 放与框架无关的 HTTP 辅助件：身份注入、JSON 响应、日志与安全响应头。
package httpx

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"campusclaw/backend/internal/auth"
)

// CookieName 是承载服务端会话标识的 Cookie 名。
const CookieName = "campusclaw_session"

type ctxKey int

const sessionKey ctxKey = iota

// WithSession 把会话身份放进请求上下文。
func WithSession(ctx context.Context, s auth.Session) context.Context {
	return context.WithValue(ctx, sessionKey, s)
}

// SessionFrom 取出会话身份；只有经过 RequireAuth 的请求才有。
func SessionFrom(ctx context.Context) (auth.Session, bool) {
	s, ok := ctx.Value(sessionKey).(auth.Session)
	return s, ok
}

// ErrorBody 是所有失败响应的统一结构。
type ErrorBody struct {
	Error string `json:"error"`
}

// JSON 写出一个 JSON 响应。
func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if v == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("写出响应失败", "error", err)
	}
}

// Fail 写出统一结构的失败响应。
func Fail(w http.ResponseWriter, status int, code string) {
	JSON(w, status, ErrorBody{Error: code})
}

// PermanentFailure 表示服务端自身的错误，对外不透露细节。
func PermanentFailure(w http.ResponseWriter) {
	Fail(w, http.StatusServiceUnavailable, "service_unavailable")
}

// RequireAuth 默认拒绝：没有有效会话时直接 401，且响应体不含任何业务数据。
func RequireAuth(next http.Handler, store *auth.SessionStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(CookieName)
		if err != nil {
			Fail(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		session, err := store.Lookup(r.Context(), cookie.Value)
		switch {
		case errors.Is(err, auth.ErrNoSession):
			// 会话不存在或已过期：这是"未认证"，返回 401。
			Fail(w, http.StatusUnauthorized, "unauthorized")
			return
		case err != nil:
			// 会话存储不可用（例如数据库故障）：这是"服务不可用"，
			// 不能把已登录用户误判成会话失效，否则客户端会去重新登录而问题依旧。
			slog.Error("读取会话失败", "error", err)
			PermanentFailure(w)
			return
		}
		next.ServeHTTP(w, r.WithContext(WithSession(r.Context(), session)))
	})
}

// RequireRole 在已认证的基础上再做角色授权（垂直权限）。
func RequireRole(role string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session, ok := SessionFrom(r.Context())
		if !ok {
			Fail(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		if session.Role != role {
			Fail(w, http.StatusForbidden, "forbidden")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// SetSessionCookie 写出会话 Cookie：HttpOnly + SameSite=Lax。
// 本机 HTTP 环境不加 Secure，否则浏览器会拒绝写入。
func SetSessionCookie(w http.ResponseWriter, token string, ttl time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(ttl.Seconds()),
	})
}

// ClearSessionCookie 清掉浏览器上的会话 Cookie。
func ClearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

// Log 记录每个请求的方法、路径、状态码与耗时；跨班访问的真实原因也在这里留痕。
func Log(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		slog.Info("http",
			"method", r.Method,
			"path", r.URL.Path,
			"status", sw.status,
			"duration_ms", time.Since(start).Milliseconds(),
		)
	})
}

// SecurityHeaders 设置最小的一组安全响应头。
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "same-origin")
		next.ServeHTTP(w, r)
	})
}

// Recover 兜底 panic，避免单个请求把整个进程带下去。
func Recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				slog.Error("请求处理 panic", "panic", rec, "path", r.URL.Path)
				PermanentFailure(w)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
