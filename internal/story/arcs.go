package story

// v3 hierarchical outline: books are structured as arcs (volumes). The
// skeleton (arc list) is generated in one small call; chapter outlines are
// then generated arc by arc, so 1000+ chapter books never need one giant
// outline call. Completed arcs are compressed into arc summaries that replace
// per-chapter context in later prompts.

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

// —— helpers ——

func ArcIndexByID(state *Progress, id int) int {
	for i := range state.Arcs {
		if state.Arcs[i].ID == id {
			return i
		}
	}
	return -1
}

// arcForChapterNum returns the arc containing the given chapter number.
func arcForChapterNum(state *Progress, num int) *Arc {
	for i := range state.Arcs {
		if num >= state.Arcs[i].StartCh && num <= state.Arcs[i].EndCh {
			return &state.Arcs[i]
		}
	}
	return nil
}

// arcChapters returns the chapters of state that fall inside the arc range.
func arcChapters(state *Progress, arc *Arc) []*ChapterState {
	var out []*ChapterState
	for i := range state.Chapters {
		if state.Chapters[i].Num >= arc.StartCh && state.Chapters[i].Num <= arc.EndCh {
			out = append(out, &state.Chapters[i])
		}
	}
	return out
}

// arcCompleted reports whether every chapter in the arc range is accepted.
func arcCompleted(state *Progress, arc *Arc) bool {
	chs := arcChapters(state, arc)
	if len(chs) != arc.EndCh-arc.StartCh+1 {
		return false
	}
	for _, ch := range chs {
		if ch.Status != StatusAccepted {
			return false
		}
	}
	return true
}

// buildPreviousArcContext assembles the "previously" block for arc-level
// outline generation: summaries of completed arcs plus the outlines of the
// tail chapters right before startCh (for a tight handoff).
func buildPreviousArcContext(state *Progress, startCh int, lang string) string {
	en := i18n.NormalizeLanguage(lang) == i18n.LangEN
	var sb strings.Builder

	for i := range state.Arcs {
		arc := &state.Arcs[i]
		if arc.EndCh >= startCh || arc.Summary == "" {
			continue
		}
		if en {
			sb.WriteString(fmt.Sprintf("[Arc %d \"%s\" (ch.%d-%d) summary]\n%s\n\n", i+1, arc.Title, arc.StartCh, arc.EndCh, arc.Summary))
		} else {
			sb.WriteString(fmt.Sprintf("【第%d卷《%s》（第%d~%d章）卷摘要】\n%s\n\n", i+1, arc.Title, arc.StartCh, arc.EndCh, arc.Summary))
		}
	}

	// Tail chapters of the previous arc: prefer summaries, fall back to outlines.
	const tailWindow = 5
	var tail []string
	for i := range state.Chapters {
		ch := state.Chapters[i]
		if ch.Num >= startCh || ch.Num < startCh-tailWindow {
			continue
		}
		if ch.Summary != "" {
			if en {
				tail = append(tail, fmt.Sprintf("[Chapter %d summary] %s", ch.Num, ch.Summary))
			} else {
				tail = append(tail, fmt.Sprintf("[第%d章摘要] %s", ch.Num, ch.Summary))
			}
		} else if strings.TrimSpace(ch.Outline) != "" {
			tail = append(tail, strings.TrimRight(formatChapterLine(ch.Num, ch.Title, ch.Outline, lang), "\n"))
		}
	}
	if len(tail) > 0 {
		sb.WriteString(strings.Join(tail, "\n"))
		sb.WriteString("\n")
	}

	if sb.Len() == 0 {
		if en {
			return "This is the opening of the story; no prior context."
		}
		return "当前为故事开端，无历史前情。"
	}
	return sb.String()
}

// buildFutureArcsBlock lists the goals of arcs after the given arc index.
func buildFutureArcsBlock(state *Progress, arcIdx int, lang string) string {
	en := i18n.NormalizeLanguage(lang) == i18n.LangEN
	var sb strings.Builder
	for i := arcIdx + 1; i < len(state.Arcs); i++ {
		arc := state.Arcs[i]
		if en {
			sb.WriteString(fmt.Sprintf("Arc %d \"%s\" (ch.%d-%d): %s\n", i+1, arc.Title, arc.StartCh, arc.EndCh, arc.Goal))
		} else {
			sb.WriteString(fmt.Sprintf("第%d卷《%s》（第%d~%d章）：%s\n", i+1, arc.Title, arc.StartCh, arc.EndCh, arc.Goal))
		}
	}
	if sb.Len() == 0 {
		if en {
			return "(this is the final arc)"
		}
		return "（本卷为最后一卷）"
	}
	return sb.String()
}

