package kb

import (
	"context"
	"database/sql"
	"fmt"
)

// Indexer 把一份正文写成若干块并计算向量，写库必须发生在调用方的事务里，
// 以便与 materials / knowledge_entries 同生共死。
type Indexer struct {
	Embedder  Embedder
	ChunkSize int
	Overlap   int
}

// WriteTx 在给定事务中写入某篇材料的全部块。
// 写入前先删掉该材料的旧块，因此本函数天然幂等：重复执行不会产生重复块。
func (ix *Indexer) WriteTx(ctx context.Context, tx *sql.Tx, materialID, classID int64, content string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM kb_chunks WHERE material_id = ?`, materialID); err != nil {
		return fmt.Errorf("清理旧索引失败: %w", err)
	}
	chunks := Split(content, ix.ChunkSize, ix.Overlap)
	for _, chunk := range chunks {
		vector := ix.Embedder.Embed(chunk.Content)
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO kb_chunks
				(material_id, class_id, chunk_index, start_offset, end_offset, content, embedding, embedding_dim)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			materialID, classID, chunk.Index, chunk.Start, chunk.End,
			chunk.Content, EncodeVector(vector), ix.Embedder.Dim(),
		); err != nil {
			return fmt.Errorf("写入索引块失败: %w", err)
		}
	}
	return nil
}

// RebuildAll 重建全部材料的索引（幂等），用于种子数据、历史数据与分块参数调整后的补建。
// 返回处理过的材料数与写入的块数。
func (ix *Indexer) RebuildAll(ctx context.Context, db *sql.DB) (materials, chunks int, err error) {
	rows, err := db.QueryContext(ctx, `
		SELECT m.id, m.class_id, COALESCE(k.content, '')
		FROM materials m
		LEFT JOIN knowledge_entries k ON k.material_id = m.id
		ORDER BY m.id`)
	if err != nil {
		return 0, 0, fmt.Errorf("读取材料失败: %w", err)
	}
	type row struct {
		id      int64
		classID int64
		content string
	}
	var list []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.classID, &r.content); err != nil {
			rows.Close()
			return 0, 0, err
		}
		list = append(list, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, 0, err
	}

	for _, r := range list {
		if r.content == "" {
			continue
		}
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return materials, chunks, err
		}
		if err := ix.WriteTx(ctx, tx, r.id, r.classID, r.content); err != nil {
			_ = tx.Rollback()
			return materials, chunks, err
		}
		if err := tx.Commit(); err != nil {
			return materials, chunks, err
		}
		materials++
		chunks += len(Split(r.content, ix.ChunkSize, ix.Overlap))
	}
	return materials, chunks, nil
}

// IndexMissing 只给"还没有任何块"的材料补建索引，在服务启动时调用，
// 用于覆盖迭代 1 时期已入库的历史材料。
func (ix *Indexer) IndexMissing(ctx context.Context, db *sql.DB) (int, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT m.id, m.class_id, COALESCE(k.content, '')
		FROM materials m
		LEFT JOIN knowledge_entries k ON k.material_id = m.id
		WHERE NOT EXISTS (SELECT 1 FROM kb_chunks c WHERE c.material_id = m.id)
		ORDER BY m.id`)
	if err != nil {
		return 0, fmt.Errorf("读取待补建材料失败: %w", err)
	}
	type row struct {
		id      int64
		classID int64
		content string
	}
	var list []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.classID, &r.content); err != nil {
			rows.Close()
			return 0, err
		}
		list = append(list, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	count := 0
	for _, r := range list {
		if r.content == "" {
			continue
		}
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return count, err
		}
		if err := ix.WriteTx(ctx, tx, r.id, r.classID, r.content); err != nil {
			_ = tx.Rollback()
			return count, err
		}
		if err := tx.Commit(); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}
