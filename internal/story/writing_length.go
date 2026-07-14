package story

import (
	"context"
	"fmt"
	"showmethestory/internal/config"
	"showmethestory/internal/i18n"
	"showmethestory/internal/llm"
	"showmethestory/internal/prose"
	"showmethestory/internal/sse"
	"strings"
)

const (
	chapterLengthToleranceAbsolute = 1000
	chapterLengthTolerancePercent  = 15
	chapterGenMaxLengthAttempts    = 1 // initial draft + one rewrite, then compress/expand

	chapterLengthSoftOverflowAbsolute = 200 // soft keep: within max(±200, ±5%) of bound
	chapterLengthSoftOverflowPercent  = 5
	chapterLengthAdjustMaxOverRatio   = 120 // skip compress when actual > max * 1.2
	chapterLengthAdjustMinUnderRatio  = 80  // skip expand when actual < min * 0.8
)

const proseLengthOutOfRangeBase = 1_000_000

// calcChapterLengthRange returns acceptable chapter prose length bounds (prose units).
// Tolerance is max(±1000, ±15% of target). ponytail: fixed policy; tune constants if needed.
func calcChapterLengthRange(targetWordsPerChapter int) (minLen, maxLen int) {
	if targetWordsPerChapter < 1 {
		targetWordsPerChapter = 2500
	}
	tol := chapterLengthToleranceAbsolute
	if pct := targetWordsPerChapter * chapterLengthTolerancePercent / 100; pct > tol {
		tol = pct
	}
	minLen = targetWordsPerChapter - tol
	if minLen < 1 {
		minLen = 1
	}
	maxLen = targetWordsPerChapter + tol
	return minLen, maxLen
}

func chapterLengthInRange(content string, minLen, maxLen int) bool {
	n := prose.CountProseUnits(content)
	return n >= minLen && n <= maxLen
}

// proseLengthScore ranks drafts: lower is better. In-range drafts beat any out-of-range;
// in-range ties break by distance to target; out-of-range by distance to nearest bound.
func proseLengthScore(actual, minLen, maxLen, target int) int {
	if actual >= minLen && actual <= maxLen {
		diff := actual - target
		if diff < 0 {
			diff = -diff
		}
		return diff
	}
	if actual > maxLen {
		return proseLengthOutOfRangeBase + (actual - maxLen)
	}
	return proseLengthOutOfRangeBase + (minLen - actual)
}

func softLengthTolerance(boundary int) int {
	tol := chapterLengthSoftOverflowAbsolute
	if pct := boundary * chapterLengthSoftOverflowPercent / 100; pct > tol {
		tol = pct
	}
	return tol
}

func isSoftLengthDeviation(actual, minLen, maxLen int) bool {
	if actual > maxLen {
		return actual-maxLen <= softLengthTolerance(maxLen)
	}
	if actual < minLen {
		return minLen-actual <= softLengthTolerance(minLen)
	}
	return false
}

func shouldAttemptLengthAdjust(actual, minLen, maxLen int) bool {
	if actual >= minLen && actual <= maxLen {
		return false
	}
	if isSoftLengthDeviation(actual, minLen, maxLen) {
		return false
	}
	if actual > maxLen {
		return actual <= maxLen*chapterLengthAdjustMaxOverRatio/100
	}
	return actual >= minLen*chapterLengthAdjustMinUnderRatio/100
}

func maybeUpdateBestDraft(bestContent *string, bestScore *int, content string, minLen, maxLen, target int) {
	score := proseLengthScore(prose.CountProseUnits(content), minLen, maxLen, target)
	if *bestContent == "" || score < *bestScore {
		*bestContent = content
		*bestScore = score
	}
}

func formatChapterLengthRequirementBlock(minLen, maxLen, target int, lang string) string {
	if i18n.NormalizeLanguage(lang) == i18n.LangEN {
		return fmt.Sprintf("Chapter prose must be %d–%d words (target %d; tolerance ±%d words or ±%d%%, whichever is larger). Exceeding %d words is unacceptable — stay inside this chapter's outline only.", minLen, maxLen, target, chapterLengthToleranceAbsolute, chapterLengthTolerancePercent, maxLen)
	}
	return fmt.Sprintf("正文字数须严格控制在 %d–%d 字（目标 %d 字；允许误差 ±%d 字或 ±%d%%，取较大者）。超过 %d 字不可接受，只写本章大纲范围内的情节。", minLen, maxLen, target, chapterLengthToleranceAbsolute, chapterLengthTolerancePercent, maxLen)
}

