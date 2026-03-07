package collector

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScanSkills(t *testing.T) {
	// 创建临时目录结构
	tmpDir := t.TempDir()
	skillsDir := filepath.Join(tmpDir, "skills")
	if err := os.MkdirAll(skillsDir, 0755); err != nil {
		t.Fatalf("Failed to create skills dir: %v", err)
	}

	// 创建测试 skill 目录
	skill1 := filepath.Join(skillsDir, "skill1")
	skill2 := filepath.Join(skillsDir, "skill2")
	if err := os.MkdirAll(skill1, 0755); err != nil {
		t.Fatalf("Failed to create skill1: %v", err)
	}
	if err := os.MkdirAll(skill2, 0755); err != nil {
		t.Fatalf("Failed to create skill2: %v", err)
	}

	// 创建一个文件（应该被忽略）
	file := filepath.Join(skillsDir, "readme.txt")
	if err := os.WriteFile(file, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}

	// 扫描 skills
	skills := ScanSkills([]string{skillsDir})

	// 验证结果
	if len(skills) != 2 {
		t.Errorf("Expected 2 skills, got %d", len(skills))
	}

	// 验证 skill 信息
	for _, skill := range skills {
		if skill.SkillName == "" {
			t.Error("SkillName should not be empty")
		}
		if skill.SkillPath == "" {
			t.Error("SkillPath should not be empty")
		}
		if skill.SkillType != "directory" {
			t.Errorf("SkillType should be 'directory', got %s", skill.SkillType)
		}
		t.Logf("Skill: %+v", skill)
	}
}

func TestScanSkillsEmptyDir(t *testing.T) {
	// 测试空目录
	tmpDir := t.TempDir()
	skills := ScanSkills([]string{tmpDir})

	if len(skills) != 0 {
		t.Errorf("Expected 0 skills in empty dir, got %d", len(skills))
	}
}

func TestScanSkillsNonexistentDir(t *testing.T) {
	// 测试不存在的目录
	skills := ScanSkills([]string{"/nonexistent/path"})

	if len(skills) != 0 {
		t.Errorf("Expected 0 skills for nonexistent dir, got %d", len(skills))
	}
}

func TestScanSkillsMultiplePaths(t *testing.T) {
	// 创建多个目录
	tmpDir1 := t.TempDir()
	tmpDir2 := t.TempDir()

	skill1 := filepath.Join(tmpDir1, "skill1")
	skill2 := filepath.Join(tmpDir2, "skill2")
	if err := os.MkdirAll(skill1, 0755); err != nil {
		t.Fatalf("Failed to create skill1: %v", err)
	}
	if err := os.MkdirAll(skill2, 0755); err != nil {
		t.Fatalf("Failed to create skill2: %v", err)
	}

	// 扫描多个路径
	skills := ScanSkills([]string{tmpDir1, tmpDir2})

	if len(skills) != 2 {
		t.Errorf("Expected 2 skills from multiple paths, got %d", len(skills))
	}
}
