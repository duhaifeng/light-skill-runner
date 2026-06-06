// Package store provides the local SQLite persistence layer.
package store

import (
	"database/sql"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	schemadb "github.com/duhaifeng/light-skill-runner/scripts/db"

	_ "modernc.org/sqlite"
)

// ModelConfig is a persisted LLM configuration.
type ModelConfig struct {
	ID                 int64  `json:"id"`
	Name               string `json:"name"`
	Provider           string `json:"provider"`
	BaseURL            string `json:"base_url"`
	Model              string `json:"model"`
	APIKey             string `json:"api_key,omitempty"`
	ForceToolEmulation bool   `json:"force_tool_emulation"`
	IsDefault          bool   `json:"is_default"`
}

// SkillSetting is the UI-managed metadata for a filesystem skill.
type SkillSetting struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Path        string `json:"path"`
	Enabled     bool   `json:"enabled"`
	Tags        string `json:"tags"`
	SortOrder   int    `json:"sort_order"`
}

// SkillSource is the minimal shape needed to sync scanned skills.
type SkillSource struct {
	Name        string
	Description string
	Path        string
}

// Store wraps a SQLite database.
type Store struct {
	db *sql.DB
}

// Open opens the local database and applies embedded migrations.
func Open(path string) (*Store, error) {
	if strings.TrimSpace(path) == "" {
		path = "./data/light-skill-runner.db"
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("创建数据库目录失败: %w", err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("打开数据库失败: %w", err)
	}
	db.SetMaxOpenConns(1)

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// Close closes the underlying database.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) migrate() error {
	if _, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version TEXT PRIMARY KEY,
			applied_at TEXT NOT NULL DEFAULT (datetime('now'))
		)`); err != nil {
		return fmt.Errorf("初始化迁移表失败: %w", err)
	}
	files, err := fs.Glob(schemadb.Migrations, "*.sql")
	if err != nil {
		return fmt.Errorf("读取迁移脚本失败: %w", err)
	}
	sort.Strings(files)
	for _, file := range files {
		var applied int
		if err := s.db.QueryRow("SELECT COUNT(1) FROM schema_migrations WHERE version = ?", file).Scan(&applied); err != nil {
			return fmt.Errorf("检查迁移状态失败: %w", err)
		}
		if applied > 0 {
			continue
		}
		sqlText, err := schemadb.Migrations.ReadFile(file)
		if err != nil {
			return fmt.Errorf("读取迁移 %s 失败: %w", file, err)
		}
		tx, err := s.db.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(string(sqlText)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("执行迁移 %s 失败: %w", file, err)
		}
		if _, err := tx.Exec("INSERT INTO schema_migrations(version) VALUES (?)", file); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("记录迁移 %s 失败: %w", file, err)
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

// SeedModel inserts the config model as default when the model table is empty.
func (s *Store) SeedModel(m ModelConfig) error {
	var count int
	if err := s.db.QueryRow("SELECT COUNT(1) FROM model_configs").Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	if strings.TrimSpace(m.Name) == "" {
		m.Name = defaultModelName(m)
	}
	m.IsDefault = true
	return s.CreateModel(m)
}

// ListModels returns all model configs.
func (s *Store) ListModels() ([]ModelConfig, error) {
	rows, err := s.db.Query(`
		SELECT id, name, provider, base_url, model, api_key, force_tool_emulation, is_default
		FROM model_configs
		ORDER BY is_default DESC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ModelConfig
	for rows.Next() {
		var m ModelConfig
		var force, def int
		if err := rows.Scan(&m.ID, &m.Name, &m.Provider, &m.BaseURL, &m.Model, &m.APIKey, &force, &def); err != nil {
			return nil, err
		}
		m.ForceToolEmulation = force != 0
		m.IsDefault = def != 0
		out = append(out, m)
	}
	return out, rows.Err()
}

// DefaultModel returns the configured default model.
func (s *Store) DefaultModel() (ModelConfig, bool, error) {
	var m ModelConfig
	var force, def int
	err := s.db.QueryRow(`
		SELECT id, name, provider, base_url, model, api_key, force_tool_emulation, is_default
		FROM model_configs
		WHERE is_default = 1
		LIMIT 1`).Scan(&m.ID, &m.Name, &m.Provider, &m.BaseURL, &m.Model, &m.APIKey, &force, &def)
	if err == sql.ErrNoRows {
		return ModelConfig{}, false, nil
	}
	if err != nil {
		return ModelConfig{}, false, err
	}
	m.ForceToolEmulation = force != 0
	m.IsDefault = def != 0
	return m, true, nil
}

