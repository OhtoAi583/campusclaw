// Package knowledge 管理知识库正文。
// 本迭代的定位只是"存下可查询的正文"，为第 4 课的检索预置数据：
// 这里既不做向量化，也不提供问答接口——那是 Non-goals 里明确不做的部分。
package knowledge

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Entry 是一条知识库正文，与材料一一对应。
type Entry struct {
	ID         int64
	MaterialID int64
	ClassID    int64
	Content    string
	CreatedAt  time.Time
}

// InsertTx 在调用方的同一个事务里写入正文。
// 材料行与正文行必须同事务：只写材料则第 4 课检索无数据，只写正文则下载无文件。
func InsertTx(ctx context.Context, tx *sql.Tx, materialID, classID int64, content string) error {
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO knowledge_entries (material_id, class_id, content) VALUES (?, ?, ?)`,
		materialID, classID, content); err != nil {
		return fmt.Errorf("写入知识库正文失败: %w", err)
	}
	return nil
}

// ContentByMaterial 读取某条材料的正文，供详情接口使用。
func ContentByMaterial(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, materialID, classID int64) (string, error) {
	var content string
	err := q.QueryRowContext(ctx,
		`SELECT content FROM knowledge_entries WHERE material_id = ? AND class_id = ? LIMIT 1`,
		materialID, classID).Scan(&content)
	if err != nil {
		return "", err
	}
	return content, nil
}
