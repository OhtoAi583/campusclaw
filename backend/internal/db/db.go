// Package db 负责连接数据库、建表与写入幂等种子数据。
package db

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"log/slog"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"campusclaw/backend/internal/config"
)

//go:embed migrations/001_init.sql
var schemaSQL string

// Open 建立连接池并在超时时间内重试等待数据库可连接。
// depends_on 只保证启动顺序，不保证数据库已经能接受连接，因此重试是必需的。
func Open(ctx context.Context, cfg config.DB) (*sql.DB, error) {
	pool, err := sql.Open("mysql", cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("打开数据库失败: %w", err)
	}
	pool.SetMaxOpenConns(16)
	pool.SetMaxIdleConns(8)
	pool.SetConnMaxLifetime(30 * time.Minute)

	deadline := time.Now().Add(60 * time.Second)
	for {
		pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		err = pool.PingContext(pingCtx)
		cancel()
		if err == nil {
			return pool, nil
		}
		if time.Now().After(deadline) {
			pool.Close()
			return nil, fmt.Errorf("等待数据库就绪超时: %w", err)
		}
		slog.Info("等待数据库就绪", "error", err)
		select {
		case <-ctx.Done():
			pool.Close()
			return nil, ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}

// Migrate 执行建表语句。语句全部是 CREATE TABLE IF NOT EXISTS，可重复执行。
func Migrate(ctx context.Context, pool *sql.DB) error {
	for _, stmt := range splitStatements(schemaSQL) {
		if _, err := pool.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("执行建表语句失败: %w", err)
		}
	}
	return nil
}

// splitStatements 按分号切分 SQL 文件；本文件不含存储过程，逐行累积到分号结尾即可。
func splitStatements(script string) []string {
	var out []string
	var current strings.Builder
	for _, line := range strings.Split(script, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "--") {
			continue
		}
		current.WriteString(line)
		current.WriteString("\n")
		if strings.HasSuffix(strings.TrimSpace(line), ";") {
			if stmt := strings.TrimSpace(current.String()); stmt != "" {
				out = append(out, stmt)
			}
			current.Reset()
		}
	}
	if stmt := strings.TrimSpace(current.String()); stmt != "" {
		out = append(out, stmt)
	}
	return out
}
