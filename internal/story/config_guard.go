package story

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"showmethestory/internal/config"
	"showmethestory/internal/fsutil"
	"showmethestory/internal/i18n"
	"showmethestory/internal/sse"
	"strings"
)

var protectedStoryFields = []string{"type", "title", "writing_style", "writing_pov", "story_synopsis"}

type ConfigFieldChange struct {
	Field    string `json:"field"`
	Current  string `json:"current"`
	Proposed string `json:"proposed"`
	Source   string `json:"source"`
	Reason   string `json:"reason,omitempty"`
}

type PendingConfigChanges struct {
	Changes []ConfigFieldChange `json:"changes"`
}

func PendingConfigChangesPath(progressPath string) string {
	return filepath.Join(filepath.Dir(progressPath), "pending_config_changes.json")
}

func storyFieldValue(story config.StoryConfig, field string) string {
	switch field {
	case "type":
		return story.Type
	case "title":
		return story.Title
	case "writing_style":
		return story.WritingStyle
	case "writing_pov":
		return story.WritingPOV
	case "story_synopsis":
		return story.StorySynopsis
	default:
		return ""
	}
}

func setStoryFieldValue(story *config.StoryConfig, field, value string) {
	switch field {
	case "type":
		story.Type = value
	case "title":
		story.Title = value
	case "writing_style":
		story.WritingStyle = value
	case "writing_pov":
		story.WritingPOV = value
	case "story_synopsis":
		story.StorySynopsis = value
	}
}

func isUserFilledStoryField(story config.StoryConfig, field string) bool {
	return strings.TrimSpace(storyFieldValue(story, field)) != ""
}

func storyFieldsEqual(a, b string) bool {
	return strings.TrimSpace(a) == strings.TrimSpace(b)
}

func storyConfigFromOutline(resp OutlineResponse, current config.StoryConfig) config.StoryConfig {
	proposed := current
	if resp.Title != "" {
		proposed.Title = resp.Title
	}
	if resp.StorySynopsis != "" {
		proposed.StorySynopsis = resp.StorySynopsis
	}
	return proposed
}

func storyConfigFromReconciliation(result ReconciliationResult, base config.StoryConfig) config.StoryConfig {
	adjusted := base
	if result.Type != "" {
		adjusted.Type = result.Type
	}
	if result.WritingStyle != "" {
		adjusted.WritingStyle = result.WritingStyle
	}
	if result.WritingPOV != "" {
		adjusted.WritingPOV = result.WritingPOV
	}
	if result.StorySynopsis != "" {
		adjusted.StorySynopsis = result.StorySynopsis
	}
	return adjusted
}

func CollectStoryConfigConflicts(current, proposed config.StoryConfig, source, reason string) []ConfigFieldChange {
	var conflicts []ConfigFieldChange
	for _, field := range protectedStoryFields {
		prop := storyFieldValue(proposed, field)
		if prop == "" {
			continue
		}
		cur := storyFieldValue(current, field)
		if isUserFilledStoryField(current, field) && !storyFieldsEqual(cur, prop) {
			conflicts = append(conflicts, ConfigFieldChange{
				Field:    field,
				Current:  cur,
				Proposed: prop,
				Source:   source,
				Reason:   reason,
			})
		}
	}
	return conflicts
}

func applyStoryConfigMerge(current, proposed config.StoryConfig, allowFields map[string]bool) config.StoryConfig {
	merged := current
	for _, field := range protectedStoryFields {
		prop := storyFieldValue(proposed, field)
		if prop == "" {
			continue
		}
		if allowFields != nil && allowFields[field] {
			setStoryFieldValue(&merged, field, prop)
			continue
		}
		if !isUserFilledStoryField(current, field) {
			setStoryFieldValue(&merged, field, prop)
		}
	}
	return merged
}

func mergePending(existing, incoming []ConfigFieldChange) []ConfigFieldChange {
	byField := make(map[string]ConfigFieldChange, len(existing)+len(incoming))
	for _, c := range existing {
		byField[c.Field] = c
	}
	for _, c := range incoming {
		byField[c.Field] = c
	}
	out := make([]ConfigFieldChange, 0, len(byField))
	for _, field := range protectedStoryFields {
		if c, ok := byField[field]; ok {
			out = append(out, c)
			delete(byField, field)
		}
	}
	for _, c := range byField {
		out = append(out, c)
	}
	return out
}

