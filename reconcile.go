package main

import (
	"context"
	"encoding/json"
	"fmt"
)

type ReconciliationResult struct {
	Type          string `json:"type"`
	WritingStyle  string `json:"writing_style"`
	WritingPOV    string `json:"writing_pov"`
	StorySynopsis string `json:"story_synopsis"`
	Explanation   string `json:"explanation"`
}

func ReconcileSettingsAction(ctx context.Context, apiCfg *APIConfig, cfg *Config, state *Progress,
	newSettings StoryConfig, progressPath string, cfgPath string, logger *LogBroadcaster) error {

	logger.StepInfo(1, 3, "正在分析已有章节与新设定的兼容性...")
	lang := cfg.Language
	en := NormalizeLanguage(lang) == LangEN

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

	userPrompt := RenderPrompt(cfg.Prompts.SettingsReconciliation, map[string]string{
		"NewType":           newSettings.Type,
		"NewWritingStyle":   newSettings.WritingStyle,
		"NewWritingPOV":     newSettings.WritingPOV,
		"NewStorySynopsis":  newSettings.StorySynopsis,
		"ExistingSummaries": acceptedSummaries,
	})

	systemPrompt := SystemPromptFor(lang, "consistency_reviewer_json")

	rawResp := CallAPIWithRetry(ctx, apiCfg, systemPrompt, userPrompt)
	if rawResp == "" {
		return fmt.Errorf("API 调用失败或被取消")
	}
	rawResp = cleanJSONResponse(rawResp)

	var result ReconciliationResult
	if err := json.Unmarshal([]byte(rawResp), &result); err != nil {
		return fmt.Errorf("解析协调结果JSON失败: %w\n原始响应: %s", err, rawResp)
	}

	logger.StepInfo(2, 3, "正在更新设定...")

	adjustedStory := cfg.Story
	adjustedStory.Type = result.Type
	adjustedStory.WritingStyle = result.WritingStyle
	adjustedStory.WritingPOV = result.WritingPOV
	adjustedStory.StorySynopsis = result.StorySynopsis

	state.StoryConfigSnapshot = &adjustedStory

	hasPending := false
	for _, ch := range state.Chapters {
		if ch.Status == StatusPending {
			hasPending = true
			break
		}
	}

	if hasPending {
		logger.StepInfo(3, 3, "正在基于新设定重新生成待定章节大纲...")
		origStory := cfg.Story
		cfg.Story = adjustedStory
		if err := regeneratePendingOutlines(ctx, apiCfg, cfg, state, logger); err != nil {
			logger.WarnKey("log.reconcile_pending_outline_failed", err)
		}
		cfg.Story = origStory
	}

	cfg.Story = adjustedStory

	if err := saveConfig(cfgPath, cfg); err != nil {
		return fmt.Errorf("保存配置失败: %w", err)
	}

	if err := SaveProgress(progressPath, state); err != nil {
		return fmt.Errorf("保存进度失败: %w", err)
	}

	RunForeshadowOutlineCheckAndSave(ctx, apiCfg, cfg, state, progressPath, logger)

	logger.SuccessKey("log.reconcile_done_explain" + result.Explanation)

	changedFields := []string{}
	if result.Type != newSettings.Type {
		changedFields = append(changedFields, "type")
	}
	if result.WritingStyle != newSettings.WritingStyle {
		changedFields = append(changedFields, "writing_style")
	}
	if result.WritingPOV != newSettings.WritingPOV {
		changedFields = append(changedFields, "writing_pov")
	}
	if result.StorySynopsis != newSettings.StorySynopsis {
		changedFields = append(changedFields, "story_synopsis")
	}

	logger.SettingsReconciled(map[string]interface{}{
		"explanation":    result.Explanation,
		"changed_fields": changedFields,
	})

	return nil
}

func regeneratePendingOutlines(ctx context.Context, apiCfg *APIConfig, cfg *Config, state *Progress, logger *LogBroadcaster) error {
	lang := cfg.Language
	en := NormalizeLanguage(lang) == LangEN

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

	userPrompt := RenderPrompt(cfg.Prompts.OutlineRevision, map[string]string{
		"CurrentOutline": pendingChapters,
		"UserFeedback":   feedback,
		"LockedChapters": lockedChapters,
	})

	systemPrompt := SystemPromptFor(lang, "outline_editor_locked_json")

	rawResp := CallAPIWithRetry(ctx, apiCfg, systemPrompt, userPrompt)
	if rawResp == "" {
		return fmt.Errorf("API 调用失败或被取消")
	}
	rawResp = cleanJSONResponse(rawResp)

	var resp OutlineResponse
	if err := json.Unmarshal([]byte(rawResp), &resp); err != nil {
		return fmt.Errorf("解析修订大纲JSON失败: %w", err)
	}

	applyOutlineRevision(resp, state)

	return nil
}
