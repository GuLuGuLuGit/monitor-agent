package collector

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

// SkillItem 与后端 SkillItem 一致
type SkillItem struct {
	SkillName    string     `json:"skill_name"`
	SkillVersion string     `json:"skill_version"`
	SkillPath    string     `json:"skill_path"`
	SkillType    string     `json:"skill_type"`
	Description  string     `json:"description"`
	Author       string     `json:"author"`
	Enabled      bool       `json:"enabled"`
	LastUsedAt   *time.Time `json:"last_used_at"`
}

// ScanSkills 扫描目录下的 Skills；目录不存在或为空则返回空列表
func ScanSkills(scanPaths []string) []SkillItem {
	var skills []SkillItem
	seen := make(map[string]bool)

	for _, root := range scanPaths {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			skillPath := filepath.Join(root, e.Name())
			if seen[skillPath] {
				continue
			}
			seen[skillPath] = true
			skill := parseSkill(skillPath, e.Name())
			skills = append(skills, skill)
		}
	}
	return skills
}

// parseSkill 从目录名和路径解析基础信息；SKILL.md 等元数据可在此扩展
func parseSkill(skillPath, name string) SkillItem {
	skill := SkillItem{
		SkillName:   name,
		SkillPath:   skillPath,
		SkillType:   "directory",
		SkillVersion: "",
		Enabled:     true,
	}
	// 可选：读取 SKILL.md 或 package.json 等获取 version/description/author
	// 此处仅做占位，后续可与 OpenClaw 约定格式后扩展
	return skill
}
