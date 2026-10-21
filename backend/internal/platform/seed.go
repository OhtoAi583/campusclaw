package platform

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
)

type seedTemplate struct {
	id, name, description, system, user, vars string
}

type seedApp struct {
	id, name, description, icon, templateID string
	kbEnabled                               bool
	kbTopK                                  int
	skills                                  []string
	mcpServers                              []string
}

// builtinSkills 是内置技能。技能正文会**完整**注入 system prompt（有长度上限），
// 这正是验收要点之一：绑定的技能全部内容都会随请求传给模型。
var builtinSkills = []struct {
	id, name, description, body string
}{
	{
		id: "cite-check", name: "引用校验",
		description: "要求每条结论标注出处编号，不得编造来源",
		body: `# 引用校验

## 何时使用
任何基于材料的回答都必须启用。

## 规则
1. 每一条事实性结论后面必须紧跟 [编号]，编号对应用户消息里提供的材料片段序号。
2. 只能引用实际出现的片段；不得编造不存在的编号或来源。
3. 如果片段不足以支撑某个结论，直接写明「资料中未找到相关内容」，不要推测。
4. 多个片段共同支撑时写成 [1][3]。

## 输出前自检
- 逐一检查每个段落是否都有编号；
- 检查编号是否落在片段数量范围内；
- 检查是否有超出片段的推断。`,
	},
	{
		id: "teaching-style", name: "教研文风",
		description: "面向教师的书面表达与结构化输出规范",
		body: `# 教研文风

## 语气
使用面向教师的书面语，克制、具体，不堆砌形容词，不使用营销腔。

## 结构
- 结论先行：第一段直接给出判断；
- 层次清晰：使用二级标题与有序列表；
- 每条建议要可执行：写清「做什么、怎么做、做到什么程度」。

## 禁止
- 不使用「赋能」「抓手」「闭环」等空泛词；
- 不给与学生年龄不符的活动设计。`,
	},
	{
		id: "outline", name: "结构化输出",
		description: "先结论、后要点的输出骨架",
		body: `# 结构化输出

## 骨架
1. 一句话结论
2. 3–5 条要点（每条一行，带 [编号]）
3. 一句使用建议

## 约束
- 总长度不超过 300 字；
- 要点之间不重复；
- 不输出与问题无关的背景铺垫。`,
	},
}

var seedTemplates = []seedTemplate{
	{
		id: "tpl-kb-qa", name: "教研问答模板",
		description: "在本班材料范围内回答问题，逐条标注出处",
		system: `你是 CampusClaw 教研助手，服务于 {{class_name}}。
只依据用户消息中提供的本班材料片段作答；片段不足以回答时，直接说明「资料中未找到相关内容」。
回答中的每条事实性结论必须用 [编号] 标注出处，不得编造来源。`,
		user: `## 本班材料片段
{{context}}

## 用户问题
{{question}}`,
		vars: "class_name,context,question",
	},
	{
		id: "tpl-lesson-plan", name: "教案生成模板",
		description: "依据本班材料生成结构化教案",
		system: `你是 CampusClaw 备课助手，服务于 {{class_name}}。
只依据提供的本班材料片段设计教案，不要引入片段之外的知识。
输出结构固定为：教学目标 / 教学重难点 / 教学流程（表格） / 板书设计 / 分层作业。
每条设计意图后用 [编号] 标注依据。`,
		user: `## 课题
{{question}}

## 本班材料片段
{{context}}`,
		vars: "class_name,context,question",
	},
	{
		id: "tpl-summary", name: "材料摘要模板",
		description: "把本班材料压缩为可引用的要点",
		system: `你是 CampusClaw 材料摘要助手，服务于 {{class_name}}。
把提供的材料片段压缩为 3–5 条要点，每条要点后标注 [编号]。
只做摘要，不做评价，不补充片段之外的信息。`,
		user: `## 待摘要材料片段
{{context}}

## 摘要要求
{{question}}`,
		vars: "class_name,context,question",
	},
	{
		id: "tpl-analysis", name: "学情分析模板",
		description: "先调用工具统计，再给出学情分析",
		system: `你是 CampusClaw 学情分析助手，服务于 {{class_name}}。
需要统计类信息时先调用提供的工具，再基于工具结果与材料片段给出分析。
每条结论后用 [编号] 标注材料依据；工具结果单独标注为「工具结果」。`,
		user: `## 本班材料片段
{{context}}

## 分析需求
{{question}}`,
		vars: "class_name,context,question",
	},
}

