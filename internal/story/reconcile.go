package story

import (
	"context"
	"encoding/json"
	"fmt"
	"showmethestory/internal/config"
	"showmethestory/internal/i18n"
	"showmethestory/internal/llm"
	"showmethestory/internal/sse"
)

type ReconciliationResult struct {
	Type          string `json:"type"`
	WritingStyle  string `json:"writing_style"`
	WritingPOV    string `json:"writing_pov"`
	StorySynopsis string `json:"story_synopsis"`
	Explanation   string `json:"explanation"`
}

func ReconcileSettingsAction(ctx context.Context, apiCfg *config.APIConfig, cfg *config.Config, state *Progress,
	newSettings config.StoryConfig, settings *ProjectSettings, progressPath string, cfgPath string, logger *sse.LogBroadcaster) error {

	logger.StepInfo(1, 3, "正在分析已有章节与新设定的兼容性...")
	lang := cfg.Language
	en := i18n.NormalizeLanguage(lang) == i18n.LangEN

	acceptedSummaries := ""
	for _, ch := range state.Chapters {
		if ch.Status == StatusAccepted && ch.Summary != "" {
			if en {
				acceptedSummaries += fmt.Sprintf("Chapter %d \"%s\" summary: %s\n", ch.Num, ch.Title, ch.Summary)
			} else {
				acceptedSummaries += fmt.Sprintf("第%d章《%s》摘要: %s\n", ch.Num, ch.Title, ch.Summary)
			}
		}
	}
	if acceptedSummaries == "" {
		if en {
			acceptedSummaries = "(no confirmed chapters yet)"
		} else {
			acceptedSummaries = "尚无已确认章节。"
		}
	}

	userPrompt := config.RenderPrompt(cfg.Prompts.SettingsReconciliation, map[string]string{
		"NewType":           newSettings.Type,
		"NewWritingStyle":   newSettings.WritingStyle,
		"NewWritingPOV":     newSettings.WritingPOV,
		"NewStorySynopsis":  newSettings.StorySynopsis,
		"ExistingSummaries": acceptedSummaries,
	})

	systemPrompt := i18n.SystemPromptFor(lang, "consistency_reviewer_json")

	rawResp := llm.CallAPIWithRetry(ctx, apiCfg, systemPrompt, userPrompt)
	if rawResp == "" {
		return fmt.Errorf("API 调用失败或被取消")
	}
	rawResp = cleanJSONResponse(rawResp)

	var result ReconciliationResult
	if err := json.Unmarshal([]byte(rawResp), &result); err != nil {
		return fmt.Errorf("解析协调结果JSON失败: %w\n原始响应: %s", err, rawResp)
	}

	logger.StepInfo(2, 3, "正在更新设定...")

	adjustedStory := storyConfigFromReconciliation(result, newSettings)
	conflicts := CollectStoryConfigConflicts(newSettings, adjustedStory, "reconcile", result.Explanation)
	pendingPath := PendingConfigChangesPath(progressPath)

	cfg.Story = newSettings
	SyncProgressMetaFromStory(state, newSettings)
	snapshot := newSettings
	state.StoryConfigSnapshot = &snapshot

	hasPending := false
	for _, ch := range state.Chapters {
		if ch.Status == StatusPending {
			hasPending = true
			break
		}
	}

	if hasPending {
		logger.StepInfo(3, 3, "正在基于新设定重新生成待定章节大纲...")
		if err := regeneratePendingOutlines(ctx, apiCfg, cfg, state, progressPath, cfgPath, logger); err != nil {
			logger.WarnKey("log.reconcile_pending_outline_failed", err)
		}
	}

	if err := config.SaveConfig(cfgPath, cfg); err != nil {
		return fmt.Errorf("保存配置失败: %w", err)
	}

	if err := SaveProgress(progressPath, state); err != nil {
		return fmt.Errorf("保存进度失败: %w", err)
	}

	if err := appendPendingChanges(pendingPath, conflicts, logger); err != nil {
		return err
	}

	runOutlinePostProcessChecks(ctx, apiCfg, cfg, state, settings, progressPath, logger)

	logger.SuccessKey("log.reconcile_done_explain" + result.Explanation)

	changedFields := []string{}
	for _, c := range conflicts {
		changedFields = append(changedFields, c.Field)
	}

	logger.SettingsReconciled(map[string]interface{}{
		"explanation":    result.Explanation,
		"changed_fields": changedFields,
	})

	return nil
}

func regeneratePendingOutlines(ctx context.Context, apiCfg *config.APIConfig, cfg *config.Config, state *Progress, progressPath, cfgPath string, logger *sse.LogBroadcaster) error {
	lang := cfg.Language
	en := i18n.NormalizeLanguage(lang) == i18n.LangEN

	pendingChapters := ""
	for _, ch := range state.Chapters {
		if ch.Status == StatusPending {
			pendingChapters += formatChapterLine(ch.Num, ch.Title, ch.Outline, lang)
		}
	}

	lockedChapters := ""
	for _, ch := range state.Chapters {
		if ch.Status == StatusAccepted {
			if en {
				lockedChapters += fmt.Sprintf("Chapter %d \"%s\" (summary): %s\n", ch.Num, ch.Title, ch.Summary)
			} else {
				lockedChapters += fmt.Sprintf("第%d章《%s》（摘要）: %s\n", ch.Num, ch.Title, ch.Summary)
			}
		}
	}
	if lockedChapters == "" {
		if en {
			lockedChapters = "(no locked chapters)"
		} else {
			lockedChapters = "无已锁定章节。"
		}
	}

	var feedback string
	if en {
		feedback = fmt.Sprintf("Story settings updated to: type=%s, writing_style=%s, writing_pov=%s, synopsis=%s. Adjust the pending chapter outlines so they stay consistent with the new settings and the existing chapters.",
			cfg.Story.Type, cfg.Story.WritingStyle, cfg.Story.WritingPOV, cfg.Story.StorySynopsis)
	} else {
		feedback = fmt.Sprintf("故事设定已更新为：类型=%s，写作风格=%s，叙述视角=%s，故事梗概=%s。请根据新设定调整待定章节大纲，使其与新设定和已有章节保持一致。",
			cfg.Story.Type, cfg.Story.WritingStyle, cfg.Story.WritingPOV, cfg.Story.StorySynopsis)
	}

	userPrompt := config.RenderPrompt(cfg.Prompts.OutlineRevision, map[string]string{
		"CurrentOutline": pendingChapters,
		"UserFeedback":   feedback,
		"LockedChapters": lockedChapters,
	})

	systemPrompt := i18n.SystemPromptFor(lang, "outline_editor_locked_json")

	rawResp := llm.CallAPIWithRetry(ctx, apiCfg, systemPrompt, userPrompt)
	if rawResp == "" {
		return fmt.Errorf("API 调用失败或被取消")
	}
	rawResp = cleanJSONResponse(rawResp)

	var resp OutlineResponse
	if err := json.Unmarshal([]byte(rawResp), &resp); err != nil {
		return fmt.Errorf("解析修订大纲JSON失败: %w", err)
	}

	return applyOutlineRevision(cfg, state, resp, "outline_revision", PendingConfigChangesPath(progressPath), cfgPath, logger)
}
