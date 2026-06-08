package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Character struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Age         string `json:"age,omitempty"`
	Appearance  string `json:"appearance,omitempty"`
	Personality string `json:"personality,omitempty"`
	Background  string `json:"background,omitempty"`
	Motivation  string `json:"motivation,omitempty"`
	Abilities   string `json:"abilities,omitempty"`
	Notes       string `json:"notes,omitempty"`
}

type WorldviewEntry struct {
	ID          string `json:"id"`
	Category    string `json:"category"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Tags        string `json:"tags,omitempty"`
}

type Organization struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Type        string   `json:"type"`
	Description string   `json:"description"`
	Members     []string `json:"members,omitempty"`
}

type Relation struct {
	ID         string `json:"id"`
	SourceID   string `json:"source_id"`
	SourceType string `json:"source_type"`
	TargetID   string `json:"target_id"`
	TargetType string `json:"target_type"`
	Label      string `json:"label"`
}

type ProjectSettings struct {
	Characters    []Character      `json:"characters"`
	Worldview     []WorldviewEntry `json:"worldview"`
	Organizations []Organization   `json:"organizations"`
	Relations     []Relation       `json:"relations"`
}

func LoadProjectSettings(path string) (*ProjectSettings, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &ProjectSettings{}, nil
		}
		return nil, fmt.Errorf("读取设定文件失败: %w", err)
	}

	var ps ProjectSettings
	if err := json.Unmarshal(data, &ps); err != nil {
		return nil, fmt.Errorf("解析设定文件失败: %w", err)
	}

	return &ps, nil
}

func SaveProjectSettings(path string, ps *ProjectSettings) error {
	data, err := json.MarshalIndent(ps, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化设定失败: %w", err)
	}
	return writeFileAtomic(path, data)
}

func nextID(prefix string, existingIDs []string) string {
	maxNum := 0
	for _, id := range existingIDs {
		if strings.HasPrefix(id, prefix+"_") {
			numStr := strings.TrimPrefix(id, prefix+"_")
			if n, err := strconv.Atoi(numStr); err == nil && n > maxNum {
				maxNum = n
			}
		}
	}
	return fmt.Sprintf("%s_%d", prefix, maxNum+1)
}

func (ps *ProjectSettings) allIDs() []string {
	var ids []string
	for _, c := range ps.Characters {
		ids = append(ids, c.ID)
	}
	for _, w := range ps.Worldview {
		ids = append(ids, w.ID)
	}
	for _, o := range ps.Organizations {
		ids = append(ids, o.ID)
	}
	for _, r := range ps.Relations {
		ids = append(ids, r.ID)
	}
	return ids
}

func (ps *ProjectSettings) nextCharacterID() string {
	return nextID("c", ps.allIDs())
}

func (ps *ProjectSettings) nextWorldviewID() string {
	return nextID("w", ps.allIDs())
}

func (ps *ProjectSettings) nextOrganizationID() string {
	return nextID("o", ps.allIDs())
}

func (ps *ProjectSettings) nextRelationID() string {
	return nextID("r", ps.allIDs())
}

func buildCharacterContext(settings *ProjectSettings, chapterOutline string) string {
	if settings == nil || len(settings.Characters) == 0 {
		return ""
	}

	var relevant []Character
	for _, c := range settings.Characters {
		if strings.Contains(chapterOutline, c.Name) {
			relevant = append(relevant, c)
		}
	}

	if len(relevant) == 0 {
		relevant = settings.Characters
	}

	var sb strings.Builder
	for _, c := range relevant {
		sb.WriteString(fmt.Sprintf("【%s】", c.Name))
		if c.Age != "" {
			sb.WriteString(fmt.Sprintf(" 年龄:%s", c.Age))
		}
		sb.WriteString("\n")
		if c.Appearance != "" {
			sb.WriteString(fmt.Sprintf("  外貌: %s\n", c.Appearance))
		}
		if c.Personality != "" {
			sb.WriteString(fmt.Sprintf("  性格: %s\n", c.Personality))
		}
		if c.Background != "" {
			sb.WriteString(fmt.Sprintf("  背景: %s\n", c.Background))
		}
		if c.Motivation != "" {
			sb.WriteString(fmt.Sprintf("  动机: %s\n", c.Motivation))
		}
		if c.Abilities != "" {
			sb.WriteString(fmt.Sprintf("  能力: %s\n", c.Abilities))
		}
		if c.Notes != "" {
			sb.WriteString(fmt.Sprintf("  备注: %s\n", c.Notes))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

func buildWorldviewContext(settings *ProjectSettings, chapterOutline string) string {
	if settings == nil {
		return ""
	}

	var sb strings.Builder

	if len(settings.Worldview) > 0 {
		for _, w := range settings.Worldview {
			if strings.Contains(chapterOutline, w.Name) || strings.Contains(chapterOutline, w.Category) {
				sb.WriteString(fmt.Sprintf("【%s】(%s)\n  %s\n\n", w.Name, w.Category, w.Description))
			}
		}
		if sb.Len() == 0 {
			for _, w := range settings.Worldview {
				sb.WriteString(fmt.Sprintf("【%s】(%s)\n  %s\n\n", w.Name, w.Category, w.Description))
			}
		}
	}

	if len(settings.Organizations) > 0 {
		for _, o := range settings.Organizations {
			if strings.Contains(chapterOutline, o.Name) {
				sb.WriteString(fmt.Sprintf("【组织:%s】(%s)\n  %s\n", o.Name, o.Type, o.Description))
				if len(o.Members) > 0 {
					sb.WriteString(fmt.Sprintf("  成员IDs: %s\n", strings.Join(o.Members, ", ")))
				}
				sb.WriteString("\n")
			}
		}
	}

	return sb.String()
}
