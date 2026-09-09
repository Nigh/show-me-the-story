package story

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"showmethestory/internal/config"
	"showmethestory/internal/i18n"
	"showmethestory/internal/llm"
	"showmethestory/internal/sse"
	"strings"
	"time"
)

type ForeshadowSuggestion struct {
	Name          string `json:"name"`
	Description   string `json:"description"`
	PlantChapter  int    `json:"plant_chapter"`
	TargetChapter int    `json:"target_chapter"`
}

type ForeshadowPlanResponse struct {
	Foreshadows []ForeshadowSuggestion `json:"foreshadows"`
}

type ForeshadowUpdateItem struct {
	ID         int    `json:"id"`
	Status     string `json:"status"`
	Event      string `json:"event"`
	Resolution string `json:"resolution"`
}

type ForeshadowUpdateResponse struct {
	Updates []ForeshadowUpdateItem `json:"updates"`
}

func SuggestForeshadows(ctx context.Context, apiCfg *config.APIConfig, cfg *config.Config, state *Progress, logger *sse.LogBroadcaster) ([]ForeshadowSuggestion, error) {
	lang := cfg.Language
	outline := BuildPlanningHistory(state, state.LongTermDirection, lang)

	userPrompt := config.RenderPrompt(cfg.Prompts.ForeshadowPlanning, map[string]string{
		"Title":         state.Title,
		"CorePrompt":    state.CorePrompt,
		"StorySynopsis": state.LongTermDirection,
		"Outline":       outline,
	})

	systemPrompt := i18n.SystemPromptFor(lang, "narrative_architect_json")

	rawResp := llm.CallAPIWithRetryLog(ctx, apiCfg, systemPrompt, userPrompt, logger)
	if rawResp == "" {
		return nil, fmt.Errorf("API 调用失败或被取消")
	}
	rawResp = cleanJSONResponse(rawResp)

	var resp ForeshadowPlanResponse
	if err := json.Unmarshal([]byte(rawResp), &resp); err != nil {
		return nil, fmt.Errorf("解析伏笔建议JSON失败: %w", err)
	}

	logger.InfoKey("log.foreshadow_plan_parsed", len(resp.Foreshadows))
	return resp.Foreshadows, nil
}

func UpdateForeshadows(ctx context.Context, apiCfg *config.APIConfig, cfg *config.Config, state *Progress, chapterIdx int, logger *sse.LogBroadcaster) error {
	ch := state.Chapters[chapterIdx]
	lang := cfg.Language

	foreshadowsText := formatForeshadowsForPromptLang(state.Foreshadows, lang)
	if foreshadowsText == "无" || foreshadowsText == "(none)" {
		return nil
	}

	historySummary := buildHistorySummaryForLang(state, chapterIdx, lang)

	userPrompt := config.RenderPrompt(cfg.Prompts.ForeshadowUpdate, map[string]string{
		"Title":          state.Title,
		"ChapterNum":     fmt.Sprintf("%d", ch.Num),
		"ChapterTitle":   ch.Title,
		"ChapterContent": ch.Content,
		"HistorySummary": historySummary,
		"Foreshadows":    foreshadowsText,
	})

	systemPrompt := i18n.SystemPromptFor(lang, "foreshadow_tracker_json")

	rawResp := llm.CallAPIWithRetryLog(ctx, apiCfg, systemPrompt, userPrompt, logger)
	if rawResp == "" {
		return fmt.Errorf("API 调用失败或被取消")
	}
	rawResp = cleanJSONResponse(rawResp)

	var resp ForeshadowUpdateResponse
	if err := json.Unmarshal([]byte(rawResp), &resp); err != nil {
		return fmt.Errorf("解析伏笔更新JSON失败: %w", err)
	}

	applyForeshadowUpdates(state, resp.Updates, ch.Num)
	logger.InfoKey("log.foreshadow_status_updated", len(resp.Updates))
	return nil
}

func applyForeshadowUpdates(state *Progress, updates []ForeshadowUpdateItem, chapterNum int) {
	updateMap := make(map[int]ForeshadowUpdateItem)
	for _, u := range updates {
		updateMap[u.ID] = u
	}

	for i := range state.Foreshadows {
		fs := &state.Foreshadows[i]
		u, ok := updateMap[fs.ID]
		if !ok {
			continue
		}

		if u.Event != "" {
			fs.Events = append(fs.Events, ForeshadowEvent{
				Chapter: chapterNum,
				Note:    u.Event,
			})
		}

		if u.Status != "" {
			fs.Status = ForeshadowStatus(u.Status)
		}

		if u.Resolution != "" {
			fs.Resolution = u.Resolution
		}
	}
}

