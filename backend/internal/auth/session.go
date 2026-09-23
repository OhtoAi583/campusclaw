package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

// ErrNoSession 表示会话不存在或已过期。
var ErrNoSession = errors.New("session not found")

// Session 是鉴权中间件需要的全部身份信息。role 与 class_id 每次从 users 表读取，
// 不采信客户端回传的任何字段。
type Session struct {
	UserID    int64
	Username  string
	Role      string
	ClassID   int64
	ClassName string
	ExpiresAt time.Time
}

// SessionStore 负责签发、校验与删除服务端会话。
// 数据库里保存的是 token 的 HMAC，而不是 token 本身：即使 sessions 表被读走，也无法直接冒充用户。
type SessionStore struct {
	db     *sql.DB
	secret []byte
	ttl    time.Duration
}

// NewSessionStore 构造会话存储。secret 来自 SESSION_SECRET。
func NewSessionStore(db *sql.DB, secret string, ttl time.Duration) *SessionStore {
	return &SessionStore{db: db, secret: []byte(secret), ttl: ttl}
}

// TTL 返回会话有效期，供 Cookie Max-Age 使用。
func (s *SessionStore) TTL() time.Duration { return s.ttl }

// Create 签发一个新会话。无论是首次登录还是携带旧 Cookie 登录，都创建新行，
// 从而让登录前的会话标识失效（防会话固定）。
func (s *SessionStore) Create(ctx context.Context, userID int64) (string, time.Time, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", time.Time{}, fmt.Errorf("生成会话标识失败: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	expires := time.Now().Add(s.ttl)
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO sessions (id, user_id, created_at, expires_at) VALUES (?, ?, UTC_TIMESTAMP(), ?)`,
		s.digest(token), userID, expires.UTC())
	if err != nil {
		return "", time.Time{}, fmt.Errorf("写入会话失败: %w", err)
	}
	return token, expires, nil
}

// Invalidate 删除指定会话（登出，或登录成功时作废旧会话）。
func (s *SessionStore) Invalidate(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE id = ?`, s.digest(token)); err != nil {
		return fmt.Errorf("删除会话失败: %w", err)
	}
	return nil
}

// Lookup 校验 token 并返回会话身份。过期会话会被删除并视为不存在。
func (s *SessionStore) Lookup(ctx context.Context, token string) (Session, error) {
	if token == "" {
		return Session{}, ErrNoSession
	}
	var (
		sess      Session
		expiresAt time.Time
	)
	err := s.db.QueryRowContext(ctx, `
		SELECT s.user_id, u.username, u.role, u.class_id, c.name, s.expires_at
		FROM sessions s
		JOIN users u ON u.id = s.user_id
		JOIN classes c ON c.id = u.class_id
		WHERE s.id = ?`, s.digest(token)).
		Scan(&sess.UserID, &sess.Username, &sess.Role, &sess.ClassID, &sess.ClassName, &expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Session{}, ErrNoSession
	}
	if err != nil {
		return Session{}, fmt.Errorf("读取会话失败: %w", err)
	}
	if time.Now().After(expiresAt) {
		_ = s.Invalidate(ctx, token)
		return Session{}, ErrNoSession
	}
	sess.ExpiresAt = expiresAt
	return sess, nil
}

// DeleteExpired 清理过期会话行，由后台协程定期调用。
func (s *SessionStore) DeleteExpired(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at < UTC_TIMESTAMP()`)
	return err
}

// digest 用 SESSION_SECRET 作为密钥对 token 做 HMAC-SHA256。
func (s *SessionStore) digest(token string) string {
	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte(token))
	return hex.EncodeToString(mac.Sum(nil))
}
