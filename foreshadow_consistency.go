package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

func buildFullOutlineText(state *Progress, lang string) string {
	var sb strings.Builder
	for _, ch := range state.Chapters {
		sb.WriteString(formatChapterLine(ch.Num, ch.Title, ch.Outline, lang))
	}
	return sb.String()
}

func buildAcceptedSummariesText(state *Progress, lang string) string {
	en := NormalizeLanguage(lang) == LangEN
	var sb strings.Builder
	for _, ch := range state.Chapters {
		if ch.Status != StatusAccepted || ch.Summary == "" {
			continue
		}
		if en {
			sb.WriteString(fmt.Sprintf("Chapter %d \"%s\": %s\n", ch.Num, ch.Title, ch.Summary))
		} else {
			sb.WriteString(fmt.Sprintf("第%d章《%s》：%s\n", ch.Num, ch.Title, ch.Summary))
		}
	}
	if sb.Len() == 0 {
		if en {
			return "(no confirmed chapters yet)"
		}
		return "尚无已确认章节。"
	}
	return sb.String()
}

func CheckForeshadowOutlineConsistency(ctx context.Context, apiCfg *APIConfig, cfg *Config, state *Progress, logger *LogBroadcaster) (*ForeshadowOutlineReport, error) {
	if len(state.Foreshadows) == 0 {
		return &ForeshadowOutlineReport{Summary: "无伏笔"}, nil
	}

	lang := cfg.Language
	userPrompt := RenderPrompt(cfg.Prompts.ForeshadowOutlineConsistency, map[string]string{
		"Title":              preferUserValue(cfg.Story.Title, state.Title),
		"Outline":            buildFullOutlineText(state, lang),
		"Foreshadows":        formatForeshadowsForPromptLang(state.Foreshadows, lang),
		"AcceptedSummaries":  buildAcceptedSummariesText(state, lang),
	})
	systemPrompt := SystemPromptFor(lang, "foreshadow_outline_checker_json")

	rawResp := CallAPIWithRetryLog(ctx, apiCfg, systemPrompt, userPrompt, logger)
	if rawResp == "" {
		return nil, fmt.Errorf("API 调用失败或被取消")
	}

	var report ForeshadowOutlineReport
	jsonStr := extractJSON(cleanJSONResponse(rawResp))
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

func RunForeshadowOutlineCheckAndSave(ctx context.Context, apiCfg *APIConfig, cfg *Config, state *Progress, progressPath string, logger *LogBroadcaster) {
	if len(state.Foreshadows) == 0 {
		return
	}
	report, err := CheckForeshadowOutlineConsistency(ctx, apiCfg, cfg, state, logger)
	if err != nil {
		logger.WarnKey("log.foreshadow_outline_check_failed", err)
		return
	}
	applyForeshadowOutlineReport(state, report)
	if err := SaveProgress(progressPath, state); err != nil {
		logger.WarnKey("log.foreshadow_outline_report_save_failed", err)
		return
	}
	if report.HasConflicts {
		logger.ForeshadowOutlineConflicts(report)
	} else {
		logger.InfoKey("log.foreshadow_outline_check_pass")
	}
}
