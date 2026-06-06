package trace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Summary 是 trace 的精简信息，用于历史列表。
type Summary struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	Input     string `json:"input"`
	StartTime string `json:"start_time"`
}

// List 读取目录下所有 trace 文件并返回摘要（按开始时间倒序）。
func List(dir string) ([]Summary, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []Summary{}, nil
		}
		return nil, err
	}
	var out []Summary
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		t, err := Read(dir, strings.TrimSuffix(e.Name(), ".json"))
		if err != nil {
			continue
		}
		out = append(out, Summary{
			ID:        t.ID,
			Name:      t.Name,
			Status:    t.Status,
			Input:     t.Input,
			StartTime: t.StartTime.Format("2006-01-02 15:04:05"),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartTime > out[j].StartTime })
	return out, nil
}

// Read 读取单个 trace 文件。
func Read(dir, id string) (*Trace, error) {
	data, err := os.ReadFile(filepath.Join(dir, id+".json"))
	if err != nil {
		return nil, err
	}
	var t Trace
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, err
	}
	return &t, nil
}