func BuildForeshadowWarnings(state *Progress) string {
	if len(state.Foreshadows) == 0 {
		return ""
	}

	currentChapter := state.CurrentChapterIndex + 1
	var warnings []string

	for _, fs := range state.Foreshadows {
		if fs.Status == ForeshadowResolved || fs.Status == ForeshadowAbandoned {
			continue
		}
		if fs.TargetChapter > 0 && currentChapter > fs.TargetChapter+3 {
			warnings = append(warnings, fmt.Sprintf("伏笔 #%d \"%s\" 已超过预计回收章节 %d 章以上", fs.ID, fs.Name, fs.TargetChapter))
		}
	}

	if len(warnings) == 0 {
		return ""
	}

	return "⚠️ 伏笔超期告警: " + strings.Join(warnings, "；")
}

func NextForeshadowID(foreshadows []Foreshadow) int {
	maxID := 0
	for _, fs := range foreshadows {
		if fs.ID > maxID {
			maxID = fs.ID
		}
	}
	return maxID + 1
}

func ForeshadowStatusLabel(status ForeshadowStatus) string {
	switch status {
	case ForeshadowPlanted:
		return "已埋设"
	case ForeshadowProgressing:
		return "推进中"
	case ForeshadowResolved:
		return "已回收"
	case ForeshadowAbandoned:
		return "已放弃"
	default:
		return string(status)
	}
}

func maxChapterNum(state *Progress) int {
	maxNum := 0
	for _, ch := range state.Chapters {
		if ch.Num > maxNum {
			maxNum = ch.Num
		}
	}
	for _, fs := range state.Foreshadows {
		if fs.PlantChapter > maxNum {
			maxNum = fs.PlantChapter
		}
		if fs.TargetChapter > maxNum {
			maxNum = fs.TargetChapter
		}
		for _, ev := range fs.Events {
			if ev.Chapter > maxNum {
				maxNum = ev.Chapter
			}
		}
	}
	return maxNum
}

