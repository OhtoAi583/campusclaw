// Package platform 提供 AI 应用平台的配置读取：应用、提示词模板、技能、MCP 服务器。
package platform

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// ErrNotFound 表示配置不存在。
var ErrNotFound = errors.New("not found")

// Template 是一个提示词模板（system + user，含 {{变量}}）。
type Template struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Description    string `json:"description"`
	SystemTemplate string `json:"system_template"`
	UserTemplate   string `json:"user_template"`
	Variables      string `json:"variables"`
}

// Skill 是一个技能（SKILL.md 的完整正文）。
type Skill struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Description    string `json:"description"`
	Body           string `json:"body"`
	Builtin        bool   `json:"builtin"`
	Enabled        bool   `json:"enabled"`
	SourceFilename string `json:"source_filename"`
	SizeChars      int    `json:"size_chars"`
}

// MCPServer 是一个 MCP 服务器配置（stdio）。
type MCPServer struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Command string `json:"command"`
	Args    string `json:"args"`
	Enabled bool   `json:"enabled"`
}

// App 是一个 AI 应用：模板 + 知识库绑定 + 技能 + MCP 工具。
type App struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Icon        string      `json:"icon"`
	TemplateID  string      `json:"template_id"`
	Model       string      `json:"model"`
	Temperature float64     `json:"temperature"`
	MaxTokens   int         `json:"max_tokens"`
	KBEnabled   bool        `json:"kb_enabled"`
	KBTopK      int         `json:"kb_top_k"`
	Template    Template    `json:"template"`
	Skills      []Skill     `json:"skills"`
	MCPServers  []MCPServer `json:"mcp_servers"`
}

// Store 读取平台配置。
type Store struct{ DB *sql.DB }

// NewStore 构造配置存储。
func NewStore(db *sql.DB) *Store { return &Store{DB: db} }

// ListApps 返回已启用的应用（含模板、技能、MCP 绑定）。
func (s *Store) ListApps(ctx context.Context) ([]App, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, name, description, icon, template_id, model, temperature, max_tokens, kb_enabled, kb_top_k
		FROM ai_apps WHERE enabled = 1 ORDER BY sort_order, id`)
	if err != nil {
		return nil, err
	}
	var apps []App
	for rows.Next() {
		var a App
		if err := rows.Scan(&a.ID, &a.Name, &a.Description, &a.Icon, &a.TemplateID,
			&a.Model, &a.Temperature, &a.MaxTokens, &a.KBEnabled, &a.KBTopK); err != nil {
			rows.Close()
			return nil, err
		}
		apps = append(apps, a)
	}
	rows.Close()
	for i := range apps {
		if err := s.hydrate(ctx, &apps[i]); err != nil {
			return nil, err
		}
	}
	return apps, nil
}

// GetApp 返回单个应用。
func (s *Store) GetApp(ctx context.Context, id string) (App, error) {
	var a App
	err := s.DB.QueryRowContext(ctx, `
		SELECT id, name, description, icon, template_id, model, temperature, max_tokens, kb_enabled, kb_top_k
		FROM ai_apps WHERE id = ? AND enabled = 1`, id).
		Scan(&a.ID, &a.Name, &a.Description, &a.Icon, &a.TemplateID,
			&a.Model, &a.Temperature, &a.MaxTokens, &a.KBEnabled, &a.KBTopK)
	if errors.Is(err, sql.ErrNoRows) {
		return App{}, ErrNotFound
	}
	if err != nil {
		return App{}, err
	}
	if err := s.hydrate(ctx, &a); err != nil {
		return App{}, err
	}
	return a, nil
}

func (s *Store) hydrate(ctx context.Context, a *App) error {
	if err := s.DB.QueryRowContext(ctx, `
		SELECT id, name, description, system_template, user_template, variables
		FROM prompt_templates WHERE id = ?`, a.TemplateID).
		Scan(&a.Template.ID, &a.Template.Name, &a.Template.Description,
			&a.Template.SystemTemplate, &a.Template.UserTemplate, &a.Template.Variables); err != nil {
		return fmt.Errorf("读取模板失败: %w", err)
	}

	rows, err := s.DB.QueryContext(ctx, `
		SELECT s.id, s.name, s.description, s.body, s.builtin, s.enabled, s.source_filename, s.size_chars
		FROM skills s JOIN app_skills x ON x.skill_id = s.id
		WHERE x.app_id = ? AND s.enabled = 1 ORDER BY s.id`, a.ID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var sk Skill
		if err := rows.Scan(&sk.ID, &sk.Name, &sk.Description, &sk.Body, &sk.Builtin,
			&sk.Enabled, &sk.SourceFilename, &sk.SizeChars); err != nil {
			rows.Close()
			return err
		}
		a.Skills = append(a.Skills, sk)
	}
	rows.Close()

	rows, err = s.DB.QueryContext(ctx, `
		SELECT m.id, m.name, m.command, m.args, m.enabled
		FROM mcp_servers m JOIN app_mcp_servers x ON x.server_id = m.id
		WHERE x.app_id = ? AND m.enabled = 1 ORDER BY m.id`, a.ID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var m MCPServer
		if err := rows.Scan(&m.ID, &m.Name, &m.Command, &m.Args, &m.Enabled); err != nil {
			rows.Close()
			return err
		}
		a.MCPServers = append(a.MCPServers, m)
	}
	rows.Close()
	return rows.Err()
}

// ListTemplates 返回全部提示词模板。
func (s *Store) ListTemplates(ctx context.Context) ([]Template, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, name, description, system_template, user_template, variables
		FROM prompt_templates ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Template
	for rows.Next() {
		var t Template
		if err := rows.Scan(&t.ID, &t.Name, &t.Description, &t.SystemTemplate, &t.UserTemplate, &t.Variables); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// ListSkills 返回全部技能（含内置与上传的）。
func (s *Store) ListSkills(ctx context.Context) ([]Skill, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, name, description, body, builtin, enabled, source_filename, size_chars
		FROM skills ORDER BY builtin DESC, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Skill
	for rows.Next() {
		var sk Skill
		if err := rows.Scan(&sk.ID, &sk.Name, &sk.Description, &sk.Body, &sk.Builtin,
			&sk.Enabled, &sk.SourceFilename, &sk.SizeChars); err != nil {
			return nil, err
		}
		out = append(out, sk)
	}
	return out, rows.Err()
}

// UpsertSkill 保存上传的技能。
func (s *Store) UpsertSkill(ctx context.Context, sk Skill) error {
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO skills (id, name, description, body, builtin, enabled, source_filename, size_chars)
		VALUES (?, ?, ?, ?, 0, 1, ?, ?)
		ON DUPLICATE KEY UPDATE name=VALUES(name), description=VALUES(description), body=VALUES(body),
			source_filename=VALUES(source_filename), size_chars=VALUES(size_chars)`,
		sk.ID, sk.Name, sk.Description, sk.Body, sk.SourceFilename, sk.SizeChars)
	return err
}

// ListMCPServers 返回全部 MCP 服务器。
func (s *Store) ListMCPServers(ctx context.Context) ([]MCPServer, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, name, command, args, enabled FROM mcp_servers ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MCPServer
	for rows.Next() {
		var m MCPServer
		if err := rows.Scan(&m.ID, &m.Name, &m.Command, &m.Args, &m.Enabled); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
