package story

import (
	"context"
	"encoding/json"
	"fmt"
	"showmethestory/internal/config"
	"showmethestory/internal/i18n"
	"showmethestory/internal/llm"
	"showmethestory/internal/sse"
	"strings"
)

type OutlineResponse struct {
	Title         string           `json:"title"`
	CorePrompt    string           `json:"core_prompt"`
	StorySynopsis string           `json:"story_synopsis"`
	Chapters      []OutlineChapter `json:"chapters"`
}

// OutlineChapterCharacter is a machine-readable cast entry for one chapter outline.
// Prefer this over scraping prose for「首次登场」; name must be the proper name only.
type OutlineChapterCharacter struct {
	Name            string `json:"name"`
	FirstAppearance bool   `json:"first_appearance,omitempty"`
	Note            string `json:"note,omitempty"` // one-line role / relationship for debuts
}

type OutlineChapter struct {
	Num        int                       `json:"num"`
	Title      string                    `json:"title"`
	Outline    string                    `json:"outline"`
	Characters []OutlineChapterCharacter `json:"characters,omitempty"`
}

// normalizeOutlineCharacters trims names, drops empties, dedupes within a chapter.
func normalizeOutlineCharacters(in []OutlineChapterCharacter) []OutlineChapterCharacter {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(in))
	out := make([]OutlineChapterCharacter, 0, len(in))
	for _, c := range in {
		name := strings.TrimSpace(StripNameMarks(c.Name))
		name = strings.Trim(name, "：:、\"'「」『』")
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, OutlineChapterCharacter{
			Name:            name,
			FirstAppearance: c.FirstAppearance,
			Note:            strings.TrimSpace(c.Note),
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func chapterStateFromOutline(oc OutlineChapter, status string) ChapterState {
	return ChapterState{
		Num:        oc.Num,
		Title:      oc.Title,
		Outline:    oc.Outline,
		Characters: normalizeOutlineCharacters(oc.Characters),
		Status:     status,
	}
}

func parseOutlineResponse(rawResp string) (*OutlineResponse, error) {
	rawResp = cleanJSONResponse(rawResp)
	var resp OutlineResponse
	if err := json.Unmarshal([]byte(rawResp), &resp); err != nil {
		return nil, fmt.Errorf("解析大纲JSON失败: %w\n原始响应: %s", err, rawResp)
	}
	return &resp, nil
}

func generateOutline(ctx context.Context, apiCfg *config.APIConfig, cfg *config.Config, settings *ProjectSettings, logger *sse.LogBroadcaster) (*OutlineResponse, error) {
	chapterCountStr := fmt.Sprintf("%d", cfg.Story.ChapterCount)
	targetWordsStr := fmt.Sprintf("%d", cfg.Story.TargetWordsPerChapter)
	data := mergeOutlinePromptData(map[string]string{
		"StoryType":     cfg.Story.Type,
		"ChapterCount":  chapterCountStr,
		"TargetWords":   targetWordsStr,
		"WritingStyle":  cfg.Story.WritingStyle,
		"WritingPOV":    cfg.Story.WritingPOV,
		"StorySynopsis": cfg.Story.StorySynopsis,
	}, cfg, settings)

	systemPrompt := i18n.SystemPromptFor(cfg.Language, "outline_editor_json")
	minLen, _ := calcOutlineLengthRange(cfg.Story.TargetWordsPerChapter)

	var lastResp *OutlineResponse
	var lastShort []int
	for attempt := 0; attempt < outlineGenMaxAttempts; attempt++ {
		userPrompt := finalizeOutlinePrompt(cfg.Prompts.OutlineGeneration,
			config.RenderPrompt(cfg.Prompts.OutlineGeneration, data), cfg, settings)
		if attempt > 0 {
			userPrompt += formatShortOutlineRetryFeedback(lastShort, minLen, cfg.Language)
		}

		var rawResp string
		if logger != nil {
			rawResp = llm.CallAPIWithRetryLog(ctx, apiCfg, systemPrompt, userPrompt, logger)
		} else {
			rawResp = llm.CallAPIWithRetry(ctx, apiCfg, systemPrompt, userPrompt)
		}
		if rawResp == "" {
			return nil, fmt.Errorf("API 调用失败或被取消")
		}

		resp, err := parseOutlineResponse(rawResp)
		if err != nil {
			return nil, err
		}
		lastResp = resp
		lastShort = validateOutlineChapterLengths(resp.Chapters, minLen)
		if len(lastShort) == 0 {
			return resp, nil
		}
		if logger != nil {
			logger.WarnKey("log.outline_chapters_too_short", strings.Join(intSliceToStr(lastShort), ", "), minLen)
		}
	}

	if logger != nil && len(lastShort) > 0 {
		logger.WarnKey("log.outline_chapters_still_short", strings.Join(intSliceToStr(lastShort), ", "), minLen)
	}
	return lastResp, nil
}

func intSliceToStr(nums []int) []string {
	out := make([]string, len(nums))
	for i, n := range nums {
		out[i] = fmt.Sprintf("%d", n)
	}
	return out
}

func generateOutlineChaptersOnly(ctx context.Context, apiCfg *config.APIConfig, cfg *config.Config, settings *ProjectSettings, template string, baseData map[string]string, logger *sse.LogBroadcaster) ([]OutlineChapter, error) {
	data := mergeOutlinePromptData(baseData, cfg, settings)
	systemPrompt := i18n.SystemPromptFor(cfg.Language, "outline_editor_json")
	minLen, _ := calcOutlineLengthRange(cfg.Story.TargetWordsPerChapter)

	var lastChapters []OutlineChapter
	var lastShort []int
	for attempt := 0; attempt < outlineGenMaxAttempts; attempt++ {
		userPrompt := finalizeOutlinePrompt(template, config.RenderPrompt(template, data), cfg, settings)
		if attempt > 0 {
			userPrompt += formatShortOutlineRetryFeedback(lastShort, minLen, cfg.Language)
		}

		rawResp := llm.CallAPIWithRetryLog(ctx, apiCfg, systemPrompt, userPrompt, logger)
		if rawResp == "" {
			return nil, fmt.Errorf("API 调用失败或被取消")
		}

		var resp struct {
			Chapters []OutlineChapter `json:"chapters"`
		}
		rawResp = cleanJSONResponse(rawResp)
		if err := json.Unmarshal([]byte(rawResp), &resp); err != nil {
			return nil, fmt.Errorf("解析大纲JSON失败: %w\n原始响应: %s", err, rawResp)
		}
		lastChapters = resp.Chapters
		lastShort = validateOutlineChapterLengths(resp.Chapters, minLen)
		if len(lastShort) == 0 {
			return resp.Chapters, nil
		}
		logger.WarnKey("log.outline_chapters_too_short", strings.Join(intSliceToStr(lastShort), ", "), minLen)
	}

	if len(lastShort) > 0 {
		logger.WarnKey("log.outline_chapters_still_short", strings.Join(intSliceToStr(lastShort), ", "), minLen)
	}
	return lastChapters, nil
}

func reviseOutline(ctx context.Context, apiCfg *config.APIConfig, cfg *config.Config, state *Progress, settings *ProjectSettings, userFeedback, progressPath, cfgPath string, logger *sse.LogBroadcaster) error {
	lang := cfg.Language
	en := i18n.NormalizeLanguage(lang) == i18n.LangEN

	lockedChapters := ""
	for _, ch := range state.Chapters {
		if ch.Status == StatusAccepted {
			lockedChapters += formatChapterLine(ch.Num, ch.Title, ch.Outline, lang)
		}
	}
	if lockedChapters == "" {
		if en {
			lockedChapters = "(no locked chapters)"
		} else {
			lockedChapters = "无已锁定章节。"
		}
	}

	currentOutline := BatchSynopses(state, lang)
	for _, ch := range state.Chapters {
		currentOutline += formatChapterLine(ch.Num, ch.Title, ch.Outline, lang)
	}

	data := mergeOutlinePromptData(map[string]string{
		"CurrentOutline": currentOutline,
		"UserFeedback":   userFeedback,
		"LockedChapters": lockedChapters,
	}, cfg, settings)

	systemPrompt := i18n.SystemPromptFor(lang, "outline_editor_locked_json")
	minLen, _ := calcOutlineLengthRange(cfg.Story.TargetWordsPerChapter)

	var resp OutlineResponse
	var lastShort []int
	for attempt := 0; attempt < outlineGenMaxAttempts; attempt++ {
		userPrompt := finalizeOutlinePrompt(cfg.Prompts.OutlineRevision,
			config.RenderPrompt(cfg.Prompts.OutlineRevision, data), cfg, settings)
		if attempt > 0 {
			userPrompt += formatShortOutlineRetryFeedback(lastShort, minLen, lang)
		}

		rawResp := llm.CallAPIWithRetry(ctx, apiCfg, systemPrompt, userPrompt)
		if rawResp == "" {
			return fmt.Errorf("API 调用失败或被取消")
		}
		parsed, err := parseOutlineResponse(rawResp)
		if err != nil {
			return err
		}
		resp = *parsed
		lastShort = validateOutlineChapterLengths(resp.Chapters, minLen)
		if len(lastShort) == 0 {
			break
		}
		if logger != nil {
			logger.WarnKey("log.outline_chapters_too_short", strings.Join(intSliceToStr(lastShort), ", "), minLen)
		}
	}

	return applyOutlineRevision(cfg, state, resp, "outline_revision", PendingConfigChangesPath(progressPath), cfgPath, logger)
}

func applyOutlineRevision(cfg *config.Config, state *Progress, resp OutlineResponse, source, pendingPath, cfgPath string, logger *sse.LogBroadcaster) error {
	lockedMap := make(map[int]bool)
	for _, ch := range state.Chapters {
		if ch.Status == StatusAccepted {
			lockedMap[ch.Num] = true
		}
	}

	for _, newCh := range resp.Chapters {
		for i, existingCh := range state.Chapters {
			if existingCh.Num == newCh.Num && !lockedMap[newCh.Num] {
				state.Chapters[i].Title = newCh.Title
				state.Chapters[i].Outline = newCh.Outline
				state.Chapters[i].Characters = normalizeOutlineCharacters(newCh.Characters)
			}
		}
	}

	if len(state.OutlineBatches) > 0 {
		resp.StorySynopsis = ""
	}
	return applyOutlineMetaWithGuard(cfg, state, resp, source, pendingPath, cfgPath, logger)
}

func Truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}

func cleanJSONResponse(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```json") {
		s = strings.TrimPrefix(s, "```json")
	} else if strings.HasPrefix(s, "```") {
		s = strings.TrimPrefix(s, "```")
	}
	if strings.HasSuffix(s, "```") {
		s = strings.TrimSuffix(s, "```")
	}
	return strings.TrimSpace(s)
}