// BuildForeshadowRoadmapMarkdown 生成供用户阅读的伏笔路线图 Markdown。
func BuildForeshadowRoadmapMarkdown(state *Progress) string {
	title := state.Title
	if title == "" {
		title = "未命名小说"
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# 伏笔路线图 — 《%s》\n\n", title))
	sb.WriteString(fmt.Sprintf("> 更新时间：%s\n\n", time.Now().Format("2006-01-02 15:04:05")))

	if len(state.Foreshadows) == 0 {
		sb.WriteString("当前尚无伏笔记录。\n")
		return sb.String()
	}

	active, resolved, abandoned := 0, 0, 0
	for _, fs := range state.Foreshadows {
		switch fs.Status {
		case ForeshadowPlanted, ForeshadowProgressing:
			active++
		case ForeshadowResolved:
			resolved++
		case ForeshadowAbandoned:
			abandoned++
		}
	}

	sb.WriteString("## 概览\n\n")
	sb.WriteString(fmt.Sprintf("- 总计 **%d** 条 | 活跃 **%d** | 已回收 **%d** | 已放弃 **%d**\n\n", len(state.Foreshadows), active, resolved, abandoned))

	if warn := BuildForeshadowWarnings(state); warn != "" {
		sb.WriteString("## 超期告警\n\n")
		sb.WriteString(strings.TrimPrefix(warn, "⚠️ 伏笔超期告警: "))
		sb.WriteString("\n\n")
	}

	maxCh := maxChapterNum(state)
	if maxCh > 0 {
		sb.WriteString("## 按章节时间线\n\n")
		for chNum := 1; chNum <= maxCh; chNum++ {
			var lines []string
			for _, fs := range state.Foreshadows {
				if fs.PlantChapter == chNum {
					lines = append(lines, fmt.Sprintf("- 🔵 **#%d %s** — 埋设（%s）", fs.ID, fs.Name, ForeshadowStatusLabel(fs.Status)))
				}
				if fs.TargetChapter == chNum {
					lines = append(lines, fmt.Sprintf("- 🎯 **#%d %s** — 预计回收（%s）", fs.ID, fs.Name, ForeshadowStatusLabel(fs.Status)))
				}
				for _, ev := range fs.Events {
					if ev.Chapter == chNum {
						lines = append(lines, fmt.Sprintf("- 📌 **#%d %s** — %s", fs.ID, fs.Name, ev.Note))
					}
				}
			}
			if len(lines) == 0 {
				continue
			}
			sb.WriteString(fmt.Sprintf("### 第 %d 章\n\n", chNum))
			sb.WriteString(strings.Join(lines, "\n"))
			sb.WriteString("\n\n")
		}
	}

	sb.WriteString("## 伏笔详情\n\n")
	for _, fs := range state.Foreshadows {
		sb.WriteString(fmt.Sprintf("### #%d %s [%s]\n\n", fs.ID, fs.Name, ForeshadowStatusLabel(fs.Status)))
		sb.WriteString(fmt.Sprintf("%s\n\n", fs.Description))
		sb.WriteString(fmt.Sprintf("- 埋设章节：第 **%d** 章\n", fs.PlantChapter))
		if fs.TargetChapter > 0 {
			sb.WriteString(fmt.Sprintf("- 预计回收：第 **%d** 章\n", fs.TargetChapter))
		}
		if len(fs.Events) > 0 {
			sb.WriteString("- 进展记录：\n")
			for _, ev := range fs.Events {
				sb.WriteString(fmt.Sprintf("  - 第 %d 章：%s\n", ev.Chapter, ev.Note))
			}
		}
		if fs.Resolution != "" {
			sb.WriteString(fmt.Sprintf("- 回收方式：%s\n", fs.Resolution))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// SaveForeshadowRoadmap 将伏笔路线图写入项目目录 Foreshadows.md。
func SaveForeshadowRoadmap(projectDir string, state *Progress) error {
	content := BuildForeshadowRoadmapMarkdown(state)
	return os.WriteFile(ForeshadowRoadmapPath(projectDir), []byte(content), 0644)
}

// syncForeshadowsAfterChapter 在章节正文变更后更新伏笔状态并落盘路线图。
func syncForeshadowsAfterChapter(ctx context.Context, apiCfg *config.APIConfig, cfg *config.Config, state *Progress, chapterIdx int, progressPath string, logger *sse.LogBroadcaster) {
	if len(state.Foreshadows) == 0 {
		return
	}
	if err := UpdateForeshadows(ctx, apiCfg, cfg, state, chapterIdx, logger); err != nil {
		logger.WarnKey("log.foreshadow_sync_failed", err)
		return
	}
	active, resolved := 0, 0
	for _, fs := range state.Foreshadows {
		switch fs.Status {
		case ForeshadowPlanted, ForeshadowProgressing:
			active++
		case ForeshadowResolved:
			resolved++
		}
	}
	logger.InfoKey("log.foreshadow_sync_summary", active, resolved)
	if err := SaveForeshadowRoadmap(filepath.Dir(progressPath), state); err != nil {
		logger.WarnKey("log.foreshadow_roadmap_save_failed", err)
	}
	if warn := BuildForeshadowWarnings(state); warn != "" {
		logger.Warn(warn)
	}
}

func buildFullOutlineText(state *Progress, lang string) string {
	return BuildPlanningHistory(state, formatForeshadowsForPromptLang(state.Foreshadows, lang), lang)
}

func buildAcceptedSummariesText(state *Progress, lang string) string {
	return buildHistorySummaryForLang(state, len(state.Chapters), lang)
}

func CheckForeshadowOutlineConsistency(ctx context.Context, apiCfg *config.APIConfig, cfg *config.Config, state *Progress, logger *sse.LogBroadcaster) (*ForeshadowOutlineReport, error) {
	if len(state.Foreshadows) == 0 {
		return &ForeshadowOutlineReport{Summary: "无伏笔"}, nil
	}

	lang := cfg.Language
	userPrompt := config.RenderPrompt(cfg.Prompts.ForeshadowOutlineConsistency, map[string]string{
		"Title":             preferUserValue(cfg.Story.Title, state.Title),
		"Outline":           buildFullOutlineText(state, lang),
		"Foreshadows":       formatForeshadowsForPromptLang(state.Foreshadows, lang),
		"AcceptedSummaries": buildAcceptedSummariesText(state, lang),
	})
	systemPrompt := i18n.SystemPromptFor(lang, "foreshadow_outline_checker_json")

	rawResp := llm.CallAPIWithRetryLog(ctx, apiCfg, systemPrompt, userPrompt, logger)
	if rawResp == "" {
		return nil, fmt.Errorf("API 调用失败或被取消")
	}

	var report ForeshadowOutlineReport
	jsonStr := llm.ExtractJSON(cleanJSONResponse(rawResp))
	if jsonStr == "" {
		return nil, fmt.Errorf("无法解析伏笔-大纲一致性检查结果")
	}
	if err := json.Unmarshal([]byte(jsonStr), &report); err != nil {
		return nil, fmt.Errorf("解析伏笔-大纲一致性检查JSON失败: %w", err)
	}
	return &report, nil
}

func applyForeshadowOutlineReport(state *Progress, report *ForeshadowOutlineReport) {
	if report == nil {
		return
	}
	state.LastForeshadowOutlineReport = report
}

func RunForeshadowOutlineCheckAndSave(ctx context.Context, apiCfg *config.APIConfig, cfg *config.Config, state *Progress, progressPath string, logger *sse.LogBroadcaster) error {
	if len(state.Foreshadows) == 0 {
		return nil
	}
	report, err := CheckForeshadowOutlineConsistency(ctx, apiCfg, cfg, state, logger)
	if err != nil {
		logger.WarnKey("log.foreshadow_outline_check_failed", err)
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	before := state.LastForeshadowOutlineReport
	applyForeshadowOutlineReport(state, report)
	if err := SaveProgress(progressPath, state); err != nil {
		state.LastForeshadowOutlineReport = before
		logger.ErrorKey("log.foreshadow_outline_report_save_failed", err)
		return err
	}
	if report.HasConflicts {
		logger.ForeshadowOutlineConflicts(report)
	} else {
		logger.InfoKey("log.foreshadow_outline_check_pass")
	}
	return nil
}
