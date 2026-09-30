// Package config 负责把运行所需配置从环境变量读入，并在缺失必填项时直接失败。
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config 是进程启动所需的全部配置。所有字段都来自环境变量，没有内置密钥默认值。
type Config struct {
	Addr             string
	SessionSecret    string
	SessionTTL       time.Duration
	JWTSecret        string
	AccessTokenTTL   time.Duration
	RefreshTokenTTL  time.Duration
	UploadDir        string
	MaxUploadBytes   int64
	LoginMaxAttempts int
	LoginWindow      time.Duration
	LoginLock        time.Duration
	Seed             Seed
	DB               DB
	Search           Search
}

// Search 是知识库检索相关配置。
type Search struct {
	EmbeddingDim  int
	ChunkSize     int
	ChunkOverlap  int
	TopKMax       int
	QueryMaxChars int
	Timeout       time.Duration
	// MinScore 是最低相似度阈值：低于它的候选被视为"没有匹配"，
	// 避免哈希嵌入的碰撞噪声让无关查询也返回结果（spec R2.4）。
	MinScore float64
}

// DB 是数据库连接配置。
type DB struct {
	Host     string
	Port     string
	Name     string
	User     string
	Password string
}

// Seed 是预置账号口令；它们同样只来自环境变量。
type Seed struct {
	TeacherAPassword  string
	StudentA1Password string
	StudentB1Password string
}

// DSN 返回 go-sql-driver/mysql 使用的连接串。parseTime 让 DATETIME 直接映射到 time.Time。
func (d DB) DSN() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true&charset=utf8mb4&collation=utf8mb4_unicode_ci&loc=UTC",
		d.User, d.Password, d.Host, d.Port, d.Name)
}

// Load 读取环境变量。缺任何一个必填项都会返回错误，调用方应据此终止启动，
// 而不是退回某个内置默认值（内置默认密钥等于把会话签发权交给任何拿到仓库的人）。
func Load() (Config, error) {
	var missing []string

	required := func(key string) string {
		v := strings.TrimSpace(os.Getenv(key))
		if v == "" {
			missing = append(missing, key)
		}
		return v
	}

	cfg := Config{
		Addr:             envDefault("APP_ADDR", ":"+envDefault("APP_PORT", "8080")),
		SessionSecret:    required("SESSION_SECRET"),
		JWTSecret:        required("JWT_SECRET"),
		UploadDir:        envDefault("UPLOAD_DIR", "/data/uploads"),
		LoginMaxAttempts: envInt("LOGIN_MAX_ATTEMPTS", 5),
	}
	cfg.DB = DB{
		Host:     envDefault("DB_HOST", "db"),
		Port:     envDefault("DB_PORT", "3306"),
		Name:     required("DB_NAME"),
		User:     required("DB_USER"),
		Password: required("DB_PASSWORD"),
	}
	cfg.Seed = Seed{
		TeacherAPassword:  required("SEED_TEACHER_A_PASSWORD"),
		StudentA1Password: required("SEED_STUDENT_A1_PASSWORD"),
		StudentB1Password: required("SEED_STUDENT_B1_PASSWORD"),
	}

	cfg.Search = Search{
		EmbeddingDim:  envInt("EMBEDDING_DIM", 512),
		ChunkSize:     envInt("CHUNK_SIZE", 600),
		ChunkOverlap:  envInt("CHUNK_OVERLAP", 80),
		TopKMax:       envInt("SEARCH_TOP_K_MAX", 20),
		QueryMaxChars: envInt("SEARCH_QUERY_MAX_CHARS", 200),
		MinScore:      envFloat("SEARCH_MIN_SCORE", 0.15),
	}
	cfg.Search.Timeout = envDurationMs("SEARCH_TIMEOUT_MS", 1500*time.Millisecond)

	cfg.SessionTTL = envDuration("SESSION_TTL_HOURS", 8*time.Hour)
	cfg.AccessTokenTTL = time.Duration(envPositive("ACCESS_TOKEN_TTL_MINUTES", 15)) * time.Minute
	cfg.RefreshTokenTTL = time.Duration(envPositive("REFRESH_TOKEN_TTL_DAYS", 7)) * 24 * time.Hour
	cfg.LoginWindow = envDuration("LOGIN_WINDOW_MINUTES", 15*time.Minute)
	cfg.LoginLock = envDuration("LOGIN_LOCK_MINUTES", 15*time.Minute)
	cfg.MaxUploadBytes = envInt64("MAX_UPLOAD_BYTES", 2<<20)

	if err := validateSeedPasswords(cfg.Seed); err != nil {
		return Config{}, err
	}
	if len(missing) > 0 {
		return Config{}, fmt.Errorf("缺少必填环境变量：%s", strings.Join(unique(missing), ", "))
	}
	if cfg.MaxUploadBytes <= 0 {
		return Config{}, errors.New("MAX_UPLOAD_BYTES 必须为正整数")
	}
	if err := validateSearch(cfg.Search); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// validateSearch 拒绝会让检索不可解释的配置组合（对应 spec R10.3）。
func validateSearch(s Search) error {
	if s.EmbeddingDim <= 0 {
		return errors.New("EMBEDDING_DIM 必须为正整数")
	}
	if s.ChunkSize <= 0 {
		return errors.New("CHUNK_SIZE 必须为正整数")
	}
	if s.ChunkOverlap < 0 || s.ChunkOverlap >= s.ChunkSize {
		return fmt.Errorf("CHUNK_OVERLAP（%d）必须大于等于 0 且小于 CHUNK_SIZE（%d）", s.ChunkOverlap, s.ChunkSize)
	}
	if s.TopKMax <= 0 {
		return errors.New("SEARCH_TOP_K_MAX 必须为正整数")
	}
	if s.QueryMaxChars <= 0 {
		return errors.New("SEARCH_QUERY_MAX_CHARS 必须为正整数")
	}
	if s.Timeout <= 0 {
		return errors.New("SEARCH_TIMEOUT_MS 必须为正整数")
	}
	if s.MinScore < 0 || s.MinScore >= 1 {
		return errors.New("SEARCH_MIN_SCORE 必须在 [0,1) 之间")
	}
	return nil
}

// envFloat 读取浮点型配置。
func envFloat(key string, fallback float64) float64 {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return fallback
}

// envPositive 读取正整数配置，非法或非正时回落到默认值。
func envPositive(key string, fallback int) int {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return fallback
}

// envDurationMs 读取以毫秒为单位的时长。
func envDurationMs(key string, fallback time.Duration) time.Duration {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return time.Duration(n) * time.Millisecond
		}
	}
	return fallback
}

// validateSeedPasswords 只做最低强度的检查，避免把 "change-me" 一类口令当作可用的预置账号。
func validateSeedPasswords(s Seed) error {
	values := map[string]string{
		"SEED_TEACHER_A_PASSWORD":  s.TeacherAPassword,
		"SEED_STUDENT_A1_PASSWORD": s.StudentA1Password,
		"SEED_STUDENT_B1_PASSWORD": s.StudentB1Password,
	}
	for key, v := range values {
		if v == "" {
			continue // 缺失与否由 required 统一报告
		}
		if len(v) < 8 {
			return fmt.Errorf("%s 至少需要 8 位", key)
		}
	}
	return nil
}

func envDefault(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func envInt64(key string, fallback int64) int64 {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return n
		}
	}
	return fallback
}

// envDuration 读取"小时/分钟"为单位的数值并换算成 time.Duration。
func envDuration(key string, unit time.Duration) time.Duration {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return time.Duration(n) * unit
		}
	}
	return unit
}

func unique(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := in[:0]
	for _, v := range in {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}