// CreateModel inserts a new model config.
func (s *Store) CreateModel(m ModelConfig) error {
	if err := validateModel(m); err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	if m.IsDefault {
		if _, err := tx.Exec("UPDATE model_configs SET is_default = 0, updated_at = datetime('now')"); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	_, err = tx.Exec(`
		INSERT INTO model_configs(name, provider, base_url, model, api_key, force_tool_emulation, is_default)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		m.Name, m.Provider, m.BaseURL, m.Model, m.APIKey, boolInt(m.ForceToolEmulation), boolInt(m.IsDefault))
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// UpdateModel updates a model config by id.
func (s *Store) UpdateModel(id int64, m ModelConfig) error {
	if id <= 0 {
		return fmt.Errorf("无效模型 id")
	}
	if err := validateModel(m); err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	if m.IsDefault {
		if _, err := tx.Exec("UPDATE model_configs SET is_default = 0, updated_at = datetime('now')"); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	res, err := tx.Exec(`
		UPDATE model_configs
		SET name = ?, provider = ?, base_url = ?, model = ?, api_key = ?,
			force_tool_emulation = ?, is_default = ?, updated_at = datetime('now')
		WHERE id = ?`,
		m.Name, m.Provider, m.BaseURL, m.Model, m.APIKey, boolInt(m.ForceToolEmulation), boolInt(m.IsDefault), id)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		_ = tx.Rollback()
		return fmt.Errorf("模型不存在")
	}
	return tx.Commit()
}

// SetDefaultModel marks one model as default.
func (s *Store) SetDefaultModel(id int64) error {
	var exists int
	if err := s.db.QueryRow("SELECT COUNT(1) FROM model_configs WHERE id = ?", id).Scan(&exists); err != nil {
		return err
	}
	if exists == 0 {
		return fmt.Errorf("模型不存在")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	if _, err := tx.Exec("UPDATE model_configs SET is_default = CASE WHEN id = ? THEN 1 ELSE 0 END, updated_at = datetime('now')", id); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// SyncSkills upserts scanned skills while preserving UI-managed fields.
func (s *Store) SyncSkills(skills []SkillSource) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	stmt, err := tx.Prepare(`
		INSERT INTO skill_settings(name, description, path, enabled)
		VALUES (?, ?, ?, 1)
		ON CONFLICT(name) DO UPDATE SET
			description = excluded.description,
			path = excluded.path,
			updated_at = datetime('now')`)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	defer stmt.Close()

	for _, sk := range skills {
		if strings.TrimSpace(sk.Name) == "" {
			continue
		}
		if _, err := stmt.Exec(sk.Name, sk.Description, sk.Path); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// ListSkillSettings returns all skill settings.
func (s *Store) ListSkillSettings() ([]SkillSetting, error) {
	rows, err := s.db.Query(`
		SELECT name, description, path, enabled, tags, sort_order
		FROM skill_settings
		ORDER BY sort_order ASC, name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []SkillSetting
	for rows.Next() {
		var sk SkillSetting
		var enabled int
		if err := rows.Scan(&sk.Name, &sk.Description, &sk.Path, &enabled, &sk.Tags, &sk.SortOrder); err != nil {
			return nil, err
		}
		sk.Enabled = enabled != 0
		out = append(out, sk)
	}
	return out, rows.Err()
}

// EnabledSkillNames returns a set of enabled skill names.
func (s *Store) EnabledSkillNames() (map[string]bool, error) {
	rows, err := s.db.Query("SELECT name FROM skill_settings WHERE enabled = 1")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		out[name] = true
	}
	return out, rows.Err()
}

// UpdateSkillSetting updates UI-managed skill metadata.
func (s *Store) UpdateSkillSetting(name string, enabled bool, tags string, sortOrder int) error {
	res, err := s.db.Exec(`
		UPDATE skill_settings
		SET enabled = ?, tags = ?, sort_order = ?, updated_at = datetime('now')
		WHERE name = ?`,
		boolInt(enabled), tags, sortOrder, name)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("skill 不存在")
	}
	return nil
}

func validateModel(m ModelConfig) error {
	if strings.TrimSpace(m.Name) == "" {
		return fmt.Errorf("模型名称不能为空")
	}
	if strings.TrimSpace(m.Provider) == "" {
		return fmt.Errorf("provider 不能为空")
	}
	return nil
}

func defaultModelName(m ModelConfig) string {
	if m.Provider != "" && m.Model != "" {
		return m.Provider + " / " + m.Model
	}
	if m.Provider != "" {
		return m.Provider
	}
	return "default"
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
