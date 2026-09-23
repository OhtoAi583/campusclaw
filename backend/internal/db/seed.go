package db

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"campusclaw/backend/internal/auth"
	"campusclaw/backend/internal/config"
)

type seedUser struct {
	username string
	role     string
	class    string
	password string
}

type seedMaterial struct {
	class        string
	title        string
	originalName string
	storedName   string
	content      string
}

// seedMaterialsToInsert 的两条标题刻意做成可区分的：
// 若 A / B 两班材料的标题无法区分，跨班访问失败就没有可判定的对照。
func seedMaterialsToInsert() []seedMaterial {
	return []seedMaterial{
		{
			class:        "A",
			title:        "A班·教研材料示例：语文阅读课教学设计",
			originalName: "A班-语文阅读课教学设计.md",
			storedName:   "seed-a-class-reading.md",
			content: "# A班 语文阅读课教学设计\n\n" +
				"## 目标\n\n- 训练概括能力\n- 积累语言材料\n\n" +
				"## 课堂流程\n\n| 环节 | 时长 | 说明 |\n| --- | --- | --- |\n| 导入 | 5 分钟 | 由情境入题 |\n| 精读 | 20 分钟 | 抓关键句 |\n| 迁移 | 10 分钟 | 仿写练习 |\n\n" +
				"> 本条目由种子数据写入，属于 A 班，B 班账号不应读到。\n",
		},
		{
			class:        "B",
			title:        "B班·教研材料示例：数学课堂练习设计",
			originalName: "B班-数学课堂练习设计.md",
			storedName:   "seed-b-class-math.md",
			content: "# B班 数学课堂练习设计\n\n" +
				"## 目标\n\n- 巩固分数四则运算\n- 提升验算习惯\n\n" +
				"## 练习结构\n\n| 层级 | 题量 | 说明 |\n| --- | --- | --- |\n| 基础 | 8 | 直接计算 |\n| 提升 | 4 | 简单应用题 |\n\n" +
				"> 本条目由种子数据写入，属于 B 班，A 班账号不应读到。\n",
		},
	}
}

func seedUsersToInsert(secrets config.Seed) []seedUser {
	return []seedUser{
		{username: "teacher_a", role: "teacher", class: "A", password: secrets.TeacherAPassword},
		{username: "student_a1", role: "student", class: "A", password: secrets.StudentA1Password},
		{username: "student_b1", role: "student", class: "B", password: secrets.StudentB1Password},
	}
}

// Seed 写入预置数据，可重复执行：
// 每一条都先查后插，重复启动既不会复制用户与材料，也不会覆盖教师已上传的内容。
func Seed(ctx context.Context, pool *sql.DB, secrets config.Seed, hasher *auth.Hasher, uploadDir string) error {
	classes := map[string]int64{}
	for _, name := range []string{"A", "B"} {
		if _, err := pool.ExecContext(ctx, `INSERT IGNORE INTO classes (name) VALUES (?)`, name); err != nil {
			return fmt.Errorf("写入班级失败: %w", err)
		}
		var id int64
		if err := pool.QueryRowContext(ctx, `SELECT id FROM classes WHERE name = ?`, name).Scan(&id); err != nil {
			return fmt.Errorf("读取班级失败: %w", err)
		}
		classes[name] = id
	}

	userIDs := map[string]int64{}
	for _, u := range seedUsersToInsert(secrets) {
		var id int64
		err := pool.QueryRowContext(ctx, `SELECT id FROM users WHERE username = ?`, u.username).Scan(&id)
		switch {
		case err == nil:
			userIDs[u.username] = id
			continue
		case err != sql.ErrNoRows:
			return fmt.Errorf("读取用户失败: %w", err)
		}
		hash, err := hasher.Hash(u.password)
		if err != nil {
			return err
		}
		res, err := pool.ExecContext(ctx,
			`INSERT INTO users (username, password_hash, role, class_id) VALUES (?, ?, ?, ?)`,
			u.username, hash, u.role, classes[u.class])
		if err != nil {
			return fmt.Errorf("写入用户失败: %w", err)
		}
		id, err = res.LastInsertId()
		if err != nil {
			return fmt.Errorf("读取用户 id 失败: %w", err)
		}
		userIDs[u.username] = id
	}

	if err := os.MkdirAll(uploadDir, 0o750); err != nil {
		return fmt.Errorf("创建上传目录失败: %w", err)
	}

	for _, m := range seedMaterialsToInsert() {
		var existing int64
		if err := pool.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM materials WHERE class_id = ? AND title = ?`,
			classes[m.class], m.title).Scan(&existing); err != nil {
			return fmt.Errorf("检查种子材料失败: %w", err)
		}
		if existing > 0 {
			continue
		}

		// 种子材料同样把原文件落到上传目录，保证"下载走鉴权接口"对种子数据也成立。
		path := filepath.Join(uploadDir, m.storedName)
		if err := os.WriteFile(path, []byte(m.content), 0o640); err != nil {
			return fmt.Errorf("写入种子材料文件失败: %w", err)
		}

		tx, err := pool.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("开启事务失败: %w", err)
		}
		res, err := tx.ExecContext(ctx, `
			INSERT INTO materials (class_id, title, original_name, stored_name, size_bytes, uploaded_by)
			VALUES (?, ?, ?, ?, ?, ?)`,
			classes[m.class], m.title, m.originalName, m.storedName, len(m.content), userIDs["teacher_a"])
		if err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("写入种子材料失败: %w", err)
		}
		materialID, err := res.LastInsertId()
		if err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("读取种子材料 id 失败: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO knowledge_entries (material_id, class_id, content) VALUES (?, ?, ?)`,
			materialID, classes[m.class], m.content); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("写入种子知识库正文失败: %w", err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("提交种子事务失败: %w", err)
		}
	}
	return nil
}
