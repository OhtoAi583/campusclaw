package db

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"campusclaw/backend/internal/auth"
	"campusclaw/backend/internal/config"
	"campusclaw/backend/internal/kb"
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
				"本设计面向五年级语文阅读单元，整节课 40 分钟，重点是训练概括能力与语言积累。\n\n" +
				"## 教学目标\n\n" +
				"- 能用一句话概括段落主要内容，说清楚「谁、做了什么、结果如何」。\n" +
				"- 能找出文中的关键句，并说明它为什么关键。\n" +
				"- 能在仿写中迁移本课的语言材料，写出两处以上细节描写。\n\n" +
				"## 课堂流程\n\n" +
				"| 环节 | 时长 | 说明 |\n| --- | --- | --- |\n" +
				"| 导入 | 5 分钟 | 由情境入题，先让学生预测内容 |\n" +
				"| 精读 | 20 分钟 | 抓关键句，圈画批注，同桌互说 |\n" +
				"| 迁移 | 10 分钟 | 仿写练习，写两处细节 |\n" +
				"| 小结 | 5 分钟 | 回顾方法，布置分层作业 |\n\n" +
				"## 导入环节的设计意图\n\n" +
				"导入环节用一张生活情境图引发预测，目的是把学生的注意力拉到「内容会怎么发展」上，\n" +
				"为后面的概括训练做铺垫。教师只提问不评价，先收集学生的不同预测，再在精读环节逐一验证。\n\n" +
				"## 分层作业设计\n\n" +
				"基础层：用一句话概括全文；提高层：找出两处关键句并说明理由；挑战层：仿写一段细节描写。\n\n" +
				"## 教学反思要点\n\n" +
				"精读时间容易被个别学生的发言拖长，需要提前约定「一句话回答」的规则；\n" +
				"概括能力的评价标准要提前给到学生，避免课堂上临时加码。\n\n" +
				"> 本条目属于 A 班，B 班账号不应读到；用于演示班级隔离与内容溯源。\n",
		},
		{
			class:        "B",
			title:        "B班·教研材料示例：数学课堂练习设计",
			originalName: "B班-数学课堂练习设计.md",
			storedName:   "seed-b-class-math.md",
			content: "# B班 数学课堂练习设计\n\n" +
				"本设计面向六年级分数单元，练习课 40 分钟，目标是巩固分数四则运算并养成验算习惯。\n\n" +
				"## 练习目标\n\n" +
				"- 能正确完成分数四则运算，并说明每一步的依据。\n" +
				"- 能主动使用估算与逆运算进行验算。\n\n" +
				"## 练习结构\n\n" +
				"| 层级 | 题量 | 说明 |\n| --- | --- | --- |\n" +
				"| 基础 | 8 | 直接计算，限时完成 |\n" +
				"| 提升 | 4 | 简单应用题，先画图再列式 |\n" +
				"| 拓展 | 2 | 开放题，鼓励多种解法 |\n\n" +
				"## 验算习惯的培养\n\n" +
				"每道提升题做完后必须写一行验算；教师抽查验算过程而不是只看答案，\n" +
				"连续两周坚持后，把验算从「要求」变成「习惯」。\n\n" +
				"## 常见错误与应对\n\n" +
				"分数除法忘记乘以倒数、通分时漏乘分子，是本次练习最常见的两类错误，\n" +
				"处理办法是让学生在错题旁标注错因，而不是直接改答案。\n\n" +
				"> 本条目属于 B 班，A 班账号不应读到；用于演示班级隔离与内容溯源。\n",
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
func Seed(ctx context.Context, pool *sql.DB, secrets config.Seed, hasher *auth.Hasher, uploadDir string, indexer *kb.Indexer) error {
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
		// 种子材料同时建立检索索引，保证"从零启动即可检索"。
		if indexer != nil {
			if err := indexer.WriteTx(ctx, tx, materialID, classes[m.class], m.content); err != nil {
				_ = tx.Rollback()
				return fmt.Errorf("写入种子检索索引失败: %w", err)
			}
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("提交种子事务失败: %w", err)
		}
	}
	return nil
}
