package main

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

//go:embed embeds/skills
var builtinSkillFiles embed.FS

type Skill struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Category    string `json:"category"`
	Lang        string `json:"lang,omitempty"` // "zh", "en", or "" (language-agnostic)
	Content     string `json:"content"`
	Enabled     bool   `json:"enabled"`
	Source      string `json:"source"`
}

type SkillConfig struct {
	EnabledSkills map[string]bool `json:"enabled_skills"`
}

func (sc *SkillConfig) applyDefaults() {
	if sc.EnabledSkills == nil {
		sc.EnabledSkills = make(map[string]bool)
	}
}

func LoadBuiltinSkills() []Skill {
	var skills []Skill

	entries, err := builtinSkillFiles.ReadDir("embeds/skills")
	if err != nil {
		fmt.Printf(" [警告] 读取内置技能目录失败: %v\n", err)
		return skills
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		data, err := builtinSkillFiles.ReadFile("embeds/skills/" + entry.Name())
		if err != nil {
			fmt.Printf(" [警告] 读取内置技能文件 %s 失败: %v\n", entry.Name(), err)
			continue
		}

		skill, err := parseSkillFile(string(data), "builtin")
		if err != nil {
			fmt.Printf(" [警告] 解析内置技能文件 %s 失败: %v\n", entry.Name(), err)
			continue
		}

		skills = append(skills, skill)
	}

	return skills
}

func LoadProjectSkills(dir string) []Skill {
	skillsDir := filepath.Join(dir, "skills")
	if _, err := os.Stat(skillsDir); os.IsNotExist(err) {
		return nil
	}

	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		return nil
	}

	var skills []Skill
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		data, err := os.ReadFile(filepath.Join(skillsDir, entry.Name()))
		if err != nil {
			continue
		}

		skill, err := parseSkillFile(string(data), "project")
		if err != nil {
			continue
		}

		skills = append(skills, skill)
	}

	return skills
}

func parseSkillFile(content string, source string) (Skill, error) {
	skill := Skill{Source: source}

	parts := strings.SplitN(content, "---", 3)
	if len(parts) < 3 {
		return skill, fmt.Errorf("invalid skill file format: missing frontmatter")
	}

	frontmatter := strings.TrimSpace(parts[1])
	body := strings.TrimSpace(parts[2])

	for _, line := range strings.Split(frontmatter, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		kv := strings.SplitN(line, ":", 2)
		if len(kv) != 2 {
			continue
		}
		key := strings.TrimSpace(kv[0])
		value := strings.TrimSpace(kv[1])

		switch key {
		case "id":
			skill.ID = value
		case "name":
			skill.Name = value
		case "description":
			skill.Description = value
		case "category":
			skill.Category = value
		case "lang":
			skill.Lang = NormalizeLanguage(value)
		case "source":
			if source == "" {
				skill.Source = value
			}
		}
	}

	skill.Content = body

	if skill.ID == "" {
		return skill, fmt.Errorf("skill missing id")
	}

	return skill, nil
}

func MergeSkills(builtin, project []Skill) []Skill {
	result := make([]Skill, 0, len(builtin)+len(project))
	result = append(result, builtin...)
	result = append(result, project...)
	return result
}

func LoadAllSkills(cfg *Config, projectDir string) []Skill {
	builtin := LoadBuiltinSkills()
	project := LoadProjectSkills(projectDir)
	merged := MergeSkills(builtin, project)
	if cfg == nil {
		return merged
	}
	return FilterSkillsByLang(merged, cfg.Language)
}

// FilterSkillsByLang returns skills matching the project language.
// Skills with empty `lang` are language-agnostic and always returned.
func FilterSkillsByLang(skills []Skill, projectLang string) []Skill {
	projectLang = NormalizeLanguage(projectLang)
	out := make([]Skill, 0, len(skills))
	for _, s := range skills {
		if s.Lang == "" || s.Lang == projectLang {
			out = append(out, s)
		}
	}
	return out
}

func GetEnabledSkills(skills []Skill, sc *SkillConfig) []Skill {
	if sc == nil || sc.EnabledSkills == nil {
		return nil
	}

	var enabled []Skill
	for _, s := range skills {
		if sc.EnabledSkills[s.ID] {
			enabled = append(enabled, s)
		}
	}
	return enabled
}

func GetEnabledSkillsByCategory(skills []Skill, sc *SkillConfig, category string) []Skill {
	if sc == nil || sc.EnabledSkills == nil {
		return nil
	}

	var enabled []Skill
	for _, s := range skills {
		if sc.EnabledSkills[s.ID] && s.Category == category {
			enabled = append(enabled, s)
		}
	}
	return enabled
}

func FormatSkillsContent(skills []Skill) string {
	if len(skills) == 0 {
		return ""
	}

	var sb strings.Builder
	// Detect language from skill set: if any skill is explicitly EN, use EN header.
	en := false
	for _, s := range skills {
		if s.Lang == LangEN {
			en = true
			break
		}
	}
	if en {
		sb.WriteString("Strictly follow the skill rules below while writing:\n\n")
	} else {
		sb.WriteString("以下技能规则在创作时必须严格遵守：\n\n")
	}
	for _, s := range skills {
		sb.WriteString(fmt.Sprintf("## %s\n\n%s\n\n", s.Name, s.Content))
	}
	return sb.String()
}
