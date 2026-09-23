// Package materials 负责材料与知识库入库的读写。
//
// 两条读取路径刻意分开：
//   - 列表：WHERE class_id = 会话班级（直接由数据库完成隔离过滤）
//   - 按 ID：WHERE id = ?，不带班级条件，交由上层"先取行再核对班级"
//
// 若按 ID 的查询提前按班级过滤，跨班访问会退化成"空结果"，无法与"记录不存在"区分，
// 也就无法对外给出同形的 404。
package materials

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"campusclaw/backend/internal/knowledge"
)

// ErrNotFound 表示记录不存在；上层对外统一映射成 404。
var ErrNotFound = errors.New("material not found")

// Material 是一条材料（含知识库正文与上传者名）。
type Material struct {
	ID           int64     `json:"id"`
	ClassID      int64     `json:"class_id"`
	Title        string    `json:"title"`
	OriginalName string    `json:"original_name"`
	StoredName   string    `json:"stored_name,omitempty"`
	SizeBytes    int64     `json:"size_bytes"`
	UploadedBy   int64     `json:"uploaded_by"`
	UploaderName string    `json:"uploader_name"`
	CreatedAt    time.Time `json:"created_at"`
	Content      string    `json:"content,omitempty"`
}

// CreateInput 是新建材料所需的字段。ClassID 只能由会话给出，绝不来自请求参数。
type CreateInput struct {
	ClassID      int64
	Title        string
	OriginalName string
	StoredName   string
	SizeBytes    int64
	UploadedBy   int64
	Content      string
}

// Store 是材料的数据访问层。
type Store struct {
	db *sql.DB
}

// NewStore 构造材料存储。
func NewStore(db *sql.DB) *Store { return &Store{db: db} }

// ListByClass 返回本班材料。隔离条件写死在 SQL 里，调用方无法把它省掉。
func (s *Store) ListByClass(ctx context.Context, classID int64) ([]Material, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT m.id, m.class_id, m.title, m.original_name, m.stored_name, m.size_bytes,
		       m.uploaded_by, u.username, m.created_at
		FROM materials m
		JOIN users u ON u.id = m.uploaded_by
		WHERE m.class_id = ?
		ORDER BY m.created_at DESC, m.id DESC`, classID)
	if err != nil {
		return nil, fmt.Errorf("查询材料列表失败: %w", err)
	}
	defer rows.Close()

	list := make([]Material, 0)
	for rows.Next() {
		var m Material
		if err := rows.Scan(&m.ID, &m.ClassID, &m.Title, &m.OriginalName, &m.StoredName,
			&m.SizeBytes, &m.UploadedBy, &m.UploaderName, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("扫描材料失败: %w", err)
		}
		list = append(list, m)
	}
	return list, rows.Err()
}

// GetByID 按 ID 取行，不带班级条件——班级核对留给上层。
func (s *Store) GetByID(ctx context.Context, id int64) (Material, error) {
	var m Material
	err := s.db.QueryRowContext(ctx, `
		SELECT m.id, m.class_id, m.title, m.original_name, m.stored_name, m.size_bytes,
		       m.uploaded_by, u.username, m.created_at, COALESCE(k.content, '')
		FROM materials m
		JOIN users u ON u.id = m.uploaded_by
		LEFT JOIN knowledge_entries k ON k.material_id = m.id
		WHERE m.id = ?`, id).
		Scan(&m.ID, &m.ClassID, &m.Title, &m.OriginalName, &m.StoredName,
			&m.SizeBytes, &m.UploadedBy, &m.UploaderName, &m.CreatedAt, &m.Content)
	if errors.Is(err, sql.ErrNoRows) {
		return Material{}, ErrNotFound
	}
	if err != nil {
		return Material{}, fmt.Errorf("查询材料失败: %w", err)
	}
	return m, nil
}

// Create 在同一事务中写入 materials 与 knowledge_entries。
// 事务的调用方负责在失败时删除已经落盘的文件，避免孤儿文件。
func (s *Store) Create(ctx context.Context, in CreateInput) (Material, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Material{}, fmt.Errorf("开启事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx, `
		INSERT INTO materials (class_id, title, original_name, stored_name, size_bytes, uploaded_by)
		VALUES (?, ?, ?, ?, ?, ?)`,
		in.ClassID, in.Title, in.OriginalName, in.StoredName, in.SizeBytes, in.UploadedBy)
	if err != nil {
		return Material{}, fmt.Errorf("写入材料失败: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Material{}, fmt.Errorf("读取材料 id 失败: %w", err)
	}
	if err := knowledge.InsertTx(ctx, tx, id, in.ClassID, in.Content); err != nil {
		return Material{}, err
	}
	if err := tx.Commit(); err != nil {
		return Material{}, fmt.Errorf("提交事务失败: %w", err)
	}
	return s.GetByID(ctx, id)
}