func formatChapterLengthRetryFeedback(actual, minLen, maxLen int, lang string) string {
	if actual > maxLen {
		if i18n.NormalizeLanguage(lang) == i18n.LangEN {
			return fmt.Sprintf("IMPORTANT: The previous draft was %d words, exceeding the %d–%d word limit. Regenerate this chapter within the limit. Do not advance into later chapters; compress redundant description and keep only this chapter's outline beats.", actual, minLen, maxLen)
		}
		return fmt.Sprintf("重要：上一稿为 %d 字，超出 %d–%d 字上限。请重新撰写并严格控制在范围内；不要写入后续章节内容，精简冗余描写，只保留本章大纲情节。", actual, minLen, maxLen)
	}
	if i18n.NormalizeLanguage(lang) == i18n.LangEN {
		return fmt.Sprintf("IMPORTANT: The previous draft was only %d words, below the %d–%d word range. Expand with concrete scene, action, and dialogue while staying inside this chapter's outline.", actual, minLen, maxLen)
	}
	return fmt.Sprintf("重要：上一稿仅 %d 字，低于 %d–%d 字下限。请在不超出本章大纲的前提下补充具体场景、动作与对话。", actual, minLen, maxLen)
}

func mergeWritingConstraints(a, b string) string {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	if a == "" {
		return b
	}
	if b == "" {
		return a
	}
	return a + "\n\n" + b
}

func finalizeChapterWritingPrompt(template, rendered string, minLen, maxLen, target int, lang string) string {
	if !strings.Contains(template, "{{.TargetWordsMin}}") {
		rendered += "\n\n" + formatChapterLengthRequirementBlock(minLen, maxLen, target, lang)
	}
	return rendered
}

func adjustChapterLength(ctx context.Context, apiCfg *config.APIConfig, cfg *config.Config, content string, minLen, maxLen int, logger *sse.LogBroadcaster) (string, error) {
	actual := prose.CountProseUnits(content)
	lang := cfg.Language
	var userPrompt string
	if actual > maxLen {
		if i18n.NormalizeLanguage(lang) == i18n.LangEN {
			userPrompt = fmt.Sprintf(`You are a novel editor. The chapter below is %d words but must be %d–%d words.
Compress without changing plot beats, character actions, or key dialogue. Cut redundant description and repeated narration; do not remove core events from this chapter's outline.
Output ONLY the full revised chapter prose.

[Chapter text]
%s`, actual, minLen, maxLen, content)
		} else {
			userPrompt = fmt.Sprintf(`你是小说编辑。以下章节为 %d 字，须压缩至 %d–%d 字。
在不改变本章情节走向、人物行为与关键对话的前提下精简冗余描写与重复叙述；不得删去大纲中的核心事件。
只输出修改后的完整正文。

【章节正文】
%s`, actual, minLen, maxLen, content)
		}
	} else {
		if i18n.NormalizeLanguage(lang) == i18n.LangEN {
			userPrompt = fmt.Sprintf(`You are a novel editor. The chapter below is %d words but must be %d–%d words.
Expand with concrete scene, sensory detail, and dialogue without adding new plot beats beyond this chapter's outline.
Output ONLY the full revised chapter prose.

[Chapter text]
%s`, actual, minLen, maxLen, content)
		} else {
			userPrompt = fmt.Sprintf(`你是小说编辑。以下章节为 %d 字，须扩展至 %d–%d 字。
在不超出本章大纲的前提下补充具体场景、感官细节与对话，不要添加新情节线。
只输出修改后的完整正文。

【章节正文】
%s`, actual, minLen, maxLen, content)
		}
	}

	systemPrompt := i18n.SystemPromptFor(lang, "author_default")
	var raw string
	if logger != nil {
		raw = llm.CallAPIWithRetryLog(ctx, apiCfg, systemPrompt, userPrompt, logger)
	} else {
		raw = llm.CallAPIWithRetry(ctx, apiCfg, systemPrompt, userPrompt)
	}
	if raw == "" {
		return "", fmt.Errorf("章节字数调整 API 调用失败或被取消")
	}
	return stripChapterMetaProse(raw, lang), nil
}

