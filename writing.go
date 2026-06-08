package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

func GenerateChapterAction(apiCfg *APIConfig, cfg *Config, state *Progress, progressPath string, settings *ProjectSettings, logger *LogBroadcaster) error {
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

	logger.Info(fmt.Sprintf("开始创作第 %d 章: 《%s》", ch.Num, ch.Title))

	maxFactCheckRetries := 3
	for attempt := 0; attempt <= maxFactCheckRetries; attempt++ {
		logger.StepInfo(1, 4, "正在构思并撰写正文...")
		content := generateChapterContentStreamWithRetryLog(apiCfg, cfg, state, i, settings, logger)
		ch.Content = content
		logger.Info(fmt.Sprintf("正文撰写完毕，共 %d 字", len([]rune(content))))

		logger.StepInfo(2, 4, "正在提炼本章摘要...")
		summary := generateChapterSummaryWithRetryLog(apiCfg, cfg, content, logger)
		ch.Summary = summary
		logger.Info("摘要提炼完成")

		logger.StepInfo(3, 4, "正在对本章进行事实核查...")
		historySummary := buildHistorySummary(state, i)
		factCheckResult := generateChapterFactCheckWithRetryLog(apiCfg, cfg, content, historySummary, logger)

		if strings.Contains(factCheckResult, "FAIL") {
			if attempt < maxFactCheckRetries {
				logger.Warn(fmt.Sprintf("[事实核查] 发现问题，正在重新生成第 %d 章（第 %d 次重试）...", ch.Num, attempt+1))
				logger.Warn(fmt.Sprintf("核查详情: %s", factCheckResult))
				continue
			}
			logger.Warn("[事实核查] 已达最大重试次数，保留当前版本。")
		} else {
			logger.Info("[事实核查] 通过 ✓")
		}
		break
	}

	if len(state.Foreshadows) > 0 {
		logger.StepInfo(4, 4, "正在更新伏笔状态...")
		if err := UpdateForeshadows(apiCfg, cfg, state, i, logger); err != nil {
			logger.Warn(fmt.Sprintf("伏笔状态更新失败: %v（不影响本章）", err))
		} else {
			active := 0
			resolved := 0
			for _, fs := range state.Foreshadows {
				switch fs.Status {
				case ForeshadowPlanted, ForeshadowProgressing:
					active++
				case ForeshadowResolved:
					resolved++
				}
			}
			logger.Info(fmt.Sprintf("伏笔状态已更新（活跃: %d, 已回收: %d）", active, resolved))
		}
	}

	SaveChapterMarkdown(*ch, state.Title)

	ch.Status = StatusReview
	state.CurrentChapterIndex = i
	if err := SaveProgress(progressPath, state); err != nil {
		return err
	}

	if warn := BuildForeshadowWarnings(state); warn != "" {
		logger.Warn(warn)
	}

	logger.Success(fmt.Sprintf("第 %d 章创作完成！", ch.Num))
	return nil
}

