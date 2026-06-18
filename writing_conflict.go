package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

type WritingConflictError struct {
	Conflict *WritingConflict
}

func (e *WritingConflictError) Error() string {
	if e == nil || e.Conflict == nil {
		return "写作冲突需用户处理"
	}
	return e.Conflict.Summary
}

type writingConflictAnalysis struct {
	Reconcilable      bool                   `json:"reconcilable"`
	Summary           string                 `json:"summary"`
	RootCause         string                 `json:"root_cause"`
	ExtraConstraints  string                 `json:"extra_constraints"`
	SuggestedActions  []ConflictActionOption   `json:"suggested_actions"`
}

func analyzeWritingConflict(ctx context.Context, apiCfg *APIConfig, cfg *Config, state *Progress, idx int, content string, failedIssues []string, logger *LogBroadcaster) (*writingConflictAnalysis, error) {
	ch := state.Chapters[idx]
	lang := cfg.Language

	excerpt := content
	if runes := []rune(excerpt); len(runes) > 1200 {
		excerpt = string(runes[:600]) + "\n...\n" + string(runes[len(runes)-600:])
	}

	foreshadowBlock := formatActiveForeshadowsForChapterLang(state.Foreshadows, ch.Num, lang)
	outlineConstraints := buildOutlineConstraintsForLang(state, idx, lang)

	userPrompt := RenderPrompt(cfg.Prompts.WritingConflictAnalysis, map[string]string{
		"ChapterNum":         fmt.Sprintf("%d", ch.Num),
		"ChapterTitle":       ch.Title,
		"ChapterOutline":     ch.Outline,
		"HistorySummary":     buildHistorySummaryForLang(state, idx, lang),
		"OutlineConstraints": outlineConstraints,
		"Foreshadows":        foreshadowBlock,
		"FailedIssues":       strings.Join(failedIssues, "\n"),
		"ContentExcerpt":     excerpt,
	})
	userPrompt = appendIfMissingPlaceholder(cfg.Prompts.WritingConflictAnalysis, userPrompt, "{{.OutlineConstraints}}", outlineConstraints)
	userPrompt = appendIfMissingPlaceholder(cfg.Prompts.WritingConflictAnalysis, userPrompt, "{{.Foreshadows}}", foreshadowBlock)

	systemPrompt := SystemPromptFor(lang, "writing_conflict_analyst_json")
	rawResp := CallAPIWithRetryLog(ctx, apiCfg, systemPrompt, userPrompt, logger)
	if rawResp == "" {
		return nil, fmt.Errorf("API 调用失败或被取消")
	}

	var analysis writingConflictAnalysis
	jsonStr := extractJSON(cleanJSONResponse(rawResp))
	if jsonStr == "" {
		return nil, fmt.Errorf("无法解析写作冲突分析结果")
	}
	if err := json.Unmarshal([]byte(jsonStr), &analysis); err != nil {
		return nil, fmt.Errorf("解析写作冲突分析JSON失败: %w", err)
	}
	analysis.SuggestedActions = ensureConflictActions(analysis.SuggestedActions, lang)
	return &analysis, nil
}

func ensureConflictActions(actions []ConflictActionOption, lang string) []ConflictActionOption {
	en := NormalizeLanguage(lang) == LangEN
	byID := map[string]ConflictActionOption{}
	for _, a := range actions {
		if a.ID != "" {
			byID[a.ID] = a
		}
	}
	defaults := []ConflictActionOption{
		{ID: "edit_outline", Label: pickLang(lang, "修改本章大纲", "Edit chapter outline"), Description: pickLang(lang, "在大纲页调整本章或后续章节大纲，使情节与伏笔/前情一致", "Adjust this or later chapter outlines on the Outline page")},
		{ID: "adjust_foreshadow", Label: pickLang(lang, "调整伏笔", "Adjust foreshadows"), Description: pickLang(lang, "在伏笔页修改埋设/回收章节或描述，或放弃无法实现的伏笔", "Change plant/payoff chapters or descriptions on the Foreshadows page, or abandon unworkable foreshadows")},
		{ID: "retry", Label: pickLang(lang, "修改后重试生成", "Retry after edits"), Description: pickLang(lang, "完成大纲或伏笔调整后，重新生成本章", "Regenerate this chapter after you have edited the outline or foreshadows")},
		{ID: "force_review", Label: pickLang(lang, "保留当前稿进入审核", "Keep draft for review"), Description: pickLang(lang, "接受当前版本，进入人工审核后再决定修订或确认", "Accept the current draft and review it manually")},
	}
	_ = en
	out := make([]ConflictActionOption, 0, len(defaults))
	for _, d := range defaults {
		if a, ok := byID[d.ID]; ok {
			if a.Label == "" {
				a.Label = d.Label
			}
			if a.Description == "" {
				a.Description = d.Description
			}
			out = append(out, a)
		} else if d.ID == "retry" || d.ID == "force_review" || d.ID == "edit_outline" || d.ID == "adjust_foreshadow" {
			out = append(out, d)
		}
	}
	return out
}

func pickLang(lang, zh, en string) string {
	if NormalizeLanguage(lang) == LangEN {
		return en
	}
	return zh
}

func buildWritingConflict(state *Progress, idx int, failedIssues []string, analysis *writingConflictAnalysis) *WritingConflict {
	ch := state.Chapters[idx]
	summary := "事实核查多次失败，需人工决定修改方向"
	rootCause := "other"
	reconcilable := false
	actions := ensureConflictActions(nil, LangZH)
	if analysis != nil {
		if strings.TrimSpace(analysis.Summary) != "" {
			summary = strings.TrimSpace(analysis.Summary)
		}
		rootCause = analysis.RootCause
		reconcilable = analysis.Reconcilable
		if len(analysis.SuggestedActions) > 0 {
			actions = analysis.SuggestedActions
		}
	}
	return &WritingConflict{
		ChapterIndex:     idx,
		ChapterNum:       ch.Num,
		ChapterTitle:     ch.Title,
		Issues:           failedIssues,
		Summary:          summary,
		RootCause:        rootCause,
		Reconcilable:     reconcilable,
		SuggestedActions: actions,
	}
}

func mergeUniqueIssues(issues ...[]string) []string {
	seen := map[string]bool{}
	var out []string
	for _, list := range issues {
		for _, item := range list {
			item = strings.TrimSpace(item)
			if item == "" || seen[item] {
				continue
			}
			seen[item] = true
			out = append(out, item)
		}
	}
	return out
}

func splitFactCheckIssues(issues string) []string {
	if strings.TrimSpace(issues) == "" {
		return nil
	}
	parts := strings.Split(issues, "；")
	if len(parts) == 1 {
		parts = strings.Split(issues, ";")
	}
	return mergeUniqueIssues(parts)
}
