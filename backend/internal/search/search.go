// Package search 实现"限定班级范围的知识库检索"。
//
// 两条不可退让的约束（见 change 的 design.md）：
//  1. 候选集在 SQL 层就按会话班级过滤，跨班数据从不进入候选集，应用层再做一次断言；
//  2. 每条结果必须可溯源：带材料 id、标题、原始文件名、块序号与字符区间，且片段原文与该区间逐字符一致。
package search

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"

	"campusclaw/backend/internal/kb"
)

// ErrInvalidQuery 表示查询参数不合法（空、超长、top_k 越界），上层映射为 400。
var ErrInvalidQuery = errors.New("invalid query")

// Result 是一条检索结果。字段与 spec R3.1 一一对应。
// 注意：这里绝不返回 stored_name、磁盘路径或任何跨班信息。
type Result struct {
	MaterialID    int64   `json:"material_id"`
	MaterialTitle string  `json:"material_title"`
	OriginalName  string  `json:"original_name"`
	ChunkIndex    int     `json:"chunk_index"`
	StartOffset   int     `json:"start_offset"`
	EndOffset     int     `json:"end_offset"`
	Content       string  `json:"content"`
	Score         float64 `json:"score"`
}

// Service 执行检索。
type Service struct {
	DB       *sql.DB
	Embedder kb.Embedder
	TopKMax  int
	QueryMax int
	// MinScore 以下的结果不返回（哈希嵌入的碰撞噪声会产生无关的低分候选）。
	MinScore float64
}

type candidate struct {
	materialID   int64
	classID      int64
	chunkIndex   int
	start, end   int
	content      string
	title        string
	originalName string
	vector       []float32
}

// Search 在指定班级范围内召回 topK 条结果。
// classID 只能来自会话，调用方不得把它取自请求参数。
func (s *Service) Search(ctx context.Context, classID int64, query string, topK int) ([]Result, int, error) {
	trimmed := strings.TrimSpace(query)
	if trimmed == "" {
		return nil, 0, fmt.Errorf("%w: 查询不能为空", ErrInvalidQuery)
	}
	if len([]rune(trimmed)) > s.QueryMax {
		return nil, 0, fmt.Errorf("%w: 查询长度超过上限 %d", ErrInvalidQuery, s.QueryMax)
	}
	if topK <= 0 {
		topK = 5
	}
	if topK > s.TopKMax {
		return nil, 0, fmt.Errorf("%w: top_k 超过上限 %d", ErrInvalidQuery, s.TopKMax)
	}

	rows, err := s.DB.QueryContext(ctx, `
		SELECT c.material_id, c.class_id, c.chunk_index, c.start_offset, c.end_offset,
		       c.content, c.embedding, m.title, m.original_name
		FROM kb_chunks c
		JOIN materials m ON m.id = c.material_id
		WHERE c.class_id = ?
		ORDER BY c.material_id, c.chunk_index`, classID)
	if err != nil {
		return nil, 0, fmt.Errorf("读取候选块失败: %w", err)
	}
	defer rows.Close()

	dim := s.Embedder.Dim()
	candidates := make([]candidate, 0, 64)
	for rows.Next() {
		var c candidate
		var raw []byte
		if err := rows.Scan(&c.materialID, &c.classID, &c.chunkIndex, &c.start, &c.end,
			&c.content, &raw, &c.title, &c.originalName); err != nil {
			return nil, 0, fmt.Errorf("扫描候选块失败: %w", err)
		}
		vector, err := kb.DecodeVector(raw, dim)
		if err != nil {
			return nil, 0, fmt.Errorf("解码向量失败（material_id=%d chunk=%d）: %w", c.materialID, c.chunkIndex, err)
		}
		c.vector = vector
		candidates = append(candidates, c)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	queryVector := s.Embedder.Embed(trimmed)
	scored := make([]Result, 0, len(candidates))
	for _, c := range candidates {
		// 应用层二次断言：即使 SQL 条件被改坏，跨班块也不会被返回。
		if c.classID != classID {
			continue
		}
		score := kb.Dot(queryVector, c.vector)
		// 低于阈值的候选算作"没有匹配"：哈希嵌入会有少量碰撞噪声，
		// 不设阈值时无关查询也会返回一堆低分结果（spec R2.4）。
		if score < s.MinScore {
			continue
		}
		scored = append(scored, Result{
			MaterialID:    c.materialID,
			MaterialTitle: c.title,
			OriginalName:  c.originalName,
			ChunkIndex:    c.chunkIndex,
			StartOffset:   c.start,
			EndOffset:     c.end,
			Content:       c.content,
			Score:         score,
		})
	}

	// 稳定排序：分数降序，同分按 material_id、chunk_index，保证同样输入得到同样顺序（R5.2）。
	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].Score != scored[j].Score {
			return scored[i].Score > scored[j].Score
		}
		if scored[i].MaterialID != scored[j].MaterialID {
			return scored[i].MaterialID < scored[j].MaterialID
		}
		return scored[i].ChunkIndex < scored[j].ChunkIndex
	})
	if len(scored) > topK {
		scored = scored[:topK]
	}
	return scored, len(candidates), nil
}