// —— skeleton generation ——

type arcSkeletonResponse struct {
	Title         string `json:"title"`
	StorySynopsis string `json:"story_synopsis"`
	Arcs          []struct {
		Title        string `json:"title"`
		Goal         string `json:"goal"`
		ChapterCount int    `json:"chapter_count"`
	} `json:"arcs"`
}

// assignArcRanges converts per-arc chapter counts into [start,end] ranges
// starting at startCh, forcing the total to equal totalChapters (the last arc
// absorbs any drift; zero/negative counts get a minimum of 1).
func assignArcRanges(counts []int, startCh, totalChapters int) [](struct{ Start, End int }) {
	out := make([]struct{ Start, End int }, len(counts))
	if len(counts) == 0 {
		return out
	}
	sum := 0
	for i, c := range counts {
		if c < 1 {
			counts[i] = 1
			c = 1
		}
		sum += c
	}
	// ponytail: proportional rebalance would be fancier; clamping the last arc
	// is enough because the prompt demands exact totals and drift is rare.
	drift := totalChapters - sum
	counts[len(counts)-1] += drift
	if counts[len(counts)-1] < 1 {
		counts[len(counts)-1] = 1
	}
	cur := startCh
	for i, c := range counts {
		out[i] = struct{ Start, End int }{cur, cur + c - 1}
		cur += c
	}
	return out
}

// GenerateArcSkeletonAction generates the volume-level skeleton. Refuses when
// accepted chapters exist (same protection as full outline regeneration);
// pending chapters are dropped because they belong to the old structure.
func GenerateArcSkeletonAction(ctx context.Context, apiCfg *config.APIConfig, cfg *config.Config, state *Progress, settings *ProjectSettings, progressPath, cfgPath string, logger *sse.LogBroadcaster) error {
	if err := llm.ValidateConfig(apiCfg); err != nil {
		return err
	}
	for _, ch := range state.Chapters {
		if ch.Status == StatusAccepted || ch.Status == StatusWriting || ch.Status == StatusReview {
			return fmt.Errorf("存在已确认或写作中的章节，无法重新生成卷骨架。如需追加内容请使用「追加新卷」")
		}
	}

	data := mergeOutlinePromptData(map[string]string{
		"StoryType":     cfg.Story.Type,
		"ChapterCount":  fmt.Sprintf("%d", cfg.Story.ChapterCount),
		"TargetWords":   fmt.Sprintf("%d", cfg.Story.TargetWordsPerChapter),
		"WritingStyle":  cfg.Story.WritingStyle,
		"WritingPOV":    cfg.Story.WritingPOV,
		"StorySynopsis": cfg.Story.StorySynopsis,
	}, cfg, settings)
	userPrompt := config.RenderPrompt(cfg.Prompts.ArcSkeleton, data)
	userPrompt = appendIfMissingPlaceholder(cfg.Prompts.ArcSkeleton, userPrompt, "{{.CharacterList}}", formatCharacterListForOutline(settings, cfg.Language))
	systemPrompt := i18n.SystemPromptFor(cfg.Language, "outline_editor_json")

	rawResp := llm.CallAPIWithRetryLog(ctx, apiCfg, systemPrompt, userPrompt, logger)
	if rawResp == "" {
		return fmt.Errorf("API 调用失败或被取消")
	}
	var resp arcSkeletonResponse
	if err := json.Unmarshal([]byte(cleanJSONResponse(rawResp)), &resp); err != nil {
		return fmt.Errorf("解析卷骨架JSON失败: %w\n原始响应: %s", err, Truncate(rawResp, 500))
	}
	if len(resp.Arcs) == 0 {
		return fmt.Errorf("AI 未返回任何卷")
	}

	counts := make([]int, len(resp.Arcs))
	for i, a := range resp.Arcs {
		counts[i] = a.ChapterCount
	}
	ranges := assignArcRanges(counts, 1, cfg.Story.ChapterCount)

	state.Arcs = make([]Arc, len(resp.Arcs))
	for i, a := range resp.Arcs {
		state.Arcs[i] = Arc{
			ID:      i + 1,
			Title:   a.Title,
			Goal:    a.Goal,
			StartCh: ranges[i].Start,
			EndCh:   ranges[i].End,
		}
	}
	// Drop pending chapters from the previous structure.
	state.Chapters = nil
	state.CurrentChapterIndex = 0

	if err := applyOutlineMetaWithGuard(cfg, state, OutlineResponse{Title: resp.Title, StorySynopsis: resp.StorySynopsis}, "arc_skeleton", PendingConfigChangesPath(progressPath), cfgPath, logger); err != nil {
		return err
	}
	snapshot := cfg.Story
	state.StoryConfigSnapshot = &snapshot

	if err := SaveProgress(progressPath, state); err != nil {
		return fmt.Errorf("保存进度失败: %w", err)
	}
	logger.SuccessKey("log.arc_skeleton_done", len(state.Arcs), cfg.Story.ChapterCount)
	return nil
}

