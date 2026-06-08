package main

import (
	"context"
	"encoding/json"
	"fmt"
)

type ReconciliationResult struct {
	Type          string `json:"type"`
	WritingStyle  string `json:"writing_style"`
	StorySynopsis string `json:"story_synopsis"`
	Explanation   string `json:"explanation"`
}

func ReconcileSettingsAction(ctx context.Context, apiCfg *APIConfig, cfg *Config, state *Progress,
	newSettings StoryConfig, progressPath string, cfgPath string, logger *LogBroadcaster) error {

	logger.StepInfo(1, 3, "正在分析已有章节与新设定的兼容性...")

	acceptedSummaries := ""
	for _, ch := range state.Chapters {
		if ch.Status == StatusAccepted && ch.Summary != "" {
			acceptedSummaries += fmt.Sprintf("第%d章《%s》摘要: %s\n", ch.Num, ch.Title, ch.Summary)
		}
	}
	if acceptedSummaries == "" {
		acceptedSummaries = "尚无已确认章节。"
	}

	userPrompt := RenderPrompt(cfg.Prompts.SettingsReconciliation, map[string]string{
		"NewType":          newSettings.Type,
		"NewWritingStyle":  newSettings.WritingStyle,
		"NewStorySynopsis": newSettings.StorySynopsis,
		"ExistingSummaries": acceptedSummaries,
	})

	systemPrompt := "你是一位专业的小说一致性审查编辑。请严格按照要求的JSON格式输出，不要添加任何额外文字或markdown代码块标记。"

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
			logger.Warn(fmt.Sprintf("待定章节大纲重新生成失败: %v（设定已更新）", err))
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

	logger.Success("设定协调完成。" + result.Explanation)

	changedFields := []string{}
	if result.Type != newSettings.Type {
		changedFields = append(changedFields, "type")
	}
	if result.WritingStyle != newSettings.WritingStyle {
		changedFields = append(changedFields, "writing_style")
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
	pendingChapters := ""
	for _, ch := range state.Chapters {
		if ch.Status == StatusPending {
			pendingChapters += fmt.Sprintf("第%d章《%s》: %s\n", ch.Num, ch.Title, ch.Outline)
		}
	}

	lockedChapters := ""
	for _, ch := range state.Chapters {
		if ch.Status == StatusAccepted {
			lockedChapters += fmt.Sprintf("第%d章《%s》（摘要）: %s\n", ch.Num, ch.Title, ch.Summary)
		}
	}
	if lockedChapters == "" {
		lockedChapters = "无已锁定章节。"
	}

	feedback := fmt.Sprintf("故事设定已更新为：类型=%s，写作风格=%s，故事梗概=%s。请根据新设定调整待定章节大纲，使其与新设定和已有章节保持一致。",
		cfg.Story.Type, cfg.Story.WritingStyle, cfg.Story.StorySynopsis)

	userPrompt := RenderPrompt(cfg.Prompts.OutlineRevision, map[string]string{
		"CurrentOutline": pendingChapters,
		"UserFeedback":   feedback,
		"LockedChapters": lockedChapters,
	})

	systemPrompt := "你是一位小说策划编辑。请严格按照要求的JSON格式输出，不要添加任何额外文字或markdown代码块标记。已锁定的章节内容不可修改。"

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