func recoverChapterLengthBest(ctx context.Context, apiCfg *config.APIConfig, cfg *config.Config, bestContent string, minLen, maxLen, target int, bestScore int, logger *sse.LogBroadcaster) (string, error) {
	if bestContent == "" {
		return "", fmt.Errorf("正文生成失败或被取消")
	}
	actual := prose.CountProseUnits(bestContent)
	if chapterLengthInRange(bestContent, minLen, maxLen) {
		return bestContent, nil
	}
	if isSoftLengthDeviation(actual, minLen, maxLen) {
		if logger != nil {
			logger.WarnKey("log.chapter_length_soft_keep", actual, minLen, maxLen)
			logger.WarnKey("log.chapter_length_off_range", actual, minLen, maxLen)
		}
		return bestContent, nil
	}
	if !shouldAttemptLengthAdjust(actual, minLen, maxLen) {
		if logger != nil {
			logger.WarnKey("log.chapter_length_skip_adjust", actual, minLen, maxLen)
			logger.WarnKey("log.chapter_length_off_range", actual, minLen, maxLen)
		}
		return bestContent, nil
	}
	if logger != nil {
		logger.WarnKey("log.chapter_length_adjust", actual, minLen, maxLen)
	}
	adjusted, err := adjustChapterLength(ctx, apiCfg, cfg, bestContent, minLen, maxLen, logger)
	if err != nil || adjusted == "" {
		if logger != nil {
			logger.WarnKey("log.chapter_length_adjust_failed", err)
			logger.WarnKey("log.chapter_length_off_range", actual, minLen, maxLen)
		}
		return bestContent, nil
	}
	adjustedLen := prose.CountProseUnits(adjusted)
	if chapterLengthInRange(adjusted, minLen, maxLen) {
		return adjusted, nil
	}
	adjustedScore := proseLengthScore(adjustedLen, minLen, maxLen, target)
	if adjustedScore >= bestScore {
		if logger != nil {
			logger.WarnKey("log.chapter_length_adjust_reverted", adjustedLen, actual, minLen, maxLen)
			logger.WarnKey("log.chapter_length_off_range", actual, minLen, maxLen)
		}
		return bestContent, nil
	}
	if logger != nil {
		logger.WarnKey("log.chapter_length_off_range", adjustedLen, minLen, maxLen)
	}
	return adjusted, nil
}

func generateChapterContentWithLengthControl(ctx context.Context, apiCfg *config.APIConfig, cfg *config.Config, state *Progress, idx int, settings *ProjectSettings, extraWritingConstraints string, logger *sse.LogBroadcaster) (string, error) {
	snapshot := state.StoryConfigSnapshot
	if snapshot == nil {
		snapshot = &cfg.Story
	}
	target := snapshot.TargetWordsPerChapter
	minLen, maxLen := calcChapterLengthRange(target)
	lang := cfg.Language
	lengthFeedback := ""
	var bestContent string
	bestScore := int(^uint(0) >> 1)

	for attempt := 0; attempt <= chapterGenMaxLengthAttempts; attempt++ {
		if ctx.Err() != nil {
			return "", fmt.Errorf("任务已取消")
		}
		constraints := mergeWritingConstraints(extraWritingConstraints, lengthFeedback)
		content := generateChapterContentStreamWithRetryLog(ctx, apiCfg, cfg, state, idx, settings, constraints, logger)
		if content == "" {
			return "", fmt.Errorf("正文生成失败或被取消")
		}
		actualLen := prose.CountProseUnits(content)
		maybeUpdateBestDraft(&bestContent, &bestScore, content, minLen, maxLen, target)
		if chapterLengthInRange(content, minLen, maxLen) {
			return content, nil
		}
		if attempt < chapterGenMaxLengthAttempts {
			if logger != nil {
				logger.WarnKey("log.chapter_length_retry", actualLen, minLen, maxLen, attempt+1)
			}
			lengthFeedback = formatChapterLengthRetryFeedback(actualLen, minLen, maxLen, lang)
			continue
		}
	}

	return recoverChapterLengthBest(ctx, apiCfg, cfg, bestContent, minLen, maxLen, target, bestScore, logger)
}
