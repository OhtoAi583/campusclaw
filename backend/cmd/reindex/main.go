// Command reindex 重建知识库检索索引。
//
// 用途：种子数据、迭代 1 时期已入库的历史材料、以及调整 CHUNK_SIZE / EMBEDDING_DIM 之后重新生成块。
// 它是幂等的：先删该材料的旧块再重建，重复执行不会产生重复块。
//
// 用法：bin/reindex
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"campusclaw/backend/internal/config"
	"campusclaw/backend/internal/db"
	"campusclaw/backend/internal/kb"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "重建索引失败:", err)
		os.Exit(1)
	}
}

func run() error {
	ctx := context.Background()

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

	indexer := &kb.Indexer{
		Embedder:  kb.NewHashingEmbedder(cfg.Search.EmbeddingDim),
		ChunkSize: cfg.Search.ChunkSize,
		Overlap:   cfg.Search.ChunkOverlap,
	}
	materials, chunks, err := indexer.RebuildAll(ctx, pool)
	if err != nil {
		return err
	}
	slog.Info("索引重建完成", "materials", materials, "chunks", chunks, "embedding_dim", cfg.Search.EmbeddingDim)
	fmt.Printf("索引重建完成：材料 %d 篇，块 %d 个，向量维度 %d\n", materials, chunks, cfg.Search.EmbeddingDim)
	return nil
}