func GenerateOutlineAction(ctx context.Context, apiCfg *config.APIConfig, cfg *config.Config, state *Progress, settings *ProjectSettings, progressPath, cfgPath string, logger *sse.LogBroadcaster) error {
	if err := llm.ValidateConfig(apiCfg); err != nil {
		return err
	}
	for _, ch := range state.Chapters {
		if ch.Status == StatusAccepted {
			return fmt.Errorf("存在已确认章节，无法整体重新生成大纲（会覆盖已完成内容）。如需追加章节请使用「生成后续大纲」")
		}
	}

	logger.StepInfo(1, 2, "正在调用 AI 生成大纲...")

	outlineResp, err := generateOutline(ctx, apiCfg, cfg, settings, logger)
	if err != nil {
		return fmt.Errorf("生成大纲失败: %w", err)
	}

	logger.StepInfo(2, 2, "正在保存大纲...")

	state.Chapters = make([]ChapterState, len(outlineResp.Chapters))
	for i, ch := range outlineResp.Chapters {
		state.Chapters[i] = chapterStateFromOutline(ch, StatusPending)
	}

	if err := applyOutlineMetaWithGuard(cfg, state, *outlineResp, "outline_generation", PendingConfigChangesPath(progressPath), cfgPath, logger); err != nil {
		return err
	}

	snapshot := cfg.Story
	state.StoryConfigSnapshot = &snapshot

	if err := SaveProgress(progressPath, state); err != nil {
		return fmt.Errorf("保存进度失败: %w", err)
	}

	runOutlinePostProcessChecks(ctx, apiCfg, cfg, state, settings, progressPath, logger)

	logger.SuccessKey("log.outline_generate_summary", len(state.Chapters), state.Title)
	return nil
}

