package auth

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

// ErrInvalidCredentials 是认证失败的统一错误。调用方对外只能返回同一段文案，
// 不允许据此区分"用户不存在"与"口令错误"。
var ErrInvalidCredentials = errors.New("invalid credentials")

// Hasher 封装口令哈希与校验，便于在测试中替换成本更低的参数。
type Hasher struct {
	cost int
	// dummy 是一个固定的合法哈希，用于给不存在的用户做一次等价耗时的比较，
	// 避免通过响应时间区分账号是否存在。
	dummy []byte
}

// NewHasher 生成一个 bcrypt 哈希器，并预置一份用于恒定时比较的假哈希。
func NewHasher(cost int) (*Hasher, error) {
	if cost <= 0 {
		cost = bcrypt.DefaultCost
	}
	seed := make([]byte, 24)
	if _, err := rand.Read(seed); err != nil {
		return nil, fmt.Errorf("生成假哈希失败: %w", err)
	}
	dummy, err := bcrypt.GenerateFromPassword([]byte(base64.RawURLEncoding.EncodeToString(seed)), cost)
	if err != nil {
		return nil, fmt.Errorf("生成假哈希失败: %w", err)
	}
	return &Hasher{cost: cost, dummy: dummy}, nil
}

// Hash 返回口令的 bcrypt 哈希，用于写入 users.password_hash。
func (h *Hasher) Hash(password string) (string, error) {
	sum, err := bcrypt.GenerateFromPassword([]byte(password), h.cost)
	if err != nil {
		return "", fmt.Errorf("哈希口令失败: %w", err)
	}
	return string(sum), nil
}

// Verify 校验口令。任何失败都返回 ErrInvalidCredentials，不区分原因。
func (h *Hasher) Verify(hash, password string) error {
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		return ErrInvalidCredentials
	}
	return nil
}

// VerifyDummy 在用户不存在时执行一次同样成本的比较，让未知用户与口令错误耗时接近。
func (h *Hasher) VerifyDummy(password string) {
	_ = bcrypt.CompareHashAndPassword(h.dummy, []byte(password))
}
