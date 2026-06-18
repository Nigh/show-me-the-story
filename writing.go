package main

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

func preferUserValue(userVal, fallback string) string {
	if userVal != "" {
		return userVal
	}
	return fallback
}

var (
	chapterMetaStartZH = regexp.MustCompile(`^[（(]?第\s*\d+\s*章`)
	chapterMetaStartEN = regexp.MustCompile(`(?i)^(?:chapter\s+\d+|part\s+\d+)`)
	// 匹配 AI 常见的元信息前缀行
	chapterMetaPreambleZH = regexp.MustCompile(`(?i)^(以下是|下面是|以上是|这是)?(以下为|以下是)?(修订后|修改后|润色后|重写后)?(的)?(完整)?(全文)?(第\s*\d+\s*章)?(正文|全文|完整正文|修订后正文)?[：:。.]?\s*$`)
	chapterMetaPreambleEN = regexp.MustCompile(`(?i)^(here\s+(?:is|are)|below\s+is|the\s+following\s+is|revised|full)\b.*(?:chapter|text|prose|content|version)`)
)

// stripChapterMetaProse trims common AI-emitted chapter framing lines from prose boundaries.
// ponytail: line-based heuristics only; won't catch inline meta. Upgrade: model instructions + structured output.
func stripChapterMetaProse(content string, lang string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return content
	}
	lines := strings.Split(content, "\n")
	for len(lines) > 0 && isChapterMetaLine(strings.TrimSpace(lines[0]), lang) {
		lines = lines[1:]
	}
	for len(lines) > 0 && isChapterMetaLine(strings.TrimSpace(lines[len(lines)-1]), lang) {
		lines = lines[:len(lines)-1]
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func isChapterMetaLine(line string, lang string) bool {
	if line == "" {
		return false
	}
	exact := []string{
		"本章完", "本章终", "待续", "未完待续", "（完）", "(完)", "完", "——", "—", "***", "---", "***",
		"End of chapter", "To be continued", "The End",
	}
	for _, s := range exact {
		if line == s || strings.HasPrefix(line, s+".") || strings.HasPrefix(line, s+"。") {
			return true
		}
	}
	if NormalizeLanguage(lang) == LangEN {
		if chapterMetaStartEN.MatchString(line) {
			return true
		}
		if matched, _ := regexp.MatchString(`(?i)^\(chapter\s+\d+.*\)$`, line); matched {
			return true
		}
		if chapterMetaPreambleEN.MatchString(line) {
			return true
		}
		return false
	}
	if chapterMetaStartZH.MatchString(line) {
		return true
	}
	if matched, _ := regexp.MatchString(`^[（(]第\s*\d+\s*章[^）)]*[）)]$`, line); matched {
		return true
	}
	if chapterMetaPreambleZH.MatchString(line) {
		return true
	}
	// 匹配 "以下为修订后的第X章完整正文" 等含章节号的元信息行
	if matched, _ := regexp.MatchString(`^.{0,10}(修订|修改|润色|重写).{0,5}第\s*\d+\s*章`, line); matched {
		return true
	}
	if matched, _ := regexp.MatchString(`^.{0,10}(完整|全部)?(正文|全文|内容).{0,5}(如下|如下方|如下所示)[：:。.]?\s*$`, line); matched {
		return true
	}
	return false
}

func formatWritingPOVBlock(pov, lang string) string {
	pov = strings.TrimSpace(pov)
	if pov == "" {
		return ""
	}
	if NormalizeLanguage(lang) == LangEN {
		return "[Narrative POV] " + pov
	}
	return "【叙述视角】" + pov
}

func formatExtraWritingConstraintsBlock(constraints, lang string) string {
	constraints = strings.TrimSpace(constraints)
	if constraints == "" {
		return ""
	}
	if NormalizeLanguage(lang) == LangEN {
		return "[Extra writing constraints (fact-check reconciliation)]\n" + constraints
	}
	return "【补充写作约束（事实核查冲突调和）】\n" + constraints
}

func GenerateChapterAction(ctx context.Context, apiCfg *APIConfig, cfg *Config, state *Progress, progressPath string, settings *ProjectSettings, logger *LogBroadcaster) error {
	if err := validateAPIConfig(apiCfg); err != nil {
		return err
	}
	if state.Phase != "writing" {
		return fmt.Errorf("当前不在写作阶段")
	}

	if state.CurrentChapterIndex >= len(state.Chapters) {
		return fmt.Errorf("所有章节已完成")
	}

	i := state.CurrentChapterIndex
	ch := &state.Chapters[i]

	if ch.Status == StatusAccepted {
		return fmt.Errorf("第 %d 章已确认，请确认当前章节或重置进度", ch.Num)
	}

	ch.Status = StatusWriting
	if err := SaveProgress(progressPath, state); err != nil {
		return err
	}

	logger.InfoKey("log.chapter_start", ch.Num, ch.Title)

	// 写前检查：本章大纲若已与实际写出的剧情冲突（如大纲安排初遇但前文已认识），
	// 先最小化修订大纲再动笔，避免按过时大纲写出矛盾内容。
	if i > 0 {
		logger.StepInfo(1, 5, "正在检查本章大纲与当前剧情的一致性...")
		revised, err := checkOutlineConsistency(ctx, apiCfg, cfg, state, i, logger)
		if err != nil {
			logger.WarnKey("log.outline_check_failed", err)
		} else if revised {
			if err := SaveProgress(progressPath, state); err != nil {
				return err
			}
			logger.InfoKey("log.outline_auto_revised")
		} else {
			logger.InfoKey("log.outline_consistent")
		}
	}

	if len(state.Foreshadows) > 0 {
		RunForeshadowOutlineCheckAndSave(ctx, apiCfg, cfg, state, progressPath, logger)
	}

	maxFactCheckRetries := 3
	extraConstraints := ""
	var accumulatedIssues []string

	for attempt := 0; attempt <= maxFactCheckRetries; attempt++ {
		if ctx.Err() != nil {
			return fmt.Errorf("任务已取消")
		}
		logger.StepInfo(2, 5, "正在构思并撰写正文...")
		content := generateChapterContentStreamWithRetryLog(ctx, apiCfg, cfg, state, i, settings, extraConstraints, logger)
		if content == "" {
			return fmt.Errorf("正文生成失败或被取消")
		}
		ch.Content = content
		logger.InfoKey("log.prose_done", len([]rune(content)))

		logger.StepInfo(3, 5, "正在提炼本章摘要...")
		summary := generateChapterSummaryWithRetryLog(ctx, apiCfg, cfg, content, logger)
		if summary == "" {
			return fmt.Errorf("摘要提炼失败或被取消")
		}
		ch.Summary = summary
		logger.InfoKey("log.summary_done")

		logger.StepInfo(4, 5, "正在对本章进行事实核查...")
		historySummary := buildHistorySummary(state, i)
		factCheckResult := generateChapterFactCheckWithRetryLog(ctx, apiCfg, cfg, state, i, content, historySummary, logger)

		failed, issues := parseFactCheckResult(factCheckResult)
		if failed {
			accumulatedIssues = mergeUniqueIssues(accumulatedIssues, splitFactCheckIssues(issues))
			if attempt < maxFactCheckRetries {
				logger.WarnKey("log.factcheck_retry", ch.Num, attempt+1)
				logger.WarnKey("log.factcheck_details", issues)
				continue
			}

			logger.WarnKey("log.factcheck_max_retries")
			analysis, err := analyzeWritingConflict(ctx, apiCfg, cfg, state, i, content, accumulatedIssues, logger)
			if err != nil {
				logger.WarnKey("log.conflict_analyze_failed", err)
				break
			}

			if analysis.Reconcilable && strings.TrimSpace(analysis.ExtraConstraints) != "" {
				logger.InfoKey("log.conflict_retry")
				extraConstraints = strings.TrimSpace(analysis.ExtraConstraints)
				content = generateChapterContentStreamWithRetryLog(ctx, apiCfg, cfg, state, i, settings, extraConstraints, logger)
				if content == "" {
					return fmt.Errorf("正文生成失败或被取消")
				}
				ch.Content = content
				summary = generateChapterSummaryWithRetryLog(ctx, apiCfg, cfg, content, logger)
				if summary == "" {
					return fmt.Errorf("摘要提炼失败或被取消")
				}
				ch.Summary = summary
				factCheckResult = generateChapterFactCheckWithRetryLog(ctx, apiCfg, cfg, state, i, content, historySummary, logger)
				failed, issues = parseFactCheckResult(factCheckResult)
				if failed {
					accumulatedIssues = mergeUniqueIssues(accumulatedIssues, splitFactCheckIssues(issues))
				} else {
					logger.InfoKey("log.factcheck_constraint_pass")
					break
				}
			}

			conflict := buildWritingConflict(state, i, accumulatedIssues, analysis)
			lang := cfg.Language
			conflict.SuggestedActions = ensureConflictActions(conflict.SuggestedActions, lang)
			state.PendingWritingConflict = conflict
			if err := SaveProgress(progressPath, state); err != nil {
				return err
			}
			logger.WritingConflict(conflict)
			return &WritingConflictError{Conflict: conflict}
		}
		logger.InfoKey("log.factcheck_pass")
		break
	}

	state.PendingWritingConflict = nil

	if len(state.Foreshadows) > 0 {
		logger.StepInfo(5, 5, "正在更新伏笔状态...")
		syncForeshadowsAfterChapter(ctx, apiCfg, cfg, state, i, progressPath, logger)
	}

	SaveChapterMarkdown(filepath.Dir(progressPath), *ch, state.Title)

	ch.Status = StatusReview
	state.CurrentChapterIndex = i
	if err := SaveProgress(progressPath, state); err != nil {
		return err
	}

	logger.SuccessKey("log.chapter_write_complete", ch.Num)
	return nil
}

// parseFactCheckResult 解析事实核查结果。
// 优先解析 JSON 中的 result 字段，解析失败时退化为字符串匹配。
func parseFactCheckResult(raw string) (failed bool, issues string) {
	cleaned := cleanJSONResponse(raw)
	var resp struct {
		Result string   `json:"result"`
		Issues []string `json:"issues"`
	}
	if jsonStr := extractJSON(cleaned); jsonStr != "" {
		if err := json.Unmarshal([]byte(jsonStr), &resp); err == nil && resp.Result != "" {
			return strings.EqualFold(strings.TrimSpace(resp.Result), "FAIL"), strings.Join(resp.Issues, "；")
		}
	}
	// fallback：无法解析 JSON 时按字符串匹配
	return strings.Contains(raw, "FAIL"), truncate(raw, 300)
}

// checkOutlineConsistency 写前大纲一致性检查：对照前情提要与上一章结尾，
// 检查本章大纲是否已与实际剧情冲突（如安排初遇但前文已认识）。
// 冲突时用 AI 给出的最小化修订替换本章大纲（仅当前章），返回是否发生了修订。
func checkOutlineConsistency(ctx context.Context, apiCfg *APIConfig, cfg *Config, state *Progress, idx int, logger *LogBroadcaster) (bool, error) {
	ch := &state.Chapters[idx]
	if strings.TrimSpace(ch.Outline) == "" {
		return false, nil
	}

	lang := cfg.Language
	prevEnding := ""
	if idx > 0 && state.Chapters[idx-1].Content != "" {
		if tail := tailAtParagraph(state.Chapters[idx-1].Content, prevTailMaxRunes); tail != "" {
			if NormalizeLanguage(lang) == LangEN {
				prevEnding = "[Previous chapter ending]\n" + tail + "\n\n"
			} else {
				prevEnding = "【上一章结尾原文】\n" + tail + "\n\n"
			}
		}
	}

	userPrompt := RenderPrompt(cfg.Prompts.OutlineConsistencyCheck, map[string]string{
		"ChapterNum":     fmt.Sprintf("%d", ch.Num),
		"ChapterTitle":   ch.Title,
		"ChapterOutline": ch.Outline,
		"HistorySummary": buildHistorySummaryForLang(state, idx, lang),
		"PreviousEnding": prevEnding,
	})
	systemPrompt := SystemPromptFor(lang, "outline_editor_brief_json")

	rawResp := CallAPIWithRetryLog(ctx, apiCfg, systemPrompt, userPrompt, logger)
	if rawResp == "" {
		return false, fmt.Errorf("API 调用失败或被取消")
	}

	var resp struct {
		Conflict       bool     `json:"conflict"`
		Issues         []string `json:"issues"`
		RevisedOutline string   `json:"revised_outline"`
	}
	jsonStr := extractJSON(cleanJSONResponse(rawResp))
	if jsonStr == "" {
		return false, fmt.Errorf("无法解析检查结果")
	}
	if err := json.Unmarshal([]byte(jsonStr), &resp); err != nil {
		return false, fmt.Errorf("解析检查结果JSON失败: %w", err)
	}

	if !resp.Conflict || strings.TrimSpace(resp.RevisedOutline) == "" {
		return false, nil
	}

	logger.WarnKey("log.outline_conflict", ch.Num, strings.Join(resp.Issues, "；"))
	ch.Outline = strings.TrimSpace(resp.RevisedOutline)
	return true, nil
}

// ReviseChapterAction 修订"当前章节"（写作流程中处于 review/writing 状态的章节）。
// 使用最小化修订提示词（提供原文），并在必要时同步修订后续 pending 章节大纲。
func ReviseChapterAction(ctx context.Context, apiCfg *APIConfig, cfg *Config, state *Progress, progressPath, feedback string, settings *ProjectSettings, logger *LogBroadcaster) error {
	if err := validateAPIConfig(apiCfg); err != nil {
		return err
	}
	if state.Phase != "writing" {
		return fmt.Errorf("当前不在写作阶段")
	}

	chapterIdx := state.CurrentChapterIndex
	if chapterIdx >= len(state.Chapters) {
		return fmt.Errorf("章节索引越界")
	}

	ch := &state.Chapters[chapterIdx]
	if ch.Status != StatusReview && ch.Status != StatusWriting {
		return fmt.Errorf("当前章节不在审核/写作状态")
	}

	logger.InfoKey("log.chapter_modifying", ch.Num, ch.Title)

	logger.StepInfo(1, 3, "正在根据意见修订正文...")
	revisedContent, err := reviseChapterContentStream(ctx, apiCfg, cfg, state, chapterIdx, feedback, settings, logger)
	if err != nil {
		return fmt.Errorf("修改章节失败: %w", err)
	}
	ch.Content = revisedContent
	logger.InfoKey("log.prose_revised", len([]rune(revisedContent)))

	logger.StepInfo(2, 3, "重新提炼摘要...")
	summary := generateChapterSummaryWithRetryLog(ctx, apiCfg, cfg, ch.Content, logger)
	if summary == "" {
		return fmt.Errorf("摘要提炼失败或被取消")
	}
	ch.Summary = summary
	logger.InfoKey("log.summary_done")

	SaveChapterMarkdown(filepath.Dir(progressPath), *ch, state.Title)

	if chapterIdx+1 < len(state.Chapters) {
		logger.StepInfo(3, 3, "正在修订后续章节大纲...")
		if err := reviseSubsequentOutlines(ctx, apiCfg, cfg, state, chapterIdx, feedback); err != nil {
			logger.WarnKey("log.subsequent_outline_failed", err)
		} else {
			logger.InfoKey("log.subsequent_outline_done")
		}
	}

	ch.Status = StatusReview
	if err := SaveProgress(progressPath, state); err != nil {
		return err
	}

	if len(state.Foreshadows) > 0 {
		syncForeshadowsAfterChapter(ctx, apiCfg, cfg, state, chapterIdx, progressPath, logger)
		if err := SaveProgress(progressPath, state); err != nil {
			return err
		}
	}

	logger.SuccessKey("log.chapter_revised")
	return nil
}

// ReviseSpecificChapterAction 对指定编号的章节做最小化修订（包括已确认章节）。
// 仅修改该章正文与摘要，绝不触碰其他章节、大纲或进度指针。
func ReviseSpecificChapterAction(ctx context.Context, apiCfg *APIConfig, cfg *Config, state *Progress, progressPath string, chapterNum int, feedback string, settings *ProjectSettings, logger *LogBroadcaster) error {
	if err := validateAPIConfig(apiCfg); err != nil {
		return err
	}
	if strings.TrimSpace(feedback) == "" {
		return fmt.Errorf("缺少修改意见")
	}

	chapterIdx := -1
	for i, ch := range state.Chapters {
		if ch.Num == chapterNum {
			chapterIdx = i
			break
		}
	}
	if chapterIdx == -1 {
		return fmt.Errorf("第 %d 章不存在", chapterNum)
	}

	ch := &state.Chapters[chapterIdx]
	if ch.Content == "" {
		return fmt.Errorf("第 %d 章尚未生成内容，无法修订（请先生成该章）", chapterNum)
	}
	if ch.Status == StatusWriting {
		return fmt.Errorf("第 %d 章正在写作中，无法修订", chapterNum)
	}

	logger.InfoKey("log.chapter_specific_revising_long", ch.Num, ch.Title)

	logger.StepInfo(1, 2, "正在根据意见修订正文...")
	revisedContent, err := reviseChapterContentStream(ctx, apiCfg, cfg, state, chapterIdx, feedback, settings, logger)
	if err != nil {
		return fmt.Errorf("修订章节失败: %w", err)
	}
	ch.Content = revisedContent
	logger.InfoKey("log.prose_specific_revised", len([]rune(revisedContent)))

	logger.StepInfo(2, 2, "重新提炼摘要...")
	summary := generateChapterSummaryWithRetryLog(ctx, apiCfg, cfg, ch.Content, logger)
	if summary == "" {
		return fmt.Errorf("摘要提炼失败或被取消")
	}
	ch.Summary = summary

	SaveChapterMarkdown(filepath.Dir(progressPath), *ch, state.Title)

	if err := SaveProgress(progressPath, state); err != nil {
		return err
	}

	if len(state.Foreshadows) > 0 {
		syncForeshadowsAfterChapter(ctx, apiCfg, cfg, state, chapterIdx, progressPath, logger)
		if err := SaveProgress(progressPath, state); err != nil {
			return err
		}
	}

	logger.SuccessKey("log.chapter_specific_done", ch.Num)
	return nil
}

func ConfirmChapterAction(state *Progress, progressPath string) error {
	if state.Phase != "writing" {
		return fmt.Errorf("当前不在写作阶段")
	}

	chapterIdx := state.CurrentChapterIndex
	if chapterIdx >= len(state.Chapters) {
		return fmt.Errorf("章节索引越界")
	}

	ch := &state.Chapters[chapterIdx]
	if ch.Status != StatusReview {
		return fmt.Errorf("当前章节不在审核状态，无法确认")
	}

	ch.Status = StatusAccepted
	state.CurrentChapterIndex = chapterIdx + 1
	return SaveProgress(progressPath, state)
}

func generateChapterContentStream(ctx context.Context, apiCfg *APIConfig, cfg *Config, state *Progress, idx int, settings *ProjectSettings, extraWritingConstraints string, logger *LogBroadcaster) (string, error) {
	ch := state.Chapters[idx]
	lang := cfg.Language

	historySummary := buildHistorySummaryForLang(state, idx, lang)

	snapshot := state.StoryConfigSnapshot
	if snapshot == nil {
		snapshot = &cfg.Story
	}

	foreshadowContext := formatActiveForeshadowsForChapterLang(state.Foreshadows, ch.Num, lang)

	characterContext := buildCharacterContextForLang(settings, ch.Outline, lang)
	worldviewContext := buildWorldviewContextForLang(settings, ch.Outline, lang)
	outlineConstraints := buildOutlineConstraintsForLang(state, idx, lang)

	userPrompt := RenderPrompt(cfg.Prompts.ChapterWriting, map[string]string{
		"Title":              preferUserValue(cfg.Story.Title, state.Title),
		"ChapterNum":         fmt.Sprintf("%d", ch.Num),
		"CorePrompt":         state.CorePrompt,
		"StorySynopsis":      preferUserValue(cfg.Story.StorySynopsis, state.StorySynopsis),
		"HistorySummary":     historySummary,
		"PreviousEnding":     buildPreviousChapterTailForLang(state, idx, lang),
		"ChapterTitle":       ch.Title,
		"ChapterOutline":     ch.Outline,
		"WritingStyle":       cfg.Story.WritingStyle,
		"WritingPOV":         cfg.Story.WritingPOV,
		"CharacterContext":   characterContext,
		"WorldviewContext":   worldviewContext,
		"TargetWords":        fmt.Sprintf("%d", snapshot.TargetWordsPerChapter),
		"Foreshadows":        foreshadowContext,
		"OutlineConstraints": outlineConstraints,
	})
	userPrompt = appendIfMissingPlaceholder(cfg.Prompts.ChapterWriting, userPrompt, "{{.OutlineConstraints}}", outlineConstraints)
	userPrompt = appendIfMissingPlaceholder(cfg.Prompts.ChapterWriting, userPrompt, "{{.Foreshadows}}", foreshadowContext)
	userPrompt = appendIfMissingPlaceholder(cfg.Prompts.ChapterWriting, userPrompt, "{{.WritingPOV}}", formatWritingPOVBlock(cfg.Story.WritingPOV, lang))
	if block := formatExtraWritingConstraintsBlock(extraWritingConstraints, lang); block != "" {
		userPrompt += "\n\n" + block
	}

	systemPrompt := state.CorePrompt
	if systemPrompt == "" {
		systemPrompt = SystemPromptFor(lang, "author_default")
	}

	onChunk := func(chunk string) {
		logger.ContentChunk(idx, chunk)
	}

	// 通知前端清空流式缓冲（事实核查重试/自动连写时避免内容叠加）
	logger.StreamStart(idx)
	content, err := CallAPIStream(ctx, apiCfg, systemPrompt, userPrompt, onChunk)
	if err != nil {
		return "", err
	}
	return stripChapterMetaProse(content, lang), nil
}

func generateChapterContentStreamWithRetryLog(ctx context.Context, apiCfg *APIConfig, cfg *Config, state *Progress, idx int, settings *ProjectSettings, extraWritingConstraints string, logger *LogBroadcaster) string {
	retryCount := 0
	for {
		if ctx.Err() != nil {
			return ""
		}
		content, err := generateChapterContentStream(ctx, apiCfg, cfg, state, idx, settings, extraWritingConstraints, logger)
		if err == nil && content != "" {
			return content
		}
		if isFatalAPIError(err) {
			logger.ErrorKey("log.fatal_no_retry", err)
			return ""
		}

		retryCount++
		waitTime := getWaitTime(retryCount)
		logger.WarnKey("log.content_gen_retry", err, retryCount, waitTime)
		select {
		case <-time.After(time.Duration(waitTime) * time.Second):
		case <-ctx.Done():
			return ""
		}
	}
}

func generateChapterSummary(ctx context.Context, apiCfg *APIConfig, cfg *Config, content string) (string, error) {
	userPrompt := RenderPrompt(cfg.Prompts.ChapterSummary, map[string]string{
		"ChapterContent": content,
	})

	systemPrompt := SystemPromptFor(cfg.Language, "summary_analyst")
	return CallAPI(ctx, apiCfg, systemPrompt, userPrompt)
}

func generateChapterSummaryWithRetryLog(ctx context.Context, apiCfg *APIConfig, cfg *Config, content string, logger *LogBroadcaster) string {
	retryCount := 0
	for {
		if ctx.Err() != nil {
			return ""
		}
		summary, err := generateChapterSummary(ctx, apiCfg, cfg, content)
		if err == nil && summary != "" {
			return summary
		}
		if isFatalAPIError(err) {
			logger.ErrorKey("log.fatal_no_retry", err)
			return ""
		}

		retryCount++
		waitTime := getWaitTime(retryCount)
		logger.WarnKey("log.summary_retry", err, retryCount, waitTime)
		select {
		case <-time.After(time.Duration(waitTime) * time.Second):
		case <-ctx.Done():
			return ""
		}
	}
}

func generateChapterFactCheck(ctx context.Context, apiCfg *APIConfig, cfg *Config, state *Progress, idx int, content string, historySummary string) (string, error) {
	ch := state.Chapters[idx]
	lang := cfg.Language
	outlineConstraints := buildOutlineConstraintsForLang(state, idx, lang)

	userPrompt := RenderPrompt(cfg.Prompts.FactCheck, map[string]string{
		"ChapterContent":     content,
		"HistorySummary":     historySummary,
		"CorePrompt":         "",
		"ChapterOutline":     ch.Outline,
		"OutlineConstraints": outlineConstraints,
	})
	// Old-template fallback: if placeholder is missing, append the material and supplementary checks at the end.
	if NormalizeLanguage(lang) == LangEN {
		userPrompt = appendIfMissingPlaceholder(cfg.Prompts.FactCheck, userPrompt, "{{.ChapterOutline}}",
			"[Chapter outline]\n"+ch.Outline)
		if outlineConstraints != "" {
			userPrompt = appendIfMissingPlaceholder(cfg.Prompts.FactCheck, userPrompt, "{{.OutlineConstraints}}",
				outlineConstraints+"Supplementary audit scope (also count as reportable objective contradictions): (a) premature introduction of characters/events scheduled for later chapters per the outline; (b) one-time events from prior chapters (first meetings, identity reveals, etc.) being re-enacted as new in this chapter.")
		}
	} else {
		userPrompt = appendIfMissingPlaceholder(cfg.Prompts.FactCheck, userPrompt, "{{.ChapterOutline}}",
			"【本章大纲】\n"+ch.Outline)
		if outlineConstraints != "" {
			userPrompt = appendIfMissingPlaceholder(cfg.Prompts.FactCheck, userPrompt, "{{.OutlineConstraints}}",
				outlineConstraints+"补充核查范围（同样属于必须报告的客观矛盾）：(a) 提前引入按章节脉络安排在后续章节才登场或发生的人物/事件；(b) 前文已发生的一次性事件（初次见面、身份揭示等）在本章作为新事件重复发生。")
		}
	}

	systemPrompt := SystemPromptFor(lang, "fact_checker_json")
	return CallAPI(ctx, apiCfg, systemPrompt, userPrompt)
}

func generateChapterFactCheckWithRetryLog(ctx context.Context, apiCfg *APIConfig, cfg *Config, state *Progress, idx int, content string, historySummary string, logger *LogBroadcaster) string {
	retryCount := 0
	for {
		if ctx.Err() != nil {
			return ""
		}
		result, err := generateChapterFactCheck(ctx, apiCfg, cfg, state, idx, content, historySummary)
		if err == nil && result != "" {
			return result
		}
		if isFatalAPIError(err) {
			logger.ErrorKey("log.fatal_no_retry", err)
			return ""
		}

		retryCount++
		waitTime := getWaitTime(retryCount)
		logger.WarnKey("log.factcheck_api_retry", err, retryCount, waitTime)
		select {
		case <-time.After(time.Duration(waitTime) * time.Second):
		case <-ctx.Done():
			return ""
		}
	}
}

// reviseChapterContentStream 基于原文做最小化修订（流式）。
func reviseChapterContentStream(ctx context.Context, apiCfg *APIConfig, cfg *Config, state *Progress, chapterIdx int, userFeedback string, settings *ProjectSettings, logger *LogBroadcaster) (string, error) {
	ch := state.Chapters[chapterIdx]
	lang := cfg.Language

	historySummary := buildHistorySummaryForLang(state, chapterIdx, lang)
	characterContext := buildCharacterContextForLang(settings, ch.Outline, lang)
	worldviewContext := buildWorldviewContextForLang(settings, ch.Outline, lang)

	userPrompt := RenderPrompt(cfg.Prompts.ChapterRevision, map[string]string{
		"ChapterNum":       fmt.Sprintf("%d", ch.Num),
		"ChapterTitle":     ch.Title,
		"CorePrompt":       state.CorePrompt,
		"HistorySummary":   historySummary,
		"WritingStyle":     cfg.Story.WritingStyle,
		"WritingPOV":       cfg.Story.WritingPOV,
		"CharacterContext": characterContext,
		"WorldviewContext": worldviewContext,
		"OriginalContent":  ch.Content,
		"UserFeedback":     userFeedback,
	})
	userPrompt = appendIfMissingPlaceholder(cfg.Prompts.ChapterRevision, userPrompt, "{{.WritingPOV}}", formatWritingPOVBlock(cfg.Story.WritingPOV, lang))

	systemPrompt := state.CorePrompt
	if systemPrompt == "" {
		systemPrompt = SystemPromptFor(lang, "author_default")
	}
	systemPrompt += SystemPromptFor(lang, "chapter_revision_suffix")

	onChunk := func(chunk string) {
		logger.ContentChunk(chapterIdx, chunk)
	}

	logger.StreamStart(chapterIdx)
	content, err := CallAPIStream(ctx, apiCfg, systemPrompt, userPrompt, onChunk)
	if err != nil {
		return "", err
	}
	return stripChapterMetaProse(content, lang), nil
}

func reviseSubsequentOutlines(ctx context.Context, apiCfg *APIConfig, cfg *Config, state *Progress, currentIdx int, userFeedback string) error {
	lang := cfg.Language
	en := NormalizeLanguage(lang) == LangEN

	subsequentChapters := ""
	for i := currentIdx + 1; i < len(state.Chapters); i++ {
		ch := state.Chapters[i]
		if ch.Status != StatusAccepted {
			subsequentChapters += formatChapterLine(ch.Num, ch.Title, ch.Outline, lang)
		}
	}
	if subsequentChapters == "" {
		return nil
	}

	lockedChapters := ""
	for i := 0; i <= currentIdx; i++ {
		ch := state.Chapters[i]
		if en {
			lockedChapters += fmt.Sprintf("Chapter %d \"%s\" (summary): %s\n", ch.Num, ch.Title, ch.Summary)
		} else {
			lockedChapters += fmt.Sprintf("第%d章《%s》（摘要）: %s\n", ch.Num, ch.Title, ch.Summary)
		}
	}

	var feedbackWrap string
	if en {
		feedbackWrap = fmt.Sprintf("The user gave revision feedback on chapter %d: %s\nOnly adjust later chapter outlines if this feedback affects downstream plot. If it is just wording detail, return the outlines verbatim.", state.Chapters[currentIdx].Num, userFeedback)
	} else {
		feedbackWrap = fmt.Sprintf("用户对第%d章提出了修改意见：%s\n请仅在该意见影响后续剧情时调整后续章节大纲；若意见只是文字细节修改，请原样返回大纲。", state.Chapters[currentIdx].Num, userFeedback)
	}

	userPrompt := RenderPrompt(cfg.Prompts.OutlineRevision, map[string]string{
		"CurrentOutline": subsequentChapters,
		"UserFeedback":   feedbackWrap,
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

	for _, newCh := range resp.Chapters {
		for i, existingCh := range state.Chapters {
			if existingCh.Num == newCh.Num && existingCh.Status != StatusAccepted {
				state.Chapters[i].Title = newCh.Title
				state.Chapters[i].Outline = newCh.Outline
			}
		}
	}

	return nil
}

// futureOutlineWindow 注入后续章节大纲的窗口大小（章数）
const futureOutlineWindow = 10

// buildOutlineConstraints — Chinese default; for English projects use the *ForLang variant.
func buildOutlineConstraints(state *Progress, idx int) string {
	return buildOutlineConstraintsForLang(state, idx, LangZH)
}

// appendIfMissingPlaceholder 旧项目兼容兜底：prompts 随 config.json 持久化，
// 老项目存的是没有新占位符的旧模板，applyDefaults 只在字段为空时回填。
// 若模板中缺少占位符，则把内容块追加到渲染结果末尾，保证新上下文仍然生效。
func appendIfMissingPlaceholder(template, rendered, placeholder, block string) string {
	if strings.TrimSpace(block) == "" || strings.Contains(template, placeholder) {
		return rendered
	}
	return rendered + "\n\n" + strings.TrimSpace(block)
}

func buildHistorySummary(state *Progress, idx int) string {
	return buildHistorySummaryForLang(state, idx, LangZH)
}

const (
	prevTailMaxRunes = 800  // 注入上一章尾部原文的最大字数
	openingMaxRunes  = 1000 // 衔接优化时提取本章开头片段的最大字数
)

// tailAtParagraph 取 content 末尾约 maxRunes 字，向后对齐到段落边界，避免从半句开始。
func tailAtParagraph(content string, maxRunes int) string {
	trimmed := strings.TrimSpace(content)
	runes := []rune(trimmed)
	if len(runes) <= maxRunes {
		return trimmed
	}
	tail := string(runes[len(runes)-maxRunes:])
	if i := strings.IndexByte(tail, '\n'); i >= 0 && i+1 < len(tail) {
		tail = tail[i+1:]
	}
	return strings.TrimSpace(tail)
}

// buildPreviousChapterTail — Chinese default; for English projects use the *ForLang variant.
func buildPreviousChapterTail(state *Progress, idx int) string {
	return buildPreviousChapterTailForLang(state, idx, LangZH)
}

// splitChapterOpening 把章节正文切分为开头片段与剩余部分，切点向前对齐到段落边界。
// rest 为空表示整章都算开头（章节较短）。
func splitChapterOpening(content string, maxRunes int) (opening, rest string) {
	runes := []rune(content)
	if len(runes) <= maxRunes {
		return content, ""
	}
	cut := maxRunes
	for i := maxRunes; i > 0; i-- {
		if runes[i-1] == '\n' {
			cut = i
			break
		}
	}
	return string(runes[:cut]), string(runes[cut:])
}

// SmoothTransitionsAction 批量优化已确认章节之间的衔接：
// 逐章把上一章尾部与本章开头交给 AI 判断，仅在衔接生硬时最小化重写本章开头片段。
// 每处理完一章立即落盘，任务可随时取消且不丢已完成部分。
func SmoothTransitionsAction(ctx context.Context, apiCfg *APIConfig, cfg *Config, state *Progress, progressPath string, logger *LogBroadcaster) error {
	if err := validateAPIConfig(apiCfg); err != nil {
		return err
	}

	var targets []int
	for i := 1; i < len(state.Chapters); i++ {
		if state.Chapters[i].Status == StatusAccepted && state.Chapters[i].Content != "" &&
			state.Chapters[i-1].Status == StatusAccepted && state.Chapters[i-1].Content != "" {
			targets = append(targets, i)
		}
	}
	if len(targets) == 0 {
		return fmt.Errorf("没有可优化的章节（需要至少两个相邻的已确认章节）")
	}

	logger.InfoKey("log.smooth_start", len(targets))
	optimized := 0
	for n, idx := range targets {
		if ctx.Err() != nil {
			return fmt.Errorf("任务已取消")
		}
		ch := &state.Chapters[idx]
		logger.StepInfo(n+1, len(targets), fmt.Sprintf("正在检查第 %d 章《%s》的衔接...", ch.Num, ch.Title))

		prevTail := tailAtParagraph(state.Chapters[idx-1].Content, prevTailMaxRunes)
		opening, rest := splitChapterOpening(ch.Content, openingMaxRunes)

		userPrompt := RenderPrompt(cfg.Prompts.TransitionSmoothing, map[string]string{
			"ChapterNum":     fmt.Sprintf("%d", ch.Num),
			"ChapterTitle":   ch.Title,
			"ChapterOutline": ch.Outline,
			"PrevTail":       prevTail,
			"Opening":        opening,
		})
		systemPrompt := SystemPromptFor(cfg.Language, "transition_editor")

		resp := CallAPIWithRetryLog(ctx, apiCfg, systemPrompt, userPrompt, logger)
		if resp == "" {
			return fmt.Errorf("第 %d 章衔接检查调用失败或被取消", ch.Num)
		}
		revised := strings.TrimSpace(resp)

		head := revised
		if len([]rune(head)) > 30 {
			head = string([]rune(head)[:30])
		}
		if revised == "" || strings.Contains(head, "NO_CHANGE") {
			logger.InfoKey("log.smooth_natural", ch.Num)
			continue
		}

		if rest == "" {
			ch.Content = revised
		} else {
			ch.Content = revised + "\n\n" + strings.TrimLeft(rest, "\n")
		}
		SaveChapterMarkdown(filepath.Dir(progressPath), *ch, state.Title)
		if err := SaveProgress(progressPath, state); err != nil {
			return err
		}
		optimized++
		logger.InfoKey("log.smooth_optimized", ch.Num)
	}

	logger.SuccessKey("log.smooth_done", len(targets), optimized)
	return nil
}

func PolishChapterAction(ctx context.Context, apiCfg *APIConfig, cfg *Config, state *Progress, chapterIdx int, skills []Skill, progressPath string, logger *LogBroadcaster) error {
	if chapterIdx < 0 || chapterIdx >= len(state.Chapters) {
		return fmt.Errorf("章节索引越界")
	}

	ch := &state.Chapters[chapterIdx]
	if ch.Content == "" {
		return fmt.Errorf("章节内容为空，无法润色")
	}

	skillsContent := FormatSkillsContent(skills)
	if skillsContent == "" {
		return fmt.Errorf("没有启用的润色技能，请先在技能管理页启用")
	}

	var userPrompt string
	if NormalizeLanguage(cfg.Language) == LangEN {
		userPrompt = fmt.Sprintf(`Polish the chapter below according to the rules. Output the full revised chapter prose. Do not add chapter titles, numbers, "End of chapter", or any other meta or explanatory text.

## Polish rules

%s

## Chapter to polish

%s`, skillsContent, ch.Content)
	} else {
		userPrompt = fmt.Sprintf(`请根据以下规则对下面的章节正文进行去AI味处理，输出修改后的完整正文。不要添加章节标题、章节号、「本章完」等任何元信息或说明性文字。

## 润色规则

%s

## 待处理正文

%s`, skillsContent, ch.Content)
	}

	systemPrompt := SystemPromptFor(cfg.Language, "polish_editor")

	onChunk := func(chunk string) {
		logger.ContentChunk(chapterIdx, chunk)
	}

	logger.StreamStart(chapterIdx)
	result, err := CallAPIStream(ctx, apiCfg, systemPrompt, userPrompt, onChunk)
	if err != nil {
		return fmt.Errorf("润色失败: %w", err)
	}

	ch.Content = stripChapterMetaProse(result, cfg.Language)
	ch.Status = StatusReview

	SaveChapterMarkdown(filepath.Dir(progressPath), *ch, state.Title)

	if err := SaveProgress(progressPath, state); err != nil {
		return fmt.Errorf("保存进度失败: %w", err)
	}

	return nil
}
