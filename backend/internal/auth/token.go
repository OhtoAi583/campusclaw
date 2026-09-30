package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrInvalidToken 表示访问令牌无效（签名、格式或过期）。
var ErrInvalidToken = errors.New("invalid token")

// TokenClaims 是访问令牌的载荷。
//
// 刻意只放 sub / jti / 时间戳：**不放 role 与 class_id**。
// 载荷是客户端可读的，把权限声明放进去，容易被误当成授权依据；
// 本实现每次请求都按 sub 回 users 表取 role 与 class_id，与 Cookie 方案完全一致。
type TokenClaims struct {
	Subject   int64  `json:"sub"`
	TokenID   string `json:"jti"`
	IssuedAt  int64  `json:"iat"`
	ExpiresAt int64  `json:"exp"`
}

// TokenPair 是一次签发的令牌对。
type TokenPair struct {
	AccessToken  string    `json:"access_token"`
	TokenType    string    `json:"token_type"`
	ExpiresIn    int       `json:"expires_in"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// TokenService 负责签发与校验访问令牌，并管理可撤销的刷新令牌。
// 访问令牌是无状态 JWT（HS256）；刷新令牌是随机串，只有哈希入库，可随时撤销。
type TokenService struct {
	DB         *sql.DB
	secret     []byte
	accessTTL  time.Duration
	refreshTTL time.Duration
	now        func() time.Time
}

// NewTokenService 构造令牌服务。secret 来自 JWT_SECRET。
func NewTokenService(db *sql.DB, secret string, accessTTL, refreshTTL time.Duration) *TokenService {
	return &TokenService{DB: db, secret: []byte(secret), accessTTL: accessTTL, refreshTTL: refreshTTL, now: time.Now}
}

// Issue 签发一对新令牌：JWT 访问令牌 + 随机刷新令牌（刷新令牌哈希入库，可撤销）。
func (t *TokenService) Issue(ctx context.Context, userID int64) (TokenPair, error) {
	now := t.now()
	claims := TokenClaims{
		Subject:   userID,
		TokenID:   randomHex(16),
		IssuedAt:  now.Unix(),
		ExpiresAt: now.Add(t.accessTTL).Unix(),
	}
	access, err := t.sign(claims)
	if err != nil {
		return TokenPair{}, err
	}
	refresh := randomHex(32)
	if _, err := t.DB.ExecContext(ctx, `
		INSERT INTO refresh_tokens (id, user_id, created_at, expires_at) VALUES (?, ?, ?, ?)`,
		digestSecret(t.secret, refresh), userID, now.UTC(), now.Add(t.refreshTTL).UTC()); err != nil {
		return TokenPair{}, fmt.Errorf("写入刷新令牌失败: %w", err)
	}
	return TokenPair{
		AccessToken:  access,
		TokenType:    "Bearer",
		ExpiresIn:    int(t.accessTTL.Seconds()),
		RefreshToken: refresh,
		ExpiresAt:    time.Unix(claims.ExpiresAt, 0),
	}, nil
}

// Refresh 用刷新令牌换一对新令牌，并**轮换**旧刷新令牌（一次性使用）。
func (t *TokenService) Refresh(ctx context.Context, refreshToken string) (TokenPair, error) {
	id := digestSecret(t.secret, refreshToken)
	var userID int64
	var expiresAt time.Time
	err := t.DB.QueryRowContext(ctx,
		`SELECT user_id, expires_at FROM refresh_tokens WHERE id = ?`, id).Scan(&userID, &expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return TokenPair{}, ErrInvalidToken
	}
	if err != nil {
		return TokenPair{}, fmt.Errorf("读取刷新令牌失败: %w", err)
	}
	if t.now().After(expiresAt) {
		_, _ = t.DB.ExecContext(ctx, `DELETE FROM refresh_tokens WHERE id = ?`, id)
		return TokenPair{}, ErrInvalidToken
	}
	if _, err := t.DB.ExecContext(ctx, `DELETE FROM refresh_tokens WHERE id = ?`, id); err != nil {
		return TokenPair{}, fmt.Errorf("轮换刷新令牌失败: %w", err)
	}
	return t.Issue(ctx, userID)
}

// Revoke 撤销一个刷新令牌（登出）。访问令牌是无状态的，只能等它自然过期。
func (t *TokenService) Revoke(ctx context.Context, refreshToken string) error {
	_, err := t.DB.ExecContext(ctx, `DELETE FROM refresh_tokens WHERE id = ?`, digestSecret(t.secret, refreshToken))
	return err
}

// Verify 校验访问令牌（签名 + 有效期），返回载荷。
func (t *TokenService) Verify(token string) (TokenClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return TokenClaims{}, ErrInvalidToken
	}
	signing := parts[0] + "." + parts[1]
	mac := hmac.New(sha256.New, t.secret)
	mac.Write([]byte(signing))
	expected := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expected), []byte(parts[2])) {
		return TokenClaims{}, ErrInvalidToken
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return TokenClaims{}, ErrInvalidToken
	}
	var claims TokenClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return TokenClaims{}, ErrInvalidToken
	}
	if claims.Subject <= 0 || t.now().Unix() >= claims.ExpiresAt {
		return TokenClaims{}, ErrInvalidToken
	}
	return claims, nil
}

// sign 生成 HS256 的紧凑 JWT。
func (t *TokenService) sign(claims TokenClaims) (string, error) {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	signing := header + "." + base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, t.secret)
	mac.Write([]byte(signing))
	return signing + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

// DecodeClaims 只解码载荷，不校验签名——仅用于演示页面展示令牌结构。
func DecodeClaims(token string) (map[string]any, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, ErrInvalidToken
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, ErrInvalidToken
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, ErrInvalidToken
	}
	return out, nil
}

// dig 是与 Cookie 会话方案一致的哈希方式：HMAC(secret, value)，库里不存明文。
func digestSecret(secret []byte, value string) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(value))
	return hex.EncodeToString(mac.Sum(nil))
}

func randomHex(n int) string {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		// crypto/rand 失败属于不可恢复的环境问题
		panic(fmt.Sprintf("生成随机数失败: %v", err))
	}
	return hex.EncodeToString(buf)
}