// —— per-arc chapter outline generation ——

// GenerateArcOutlineAction generates chapter outlines for one arc. Pending
// chapters inside the range are replaced; accepted ones are refused.
func GenerateArcOutlineAction(ctx context.Context, apiCfg *config.APIConfig, cfg *config.Config, state *Progress, settings *ProjectSettings, arcID int, requirements, progressPath string, logger *sse.LogBroadcaster) error {
	if err := llm.ValidateConfig(apiCfg); err != nil {
		return err
	}
	ai := ArcIndexByID(state, arcID)
	if ai < 0 {
		return fmt.Errorf("卷 %d 不存在", arcID)
	}
	arc := &state.Arcs[ai]
	for _, ch := range arcChapters(state, arc) {
		if ch.Status != StatusPending {
			return fmt.Errorf("第 %d 卷范围内存在已确认/写作中的章节，无法重新生成本卷章纲", ai+1)
		}
	}

	lang := cfg.Language
	count := arc.EndCh - arc.StartCh + 1
	baseData := map[string]string{
		"Title":            preferUserValue(cfg.Story.Title, state.Title),
		"StoryType":        cfg.Story.Type,
		"CorePrompt":       state.CorePrompt,
		"StorySynopsis":    preferUserValue(cfg.Story.StorySynopsis, state.StorySynopsis),
		"WritingStyle":     cfg.Story.WritingStyle,
		"WritingPOV":       cfg.Story.WritingPOV,
		"PreviousContext":  buildPreviousArcContext(state, arc.StartCh, lang),
		"ArcIndex":         fmt.Sprintf("%d", ai+1),
		"ArcTitle":         arc.Title,
		"ArcGoal":          arc.Goal,
		"FutureArcs":       buildFutureArcsBlock(state, ai, lang),
		"UserRequirements": strings.TrimSpace(requirements),
		"NewChapterCount":  fmt.Sprintf("%d", count),
		"StartNum":         fmt.Sprintf("%d", arc.StartCh),
		"EndNum":           fmt.Sprintf("%d", arc.EndCh),
	}
	if baseData["UserRequirements"] == "" {
		if i18n.NormalizeLanguage(lang) == i18n.LangEN {
			baseData["UserRequirements"] = "(none)"
		} else {
			baseData["UserRequirements"] = "（无）"
		}
	}

	chapters, err := generateOutlineChaptersOnly(ctx, apiCfg, cfg, settings, cfg.Prompts.ArcChapterOutline, baseData, logger)
	if err != nil {
		return fmt.Errorf("生成第 %d 卷章纲失败: %w", ai+1, err)
	}
	if len(chapters) == 0 {
		return fmt.Errorf("AI 未返回任何章节")
	}

	// Renumber defensively into the arc range, then replace pending chapters
	// in range and keep everything else ordered by Num.
	kept := state.Chapters[:0]
	for _, ch := range state.Chapters {
		if ch.Num < arc.StartCh || ch.Num > arc.EndCh {
			kept = append(kept, ch)
		}
	}
	for i, oc := range chapters {
		num := arc.StartCh + i
		if num > arc.EndCh {
			break
		}
		kept = append(kept, ChapterState{
			Num:     num,
			Title:   oc.Title,
			Outline: oc.Outline,
			Status:  StatusPending,
		})
	}
	sortChaptersByNum(kept)
	state.Chapters = kept

	if err := SaveProgress(progressPath, state); err != nil {
		return fmt.Errorf("保存进度失败: %w", err)
	}
	runOutlinePostProcessChecks(ctx, apiCfg, cfg, state, settings, progressPath, logger)
	logger.SuccessKey("log.arc_outline_done", ai+1, len(chapters))
	return nil
}