func ReviseOutlineAction(ctx context.Context, apiCfg *config.APIConfig, cfg *config.Config, state *Progress, settings *ProjectSettings, progressPath, cfgPath, feedback string, logger *sse.LogBroadcaster) error {
	logger.StepInfo(1, 2, "正在根据意见修订大纲...")

	if err := reviseOutline(ctx, apiCfg, cfg, state, settings, feedback, progressPath, cfgPath, logger); err != nil {
		return fmt.Errorf("修订大纲失败: %w", err)
	}

	logger.StepInfo(2, 2, "正在保存修订后的大纲...")

	if err := SaveProgress(progressPath, state); err != nil {
		return fmt.Errorf("保存进度失败: %w", err)
	}

	runOutlinePostProcessChecks(ctx, apiCfg, cfg, state, settings, progressPath, logger)

	logger.SuccessKey("log.outline_revise_summary", len(state.Chapters))
	return nil
}

func ConfirmOutlineAction(state *Progress, progressPath string) error {
	if len(state.Chapters) == 0 {
		return fmt.Errorf("大纲为空")
	}

	state.Phase = "writing"
	return SaveProgress(progressPath, state)
}

func outlineEditable(status string) bool {
	switch status {
	case StatusPending, StatusWriting, StatusReview:
		return true
	default:
		return false
	}
}

// EditChapterOutline updates title/outline. If characters != nil, replaces the
// structured cast; if nil, leaves existing Characters unchanged.
func EditChapterOutline(state *Progress, chapterNum int, title, outline string, characters *[]OutlineChapterCharacter) error {
	idx := -1
	for i, ch := range state.Chapters {
		if ch.Num == chapterNum {
			idx = i
			break
		}
	}
	if idx == -1 {
		return fmt.Errorf("章节 %d 不存在", chapterNum)
	}
	if !outlineEditable(state.Chapters[idx].Status) {
		return fmt.Errorf("只能编辑待定/写作中/审核中章节的大纲（已确认章节不可改）")
	}
	state.Chapters[idx].Title = title
	state.Chapters[idx].Outline = outline
	if characters != nil {
		state.Chapters[idx].Characters = normalizeOutlineCharacters(*characters)
	}
	return nil
}

// Continuation outline generation for imported / finished books.
// The v3 import pipeline itself lives in importer.go.

// ContinuationOutlineAllowed gates POST /api/outline/generate-continuation.
// Batch planning accepts empty projects and projects in outline/writing phases.
func ContinuationOutlineAllowed(phase string, chapterCount int) bool {
	return phase == "" || phase == "outline" || phase == "writing"
}
