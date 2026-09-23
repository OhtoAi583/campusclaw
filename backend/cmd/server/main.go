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

	"campusclaw/backend/internal/auth"
	"campusclaw/backend/internal/config"
	"campusclaw/backend/internal/db"
	"campusclaw/backend/internal/materials"
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
	if err := db.Seed(ctx, pool, cfg.Seed, hasher, cfg.UploadDir); err != nil {
		return err
	}

	sessions := auth.NewSessionStore(pool, cfg.SessionSecret, cfg.SessionTTL)
	limiter := auth.NewLimiter(cfg.LoginMaxAttempts, cfg.LoginWindow, cfg.LoginLock)

	api := server.New(server.Deps{
		Config:   cfg,
		DB:       pool,
		Sessions: sessions,
		Hasher:   hasher,
		Limiter:  limiter,
		Materials: &materials.Handler{
			Store:          materials.NewStore(pool),
			UploadDir:      cfg.UploadDir,
			MaxUploadBytes: cfg.MaxUploadBytes,
		},
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