func sortChaptersByNum(chs []ChapterState) {
	// ponytail: insertion sort — chapter lists are already nearly sorted.
	for i := 1; i < len(chs); i++ {
		for j := i; j > 0 && chs[j].Num < chs[j-1].Num; j-- {
			chs[j], chs[j-1] = chs[j-1], chs[j]
		}
	}
}

// —— append a new arc (incremental continuation) ——

// AppendArcAction adds a new arc after the last one and generates its chapter
// outlines in the same task. This is the incremental path for endless serials.
func AppendArcAction(ctx context.Context, apiCfg *config.APIConfig, cfg *config.Config, state *Progress, settings *ProjectSettings, title, goal string, chapterCount int, progressPath string, logger *sse.LogBroadcaster) error {
	if chapterCount <= 0 {
		chapterCount = 20
	}
	startCh := 1
	if len(state.Arcs) > 0 {
		startCh = state.Arcs[len(state.Arcs)-1].EndCh + 1
	} else if len(state.Chapters) > 0 {
		startCh = state.Chapters[len(state.Chapters)-1].Num + 1
	}
	maxID := 0
	for _, a := range state.Arcs {
		if a.ID > maxID {
			maxID = a.ID
		}
	}
	arc := Arc{
		ID:      maxID + 1,
		Title:   strings.TrimSpace(title),
		Goal:    strings.TrimSpace(goal),
		StartCh: startCh,
		EndCh:   startCh + chapterCount - 1,
	}
	if arc.Title == "" {
		arc.Title = fmt.Sprintf("第%d卷", len(state.Arcs)+1)
	}
	state.Arcs = append(state.Arcs, arc)

	if err := GenerateArcOutlineAction(ctx, apiCfg, cfg, state, settings, arc.ID, goal, progressPath, logger); err != nil {
		// Roll back the appended arc so a failed generation leaves no husk.
		state.Arcs = state.Arcs[:len(state.Arcs)-1]
		return err
	}
	return nil
}

// —— arc summaries ——

// EnsureArcSummaries generates missing summaries for completed arcs. Called
// lazily from GenerateChapterAction so it always runs inside a task context.
// Failures only log — the book can proceed with per-chapter summaries.
func EnsureArcSummaries(ctx context.Context, apiCfg *config.APIConfig, cfg *config.Config, state *Progress, progressPath string, logger *sse.LogBroadcaster) {
	for i := range state.Arcs {
		arc := &state.Arcs[i]
		if arc.Summary != "" || !arcCompleted(state, arc) {
			continue
		}
		if ctx.Err() != nil {
			return
		}
		var sums strings.Builder
		for _, ch := range arcChapters(state, arc) {
			if ch.Summary != "" {
				sums.WriteString(fmt.Sprintf("[%d] %s\n", ch.Num, ch.Summary))
			}
		}
		if sums.Len() == 0 {
			continue
		}
		logger.InfoKey("log.arc_summary_generating", i+1)
		userPrompt := config.RenderPrompt(cfg.Prompts.ArcSummary, map[string]string{
			"Title":            preferUserValue(cfg.Story.Title, state.Title),
			"ArcIndex":         fmt.Sprintf("%d", i+1),
			"ArcTitle":         arc.Title,
			"ArcGoal":          arc.Goal,
			"StartNum":         fmt.Sprintf("%d", arc.StartCh),
			"EndNum":           fmt.Sprintf("%d", arc.EndCh),
			"ChapterSummaries": sums.String(),
		})
		resp, err := llm.CallAPI(ctx, apiCfg, i18n.SystemPromptFor(cfg.Language, "author_default"), userPrompt)
		if err != nil || strings.TrimSpace(resp) == "" {
			logger.WarnKey("log.arc_summary_failed", i+1, err)
			continue
		}
		arc.Summary = strings.TrimSpace(resp)
		if err := SaveProgress(progressPath, state); err != nil {
			logger.WarnKey("log.arc_save_failed", err)
		}
		logger.SuccessKey("log.arc_summary_done", i+1)
	}
}
