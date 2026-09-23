package config

import (
	"strings"
	"testing"
)

// 缺必填项时必须报错，并且错误信息里点名缺了哪些变量（R7.3）。
func TestLoadFailsWhenRequiredEnvMissing(t *testing.T) {
	clearRequired(t)
	t.Setenv("DB_NAME", "campusclaw")

	_, err := Load()
	if err == nil {
		t.Fatal("缺少必填项时应返回错误")
	}
	for _, key := range []string{"SESSION_SECRET", "DB_USER", "DB_PASSWORD", "SEED_TEACHER_A_PASSWORD"} {
		if !strings.Contains(err.Error(), key) {
			t.Errorf("错误信息应包含 %s，实际为 %v", key, err)
		}
	}
}

func TestLoadSucceedsWithAllRequiredEnv(t *testing.T) {
	setRequired(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("填齐必填项后不应报错: %v", err)
	}
	if cfg.Addr != ":8080" {
		t.Errorf("默认监听地址应为 :8080，实际 %q", cfg.Addr)
	}
	if cfg.MaxUploadBytes <= 0 {
		t.Errorf("上传上限应为正数，实际 %d", cfg.MaxUploadBytes)
	}
	if !strings.Contains(cfg.DB.DSN(), "campusclaw_user") {
		t.Errorf("DSN 应包含数据库用户，实际 %q", cfg.DB.DSN())
	}
}

// 过短的种子口令应被直接拒绝，避免把 "change-me" 一类口令当成可用账号。
func TestLoadRejectsWeakSeedPassword(t *testing.T) {
	setRequired(t)
	t.Setenv("SEED_TEACHER_A_PASSWORD", "short")

	if _, err := Load(); err == nil {
		t.Fatal("过短的种子口令应被拒绝")
	}
}

func clearRequired(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"SESSION_SECRET", "DB_NAME", "DB_USER", "DB_PASSWORD",
		"SEED_TEACHER_A_PASSWORD", "SEED_STUDENT_A1_PASSWORD", "SEED_STUDENT_B1_PASSWORD",
	} {
		t.Setenv(key, "")
	}
}

func setRequired(t *testing.T) {
	t.Helper()
	t.Setenv("SESSION_SECRET", "unit-test-secret")
	t.Setenv("DB_NAME", "campusclaw")
	t.Setenv("DB_USER", "campusclaw_user")
	t.Setenv("DB_PASSWORD", "unit-test-db-password")
	t.Setenv("SEED_TEACHER_A_PASSWORD", "teacher-password")
	t.Setenv("SEED_STUDENT_A1_PASSWORD", "student-a-password")
	t.Setenv("SEED_STUDENT_B1_PASSWORD", "student-b-password")
}
