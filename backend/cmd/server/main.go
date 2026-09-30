// Command server 是 CampusClaw 迭代 1 的 API 服务入口。
//
// 启动顺序：读配置（缺必填项即失败）→ 等待数据库就绪 → 建表 → 幂等种子 → 开始监听。
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"campusclaw/backend/internal/answer"
	"campusclaw/backend/internal/auth"
	"campusclaw/backend/internal/config"
	"campusclaw/backend/internal/db"
	"campusclaw/backend/internal/kb"
	"campusclaw/backend/internal/materials"
	"campusclaw/backend/internal/search"
	"campusclaw/backend/internal/server"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	if err := run(); err != nil {
		slog.Error("启动失败", "error", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	pool, err := db.Open(ctx, cfg.DB)
	if err != nil {
		return err
	}
	defer pool.Close()

	if err := db.Migrate(ctx, pool); err != nil {
		return err
	}

	hasher, err := auth.NewHasher(0)
	if err != nil {
		return err
	}
	// 检索所需的嵌入器与索引器：默认是本地确定性嵌入（见 change 的 design.md D3）。
	embedder := kb.NewHashingEmbedder(cfg.Search.EmbeddingDim)
	indexer := &kb.Indexer{
		Embedder:  embedder,
		ChunkSize: cfg.Search.ChunkSize,
		Overlap:   cfg.Search.ChunkOverlap,
	}

	// 维度不一致时必须拒绝启动，避免用旧向量提供不可解释的结果。
	if err := db.CheckEmbeddingDim(ctx, pool, cfg.Search.EmbeddingDim); err != nil {
		return err
	}
	if err := db.Seed(ctx, pool, cfg.Seed, hasher, cfg.UploadDir, indexer); err != nil {
		return err
	}
	// 为迭代 1 时期已入库、还没有索引块的材料补建索引。
	if n, err := indexer.IndexMissing(ctx, pool); err != nil {
		return err
	} else if n > 0 {
		slog.Info("已为历史材料补建索引", "materials", n)
	}

	sessions := auth.NewSessionStore(pool, cfg.SessionSecret, cfg.SessionTTL)
	limiter := auth.NewLimiter(cfg.LoginMaxAttempts, cfg.LoginWindow, cfg.LoginLock)
	// token 方案：HS256 访问令牌 + 可撤销的刷新令牌
	tokens := auth.NewTokenService(pool, cfg.JWTSecret, cfg.AccessTokenTTL, cfg.RefreshTokenTTL)

	searchService := &search.Service{
		DB:       pool,
		Embedder: embedder,
		TopKMax:  cfg.Search.TopKMax,
		QueryMax: cfg.Search.QueryMaxChars,
		MinScore: cfg.Search.MinScore,
	}

	api := server.New(server.Deps{
		Config:   cfg,
		DB:       pool,
		Sessions: sessions,
		Tokens:   tokens,
		Hasher:   hasher,
		Limiter:  limiter,
		Materials: &materials.Handler{
			Store:          materials.NewStore(pool).WithIndexer(indexer),
			UploadDir:      cfg.UploadDir,
			MaxUploadBytes: cfg.MaxUploadBytes,
		},
		Answer: &answer.Service{
			Search: searchService,
			// 未配置对话网关时使用本地摘录实现：回答只由检索到的切片拼成，
			// 不凭空生成内容，同样满足"可溯源"。配置网关后换成真实模型即可。
			Chat: answer.ExtractiveClient{},
		},
		Search: searchService,
	})

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           api,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}

	go sweep(ctx, limiter, sessions)

	errc := make(chan error, 1)
	go func() {
		slog.Info("开始监听", "addr", cfg.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errc <- err
		}
	}()

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}

	slog.Info("收到退出信号，开始优雅关闭")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

// sweep 定期清理登录限流条目与过期会话行，避免内存与表无界增长。
func sweep(ctx context.Context, limiter *auth.Limiter, sessions *auth.SessionStore) {
	limiterTick := time.NewTicker(5 * time.Minute)
	sessionTick := time.NewTicker(30 * time.Minute)
	defer limiterTick.Stop()
	defer sessionTick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-limiterTick.C:
			limiter.Sweep()
		case <-sessionTick.C:
			if err := sessions.DeleteExpired(ctx); err != nil {
				slog.Warn("清理过期会话失败", "error", err)
			}
		}
	}
}