func LoadPendingConfigChanges(path string) (*PendingConfigChanges, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &PendingConfigChanges{}, nil
		}
		return nil, err
	}
	var pending PendingConfigChanges
	if err := json.Unmarshal(data, &pending); err != nil {
		return nil, err
	}
	if pending.Changes == nil {
		pending.Changes = []ConfigFieldChange{}
	}
	return &pending, nil
}

func SavePendingConfigChanges(path string, pending *PendingConfigChanges) error {
	if pending == nil || len(pending.Changes) == 0 {
		if err := fsutil.Delete(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	data, err := json.MarshalIndent(pending, "", "  ")
	if err != nil {
		return err
	}
	return fsutil.WriteFileAtomic(path, data)
}

func appendPendingChanges(pendingPath string, incoming []ConfigFieldChange, logger *sse.LogBroadcaster) error {
	if len(incoming) == 0 {
		return nil
	}
	existing, err := LoadPendingConfigChanges(pendingPath)
	if err != nil {
		return err
	}
	existing.Changes = mergePending(existing.Changes, incoming)
	if err := SavePendingConfigChanges(pendingPath, existing); err != nil {
		return err
	}
	if logger != nil {
		logger.ConfigChangeProposal(existing.Changes)
	}
	return nil
}

func RemovePendingFields(pendingPath string, fields ...string) error {
	pending, err := LoadPendingConfigChanges(pendingPath)
	if err != nil {
		return err
	}
	if len(pending.Changes) == 0 {
		return nil
	}
	remove := make(map[string]bool, len(fields))
	for _, f := range fields {
		remove[f] = true
	}
	kept := make([]ConfigFieldChange, 0, len(pending.Changes))
	for _, c := range pending.Changes {
		if !remove[c.Field] {
			kept = append(kept, c)
		}
	}
	pending.Changes = kept
	return SavePendingConfigChanges(pendingPath, pending)
}

func SyncProgressMetaFromStory(state *Progress, story config.StoryConfig) {
	if story.Title != "" {
		state.Title = story.Title
	}
	if story.StorySynopsis != "" {
		state.StorySynopsis = story.StorySynopsis
	}
}

func applyOutlineMetaWithGuard(cfg *config.Config, state *Progress, resp OutlineResponse, source, pendingPath, cfgPath string, logger *sse.LogBroadcaster) error {
	proposed := storyConfigFromOutline(resp, cfg.Story)
	conflicts := CollectStoryConfigConflicts(cfg.Story, proposed, source, "")

	cfg.Story = applyStoryConfigMerge(cfg.Story, proposed, nil)
	SyncProgressMetaFromStory(state, cfg.Story)
	if resp.CorePrompt != "" {
		state.CorePrompt = resp.CorePrompt
	}

	if err := config.SaveConfig(cfgPath, cfg); err != nil {
		return fmt.Errorf("保存配置失败: %w", err)
	}
	return appendPendingChanges(pendingPath, conflicts, logger)
}

func FormatConfigConflictMessage(conflicts []ConfigFieldChange, lang string) string {
	en := i18n.NormalizeLanguage(lang) == i18n.LangEN
	var sb strings.Builder
	if en {
		sb.WriteString("Cannot overwrite user-filled config fields without explicit consent. Conflicts:\n")
	} else {
		sb.WriteString("无法覆盖用户已填写的配置字段，需先征得用户同意。冲突字段：\n")
	}
	for _, c := range conflicts {
		sb.WriteString(fmt.Sprintf("- %s: current=%q proposed=%q\n", c.Field, c.Current, c.Proposed))
	}
	if en {
		sb.WriteString("Explain the changes to the user and wait for explicit approval, then retry update_project_config with confirm_overwrite=true.")
	} else {
		sb.WriteString("请向用户说明变更理由并等待明确同意，然后带 confirm_overwrite=true 重试 update_project_config。")
	}
	return sb.String()
}

func ApplySelectedPendingChanges(cfg *config.Config, state *Progress, pending *PendingConfigChanges, fields []string) {
	allow := make(map[string]bool, len(fields))
	for _, f := range fields {
		allow[f] = true
	}
	proposed := cfg.Story
	for _, change := range pending.Changes {
		if allow[change.Field] {
			setStoryFieldValue(&proposed, change.Field, change.Proposed)
		}
	}
	cfg.Story = proposed
	SyncProgressMetaFromStory(state, cfg.Story)
	snapshot := cfg.Story
	state.StoryConfigSnapshot = &snapshot
}