var seedApps = []seedApp{
	{id: "kb-qa", name: "教研问答", description: "在本班材料范围内提问，逐条给出出处", icon: "💬",
		templateID: "tpl-kb-qa", kbEnabled: true, kbTopK: 4, skills: []string{"cite-check"}},
	{id: "lesson-plan", name: "教案生成", description: "依据本班材料生成可审阅的教案", icon: "📝",
		templateID: "tpl-lesson-plan", kbEnabled: true, kbTopK: 4, skills: []string{"cite-check", "teaching-style"}},
	{id: "summary", name: "材料摘要", description: "把长材料压缩为可引用的要点", icon: "📄",
		templateID: "tpl-summary", kbEnabled: true, kbTopK: 4, skills: []string{"outline"}},
	{id: "data-analysis", name: "学情分析", description: "调用 MCP 工具统计后给出分析", icon: "📊",
		templateID: "tpl-analysis", kbEnabled: true, kbTopK: 3, skills: []string{"cite-check"}, mcpServers: []string{"demo-tools"}},
}

// SeedPlatform 写入平台内置数据（幂等：先查后插，不覆盖用户改过的内容）。
func SeedPlatform(ctx context.Context, db *sql.DB) error {
	for _, t := range seedTemplates {
		if _, err := db.ExecContext(ctx, `
			INSERT IGNORE INTO prompt_templates (id, name, description, system_template, user_template, variables)
			VALUES (?, ?, ?, ?, ?, ?)`, t.id, t.name, t.description, t.system, t.user, t.vars); err != nil {
			return fmt.Errorf("写入提示词模板失败: %w", err)
		}
	}
	for _, s := range builtinSkills {
		if _, err := db.ExecContext(ctx, `
			INSERT IGNORE INTO skills (id, name, description, body, builtin, enabled, source_filename, size_chars)
			VALUES (?, ?, ?, ?, 1, 1, ?, ?)`, s.id, s.name, s.description, s.body, s.id+".md", len([]rune(s.body))); err != nil {
			return fmt.Errorf("写入内置技能失败: %w", err)
		}
	}
	// MCP 演示服务器：仓库内自带的 bin/mcp-demo，通过 stdio 通信。
	if _, err := db.ExecContext(ctx, `
		INSERT IGNORE INTO mcp_servers (id, name, command, args, builtin, enabled)
		VALUES ('demo-tools', '教研工具（MCP 演示）', 'bin/mcp-demo', '', 1, 1)`); err != nil {
		return fmt.Errorf("写入 MCP 服务器失败: %w", err)
	}
	for _, a := range seedApps {
		if _, err := db.ExecContext(ctx, `
			INSERT IGNORE INTO ai_apps (id, name, description, icon, template_id, model, temperature, max_tokens, kb_enabled, kb_top_k, sort_order)
			VALUES (?, ?, ?, ?, ?, 'deepseek-chat', 0.2, 900, ?, ?, ?)`,
			a.id, a.name, a.description, a.icon, a.templateID, a.kbEnabled, a.kbTopK, len(seedApps)); err != nil {
			return fmt.Errorf("写入 AI 应用失败: %w", err)
		}
		for _, sid := range a.skills {
			if _, err := db.ExecContext(ctx, `INSERT IGNORE INTO app_skills (app_id, skill_id) VALUES (?, ?)`, a.id, sid); err != nil {
				return fmt.Errorf("绑定技能失败: %w", err)
			}
		}
		for _, mid := range a.mcpServers {
			if _, err := db.ExecContext(ctx, `INSERT IGNORE INTO app_mcp_servers (app_id, server_id) VALUES (?, ?)`, a.id, mid); err != nil {
				return fmt.Errorf("绑定 MCP 失败: %w", err)
			}
		}
	}
	slog.Info("平台种子数据已就绪", "apps", len(seedApps), "templates", len(seedTemplates), "skills", len(builtinSkills))
	return nil
}
