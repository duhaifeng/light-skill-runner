// Package loader 负责扫描 skill 目录并解析每个 SKILL.md。
package loader

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Skill 表示一个已加载的 skill。
type Skill struct {
	Name        string // frontmatter: name
	Description string // frontmatter: description
	Body        string // SKILL.md 正文（frontmatter 之后的部分）
	Dir         string // skill 所在目录的绝对路径
	Path        string // SKILL.md 的绝对路径
}

// Load 扫描 root 下的一级子目录，解析其中的 SKILL.md。
// 没有 SKILL.md 的子目录会被忽略。
func Load(root string) ([]Skill, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("读取 skills 目录失败: %w", err)
	}

	var skills []Skill
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(root, e.Name())
		path := filepath.Join(dir, "SKILL.md")
		if _, err := os.Stat(path); err != nil {
			continue // 没有 SKILL.md，跳过
		}
		s, err := parseFile(path)
		if err != nil {
			return nil, fmt.Errorf("解析 %s 失败: %w", path, err)
		}
		absDir, _ := filepath.Abs(dir)
		absPath, _ := filepath.Abs(path)
		s.Dir = absDir
		s.Path = absPath
		if s.Name == "" {
			s.Name = e.Name() // 缺省用目录名兜底
		}
		skills = append(skills, s)
	}
	return skills, nil
}

// parseFile 读取并解析单个 SKILL.md。
func parseFile(path string) (Skill, error) {
	f, err := os.Open(path)
	if err != nil {
		return Skill{}, err
	}
	defer f.Close()

	var (
		s          Skill
		inFM       bool
		fmDone     bool
		bodyLines  []string
		sc         = bufio.NewScanner(f)
	)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	for sc.Scan() {
		line := sc.Text()
		trimmed := strings.TrimSpace(line)

		// frontmatter 起止用 "---" 标记，且必须在文件最开头。
		if !fmDone && trimmed == "---" {
			if !inFM && len(bodyLines) == 0 {
				inFM = true
				continue
			}
			if inFM {
				inFM = false
				fmDone = true
				continue
			}
		}

		if inFM {
			key, val, ok := splitKeyValue(trimmed)
			if !ok {
				continue
			}
			switch key {
			case "name":
				s.Name = val
			case "description":
				s.Description = val
			}
			continue
		}

		bodyLines = append(bodyLines, line)
	}
	if err := sc.Err(); err != nil {
		return Skill{}, err
	}

	s.Body = strings.TrimSpace(strings.Join(bodyLines, "\n"))
	return s, nil
}

// splitKeyValue 解析 "key: value" 形式的一行，去除可选引号。
func splitKeyValue(line string) (key, val string, ok bool) {
	idx := strings.Index(line, ":")
	if idx < 0 {
		return "", "", false
	}
	key = strings.TrimSpace(line[:idx])
	val = strings.TrimSpace(line[idx+1:])
	val = strings.Trim(val, `"'`)
	if key == "" {
		return "", "", false
	}
	return key, val, true
}
