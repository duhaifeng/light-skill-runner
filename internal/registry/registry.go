// Package registry 在内存中维护已加载的 skill。
package registry

import "github.com/duhaifeng/light-skill-runner/internal/loader"

// Registry 是 skill 的内存注册表。
type Registry struct {
	order  []string
	skills map[string]loader.Skill
}

// New 根据已加载的 skill 列表构建注册表。
func New(skills []loader.Skill) *Registry {
	r := &Registry{skills: make(map[string]loader.Skill, len(skills))}
	for _, s := range skills {
		if _, exists := r.skills[s.Name]; !exists {
			r.order = append(r.order, s.Name)
		}
		r.skills[s.Name] = s
	}
	return r
}

// List 按加载顺序返回所有 skill。
func (r *Registry) List() []loader.Skill {
	out := make([]loader.Skill, 0, len(r.order))
	for _, name := range r.order {
		out = append(out, r.skills[name])
	}
	return out
}

// Get 按名称查找 skill。
func (r *Registry) Get(name string) (loader.Skill, bool) {
	s, ok := r.skills[name]
	return s, ok
}

// Len 返回 skill 数量。
func (r *Registry) Len() int { return len(r.order) }
