// Package db 负责连接数据库、建表与写入幂等种子数据。
package db

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"sort"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"campusclaw/backend/internal/config"
)

//go:embed migrations/*.sql
var migrations embed.FS

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

// Migrate 按文件名顺序执行 migrations 下的建表语句。
// 语句全部是 CREATE TABLE IF NOT EXISTS，可重复执行。
func Migrate(ctx context.Context, pool *sql.DB) error {
	entries, err := fs.ReadDir(migrations, "migrations")
	if err != nil {
		return fmt.Errorf("读取迁移目录失败: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	for _, name := range names {
		script, err := fs.ReadFile(migrations, "migrations/"+name)
		if err != nil {
			return fmt.Errorf("读取迁移 %s 失败: %w", name, err)
		}
		for _, stmt := range splitStatements(string(script)) {
			if _, err := pool.ExecContext(ctx, stmt); err != nil {
				return fmt.Errorf("执行迁移 %s 失败: %w", name, err)
			}
		}
		slog.Info("迁移已应用", "file", name)
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

// CheckEmbeddingDim 校验库中已有向量的维度与当前配置一致（spec R5.4）。
// 改过 EMBEDDING_DIM 却没有重建索引时，必须拒绝启动，而不是用旧向量提供不可解释的检索结果。
func CheckEmbeddingDim(ctx context.Context, pool *sql.DB, want int) error {
	var got int
	err := pool.QueryRowContext(ctx, `SELECT embedding_dim FROM kb_chunks LIMIT 1`).Scan(&got)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("读取向量维度失败: %w", err)
	}
	if got != want {
		return fmt.Errorf("索引向量维度为 %d，当前 EMBEDDING_DIM 为 %d；请先执行重建索引（bin/reindex）", got, want)
	}
	return nil
}
