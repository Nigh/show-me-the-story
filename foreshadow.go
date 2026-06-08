package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
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

func SuggestForeshadows(ctx context.Context, apiCfg *APIConfig, cfg *Config, state *Progress, logger *LogBroadcaster) ([]ForeshadowSuggestion, error) {
	outline := ""
	for _, ch := range state.Chapters {
		outline += fmt.Sprintf("第%d章《%s》: %s\n", ch.Num, ch.Title, ch.Outline)
	}

	userPrompt := RenderPrompt(cfg.Prompts.ForeshadowPlanning, map[string]string{
		"Title":         state.Title,
		"CorePrompt":    state.CorePrompt,
		"StorySynopsis": state.StorySynopsis,
		"Outline":       outline,
	})

	systemPrompt := "你是一位资深的小说叙事架构师。请严格按照要求的JSON格式输出，不要添加任何额外文字或markdown代码块标记。"

	rawResp := CallAPIWithRetryLog(ctx, apiCfg, systemPrompt, userPrompt, logger)
	if rawResp == "" {
		return nil, fmt.Errorf("API 调用失败或被取消")
	}
	rawResp = cleanJSONResponse(rawResp)

	var resp ForeshadowPlanResponse
	if err := json.Unmarshal([]byte(rawResp), &resp); err != nil {
		return nil, fmt.Errorf("解析伏笔建议JSON失败: %w", err)
	}

	logger.Info(fmt.Sprintf("伏笔方案解析完成，共 %d 条", len(resp.Foreshadows)))
	return resp.Foreshadows, nil
}

func UpdateForeshadows(ctx context.Context, apiCfg *APIConfig, cfg *Config, state *Progress, chapterIdx int, logger *LogBroadcaster) error {
	ch := state.Chapters[chapterIdx]

	foreshadowsText := formatForeshadowsForPrompt(state.Foreshadows)
	if foreshadowsText == "无" {
		return nil
	}

	historySummary := buildHistorySummary(state, chapterIdx)

	userPrompt := RenderPrompt(cfg.Prompts.ForeshadowUpdate, map[string]string{
		"Title":          state.Title,
		"ChapterNum":     fmt.Sprintf("%d", ch.Num),
		"ChapterTitle":   ch.Title,
		"ChapterContent": ch.Content,
		"HistorySummary": historySummary,
		"Foreshadows":    foreshadowsText,
	})

	systemPrompt := "你是一位严谨的小说伏笔追踪员。请严格按照要求的JSON格式输出，不要添加任何额外文字或markdown代码块标记。"

	rawResp := CallAPIWithRetryLog(ctx, apiCfg, systemPrompt, userPrompt, logger)
	if rawResp == "" {
		return fmt.Errorf("API 调用失败或被取消")
	}
	rawResp = cleanJSONResponse(rawResp)

	var resp ForeshadowUpdateResponse
	if err := json.Unmarshal([]byte(rawResp), &resp); err != nil {
		return fmt.Errorf("解析伏笔更新JSON失败: %w", err)
	}

	applyForeshadowUpdates(state, resp.Updates, ch.Num)
	logger.Info(fmt.Sprintf("伏笔状态更新完成，处理 %d 条变更", len(resp.Updates)))
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

func formatForeshadowsForPrompt(foreshadows []Foreshadow) string {
	if len(foreshadows) == 0 {
		return "无"
	}

	var sb strings.Builder
	for _, fs := range foreshadows {
		sb.WriteString(fmt.Sprintf("#%d [%s] %s\n", fs.ID, fs.Status, fs.Name))
		sb.WriteString(fmt.Sprintf("   描述: %s\n", fs.Description))
		sb.WriteString(fmt.Sprintf("   埋设于: 第%d章", fs.PlantChapter))
		if fs.TargetChapter > 0 {
			sb.WriteString(fmt.Sprintf("，预计回收: 第%d章", fs.TargetChapter))
		}
		sb.WriteString("\n")

		if len(fs.Events) > 0 {
			sb.WriteString("   已有进展:\n")
			for _, ev := range fs.Events {
				sb.WriteString(fmt.Sprintf("   - 第%d章: %s\n", ev.Chapter, ev.Note))
			}
		}

		if fs.Resolution != "" {
			sb.WriteString(fmt.Sprintf("   回收方式: %s\n", fs.Resolution))
		}

		sb.WriteString("\n")
	}

	return sb.String()
}

func formatActiveForeshadowsForChapter(foreshadows []Foreshadow, chapterNum int) string {
	var active []Foreshadow
	var overdue []Foreshadow

	for _, fs := range foreshadows {
		if fs.Status == ForeshadowPlanted || fs.Status == ForeshadowProgressing {
			active = append(active, fs)
			if fs.TargetChapter > 0 && chapterNum >= fs.TargetChapter {
				overdue = append(overdue, fs)
			}
		}
	}

	if len(active) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("【活跃伏笔（写作时必须注意推进或回收）】\n")

	for _, fs := range active {
		sb.WriteString(fmt.Sprintf("#%d \"%s\" [第%d章埋设", fs.ID, fs.Name, fs.PlantChapter))
		if fs.TargetChapter > 0 {
			sb.WriteString(fmt.Sprintf("，预计第%d章回收", fs.TargetChapter))
		}
		sb.WriteString("]\n")
		sb.WriteString(fmt.Sprintf("   描述: %s\n", fs.Description))

		if len(fs.Events) > 0 {
			sb.WriteString("   已有进展:\n")
			for _, ev := range fs.Events {
				sb.WriteString(fmt.Sprintf("   - 第%d章: %s\n", ev.Chapter, ev.Note))
			}
		}

		isOverdue := false
		for _, od := range overdue {
			if od.ID == fs.ID {
				isOverdue = true
				break
			}
		}

		if isOverdue {
			sb.WriteString(fmt.Sprintf("   ⚠️ 该伏笔已超过预计回收章节（第%d章），本章应优先考虑回收\n", fs.TargetChapter))
		} else if fs.TargetChapter > 0 && chapterNum >= fs.TargetChapter-2 {
			sb.WriteString(fmt.Sprintf("   → 接近预计回收节点（第%d章），可开始收束\n", fs.TargetChapter))
		}

		sb.WriteString("\n")
	}

	return sb.String()
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