func ReviseChapterAction(apiCfg *APIConfig, cfg *Config, state *Progress, progressPath, feedback string, logger *LogBroadcaster) error {
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

	logger.Info(fmt.Sprintf("正在修改第 %d 章《%s》...", ch.Num, ch.Title))

	logger.StepInfo(1, 3, "正在根据意见重写正文...")
	revisedContent, err := reviseChapterContentStream(apiCfg, cfg, state, chapterIdx, feedback, logger)
	if err != nil {
		return fmt.Errorf("修改章节失败: %w", err)
	}
	ch.Content = revisedContent
	logger.Info(fmt.Sprintf("正文修改完毕，共 %d 字", len([]rune(revisedContent))))

	logger.StepInfo(2, 3, "重新提炼摘要...")
	ch.Summary = generateChapterSummaryWithRetryLog(apiCfg, cfg, ch.Content, logger)
	logger.Info("摘要提炼完成")

	SaveChapterMarkdown(*ch, state.Title)

	if chapterIdx+1 < len(state.Chapters) {
		logger.StepInfo(3, 3, "正在修订后续章节大纲...")
		if err := reviseSubsequentOutlines(apiCfg, cfg, state, chapterIdx, feedback); err != nil {
			logger.Warn(fmt.Sprintf("后续大纲修订失败: %v（不影响当前章节）", err))
		} else {
			logger.Info("后续大纲修订完成")
		}
	}

	ch.Status = StatusReview
	if err := SaveProgress(progressPath, state); err != nil {
		return err
	}

	logger.Success(fmt.Sprintf("第 %d 章已修订。", ch.Num))
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

func generateChapterContent(apiCfg *APIConfig, cfg *Config, state *Progress, idx int, settings *ProjectSettings) (string, error) {
	ch := state.Chapters[idx]

	historySummary := buildHistorySummary(state, idx)

	snapshot := state.StoryConfigSnapshot
	if snapshot == nil {
		snapshot = &cfg.Story
	}

	foreshadowContext := formatActiveForeshadowsForChapter(state.Foreshadows, ch.Num)

	characterContext := buildCharacterContext(settings, ch.Outline)
	worldviewContext := buildWorldviewContext(settings, ch.Outline)

	userPrompt := RenderPrompt(cfg.Prompts.ChapterWriting, map[string]string{
		"Title":             state.Title,
		"ChapterNum":        fmt.Sprintf("%d", ch.Num),
		"CorePrompt":        state.CorePrompt,
		"CoreRequirements":  state.CoreRequirements,
		"HistorySummary":    historySummary,
		"ChapterTitle":      ch.Title,
		"ChapterOutline":    ch.Outline,
		"WritingStyle":      snapshot.WritingStyle,
		"CharacterSetting":  snapshot.CharacterSetting,
		"CharacterContext":  characterContext,
		"WorldSetting":      snapshot.WorldSetting,
		"WorldviewContext":  worldviewContext,
		"TargetWords":       fmt.Sprintf("%d", snapshot.TargetWordsPerChapter),
		"Foreshadows":       foreshadowContext,
	})

	systemPrompt := state.CorePrompt
	if systemPrompt == "" {
		systemPrompt = "你是一位小说作者。"
	}

	return CallAPI(apiCfg, systemPrompt, userPrompt)
}

func generateChapterContentStream(apiCfg *APIConfig, cfg *Config, state *Progress, idx int, settings *ProjectSettings, logger *LogBroadcaster) (string, error) {
	ch := state.Chapters[idx]

	historySummary := buildHistorySummary(state, idx)

	snapshot := state.StoryConfigSnapshot
	if snapshot == nil {
		snapshot = &cfg.Story
	}

	foreshadowContext := formatActiveForeshadowsForChapter(state.Foreshadows, ch.Num)

	characterContext := buildCharacterContext(settings, ch.Outline)
	worldviewContext := buildWorldviewContext(settings, ch.Outline)

	userPrompt := RenderPrompt(cfg.Prompts.ChapterWriting, map[string]string{
		"Title":             state.Title,
		"ChapterNum":        fmt.Sprintf("%d", ch.Num),
		"CorePrompt":        state.CorePrompt,
		"CoreRequirements":  state.CoreRequirements,
		"HistorySummary":    historySummary,
		"ChapterTitle":      ch.Title,
		"ChapterOutline":    ch.Outline,
		"WritingStyle":      snapshot.WritingStyle,
		"CharacterSetting":  snapshot.CharacterSetting,
		"CharacterContext":  characterContext,
		"WorldSetting":      snapshot.WorldSetting,
		"WorldviewContext":  worldviewContext,
		"TargetWords":       fmt.Sprintf("%d", snapshot.TargetWordsPerChapter),
		"Foreshadows":       foreshadowContext,
	})

	systemPrompt := state.CorePrompt
	if systemPrompt == "" {
		systemPrompt = "你是一位小说作者。"
	}

	totalChars := 0
	nextReport := 500
	onChunk := func(chunk string) {
		logger.ContentChunk(idx, chunk)
		totalChars += len([]rune(chunk))
		if totalChars >= nextReport {
			logger.StreamProgress(idx, totalChars)
			nextReport += 500
		}
	}

	return CallAPIStream(apiCfg, systemPrompt, userPrompt, onChunk)
}

func generateChapterContentWithRetry(apiCfg *APIConfig, cfg *Config, state *Progress, idx int, settings *ProjectSettings) string {
	retryCount := 0
	for {
		content, err := generateChapterContent(apiCfg, cfg, state, idx, settings)
		if err == nil && content != "" {
			return content
		}

		retryCount++
		waitTime := getWaitTime(retryCount)
		fmt.Printf(" ⚠️ [错误] 正文生成失败: %v。第 %d 次重试，等待 %ds 后重试...\n", err, retryCount, waitTime)
		time.Sleep(time.Duration(waitTime) * time.Second)
	}
}

func generateChapterContentStreamWithRetry(apiCfg *APIConfig, cfg *Config, state *Progress, idx int, settings *ProjectSettings, logger *LogBroadcaster) string {
	retryCount := 0
	for {
		content, err := generateChapterContentStream(apiCfg, cfg, state, idx, settings, logger)
		if err == nil && content != "" {
			return content
		}

		retryCount++
		waitTime := getWaitTime(retryCount)
		fmt.Printf(" ⚠️ [错误] 流式正文生成失败: %v。第 %d 次重试，等待 %ds 后重试...\n", err, retryCount, waitTime)
		time.Sleep(time.Duration(waitTime) * time.Second)
	}
}

func generateChapterContentStreamWithRetryLog(apiCfg *APIConfig, cfg *Config, state *Progress, idx int, settings *ProjectSettings, logger *LogBroadcaster) string {
	retryCount := 0
	for {
		content, err := generateChapterContentStream(apiCfg, cfg, state, idx, settings, logger)
		if err == nil && content != "" {
			return content
		}

		retryCount++
		waitTime := getWaitTime(retryCount)
		logger.Warn(fmt.Sprintf("正文生成失败: %v。第 %d 次重试，等待 %ds...", err, retryCount, waitTime))
		time.Sleep(time.Duration(waitTime) * time.Second)
	}
}

func generateChapterSummary(apiCfg *APIConfig, cfg *Config, content string) (string, error) {
	userPrompt := RenderPrompt(cfg.Prompts.ChapterSummary, map[string]string{
		"ChapterContent": content,
	})

	systemPrompt := "你是一位精准的小说叙事状态分析师。"
	return CallAPI(apiCfg, systemPrompt, userPrompt)
}

func generateChapterSummaryWithRetry(apiCfg *APIConfig, cfg *Config, content string) string {
	retryCount := 0
	for {
		summary, err := generateChapterSummary(apiCfg, cfg, content)
		if err == nil && summary != "" {
			return summary
		}

		retryCount++
		waitTime := getWaitTime(retryCount)
		fmt.Printf(" ⚠️ [错误] 摘要提炼失败: %v。第 %d 次重试，等待 %ds 后重试...\n", err, retryCount, waitTime)
		time.Sleep(time.Duration(waitTime) * time.Second)
	}
}

func generateChapterSummaryWithRetryLog(apiCfg *APIConfig, cfg *Config, content string, logger *LogBroadcaster) string {
	retryCount := 0
	for {
		summary, err := generateChapterSummary(apiCfg, cfg, content)
		if err == nil && summary != "" {
			return summary
		}

		retryCount++
		waitTime := getWaitTime(retryCount)
		logger.Warn(fmt.Sprintf("摘要提炼失败: %v。第 %d 次重试，等待 %ds...", err, retryCount, waitTime))
		time.Sleep(time.Duration(waitTime) * time.Second)
	}
}

func generateChapterFactCheck(apiCfg *APIConfig, cfg *Config, content string, historySummary string) (string, error) {
	userPrompt := RenderPrompt(cfg.Prompts.FactCheck, map[string]string{
		"ChapterContent": content,
		"HistorySummary": historySummary,
		"CorePrompt":     "",
	})

	systemPrompt := "你是一位严谨的小说事实核查员。请严格按照要求的JSON格式输出。"
	return CallAPI(apiCfg, systemPrompt, userPrompt)
}

func generateChapterFactCheckWithRetry(apiCfg *APIConfig, cfg *Config, content string, historySummary string) string {
	retryCount := 0
	for {
		result, err := generateChapterFactCheck(apiCfg, cfg, content, historySummary)
		if err == nil && result != "" {
			return result
		}

		retryCount++
		waitTime := getWaitTime(retryCount)
		fmt.Printf(" ⚠️ [错误] 事实核查失败: %v。第 %d 次重试，等待 %ds 后重试...\n", err, retryCount, waitTime)
		time.Sleep(time.Duration(waitTime) * time.Second)
	}
}

func generateChapterFactCheckWithRetryLog(apiCfg *APIConfig, cfg *Config, content string, historySummary string, logger *LogBroadcaster) string {
	retryCount := 0
	for {
		result, err := generateChapterFactCheck(apiCfg, cfg, content, historySummary)
		if err == nil && result != "" {
			return result
		}

		retryCount++
		waitTime := getWaitTime(retryCount)
		logger.Warn(fmt.Sprintf("事实核查失败: %v。第 %d 次重试，等待 %ds...", err, retryCount, waitTime))
		time.Sleep(time.Duration(waitTime) * time.Second)
	}
}

func reviseChapterContent(apiCfg *APIConfig, cfg *Config, state *Progress, chapterIdx int, userFeedback string) (string, error) {
	ch := state.Chapters[chapterIdx]

	historySummary := buildHistorySummary(state, chapterIdx)

	systemPrompt := state.CorePrompt
	if systemPrompt == "" {
		systemPrompt = "你是一位小说作者。"
	}

	userPrompt := fmt.Sprintf(`请根据以下意见修改第 %d 章《%s》的正文。

【核心写作提示词】%s
【核心写作要求】%s
【前情提要】%s
【本章大纲】%s
【用户修改意见】%s

请输出修改后的完整章节正文。`,
		ch.Num, ch.Title,
		state.CorePrompt, state.CoreRequirements,
		historySummary, ch.Outline, userFeedback)

	return CallAPI(apiCfg, systemPrompt, userPrompt)
}

func reviseChapterContentStream(apiCfg *APIConfig, cfg *Config, state *Progress, chapterIdx int, userFeedback string, logger *LogBroadcaster) (string, error) {
	ch := state.Chapters[chapterIdx]

	historySummary := buildHistorySummary(state, chapterIdx)

	systemPrompt := state.CorePrompt
	if systemPrompt == "" {
		systemPrompt = "你是一位小说作者。"
	}

	userPrompt := fmt.Sprintf(`请根据以下意见修改第 %d 章《%s》的正文。

【核心写作提示词】%s
【核心写作要求】%s
【前情提要】%s
【本章大纲】%s
【用户修改意见】%s

请输出修改后的完整章节正文。`,
		ch.Num, ch.Title,
		state.CorePrompt, state.CoreRequirements,
		historySummary, ch.Outline, userFeedback)

	totalChars := 0
	nextReport := 500
	onChunk := func(chunk string) {
		logger.ContentChunk(chapterIdx, chunk)
		totalChars += len([]rune(chunk))
		if totalChars >= nextReport {
			logger.StreamProgress(chapterIdx, totalChars)
			nextReport += 500
		}
	}

	return CallAPIStream(apiCfg, systemPrompt, userPrompt, onChunk)
}

func reviseSubsequentOutlines(apiCfg *APIConfig, cfg *Config, state *Progress, currentIdx int, userFeedback string) error {
	subsequentChapters := ""
	for i := currentIdx + 1; i < len(state.Chapters); i++ {
		ch := state.Chapters[i]
		if ch.Status != StatusAccepted {
			subsequentChapters += fmt.Sprintf("第%d章《%s》: %s\n", ch.Num, ch.Title, ch.Outline)
		}
	}
	if subsequentChapters == "" {
		return nil
	}

	lockedChapters := ""
	for i := 0; i <= currentIdx; i++ {
		ch := state.Chapters[i]
		lockedChapters += fmt.Sprintf("第%d章《%s》（摘要）: %s\n", ch.Num, ch.Title, ch.Summary)
	}

	userPrompt := RenderPrompt(cfg.Prompts.OutlineRevision, map[string]string{
		"CurrentOutline": subsequentChapters,
		"UserFeedback":   fmt.Sprintf("用户对第%d章提出了修改意见：%s\n请根据此意见修订后续章节大纲。", state.Chapters[currentIdx].Num, userFeedback),
		"LockedChapters": lockedChapters,
	})

	systemPrompt := "你是一位小说策划编辑。请严格按照要求的JSON格式输出，不要添加任何额外文字或markdown代码块标记。已锁定的章节内容不可修改。"

	rawResp := CallAPIWithRetry(apiCfg, systemPrompt, userPrompt)
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

func buildHistorySummary(state *Progress, idx int) string {
	startIdx := 0
	if idx > 5 {
		startIdx = idx - 5
	}
	var history string
	for i := startIdx; i < idx; i++ {
		if state.Chapters[i].Summary != "" {
			history += fmt.Sprintf("[第%d章摘要]: %s\n", state.Chapters[i].Num, state.Chapters[i].Summary)
		}
	}
	if history == "" {
		history = "当前为故事开端，无历史前情。"
	}
	return history
}

func PolishChapterAction(apiCfg *APIConfig, cfg *Config, state *Progress, chapterIdx int, skills []Skill, progressPath string, logger *LogBroadcaster) error {
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

	userPrompt := fmt.Sprintf(`请根据以下规则对下面的章节正文进行去AI味处理，输出修改后的完整正文。

## 润色规则

%s

## 待处理正文

%s`, skillsContent, ch.Content)

	systemPrompt := "你是一位专业的中文小说润色编辑。请严格按照规则修改文本，输出修改后的完整章节正文。不要添加任何解释或标记。"

	totalChars := 0
	nextReport := 500
	onChunk := func(chunk string) {
		logger.ContentChunk(chapterIdx, chunk)
		totalChars += len([]rune(chunk))
		if totalChars >= nextReport {
			logger.StreamProgress(chapterIdx, totalChars)
			nextReport += 500
		}
	}

	result, err := CallAPIStream(apiCfg, systemPrompt, userPrompt, onChunk)
	if err != nil {
		return fmt.Errorf("润色失败: %w", err)
	}

	ch.Content = result
	ch.Status = StatusReview

	SaveChapterMarkdown(*ch, state.Title)

	if err := SaveProgress(progressPath, state); err != nil {
		return fmt.Errorf("保存进度失败: %w", err)
	}

	logger.PolishResult(chapterIdx, result)
	return nil
}
