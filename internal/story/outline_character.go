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

type OutlineCharacterSuggestion struct {
	Name        string `json:"name"`
	ChapterNum  int    `json:"chapter_num"`
	Description string `json:"description"`
	Role        string `json:"role,omitempty"`
}

type OutlineCharacterReport struct {
	HasSuggestions bool                         `json:"has_suggestions"`
	Suggestions    []OutlineCharacterSuggestion `json:"suggestions"`
	Summary        string                       `json:"summary"`
}

func formatRegisteredCharactersForCheck(settings *ProjectSettings, lang string) string {
	if settings == nil || len(settings.Characters) == 0 {
		if i18n.NormalizeLanguage(lang) == i18n.LangEN {
			return "(none registered)"
		}
		return "（无已登记角色）"
	}
	var sb strings.Builder
	for _, c := range settings.Characters {
		sb.WriteString(fmt.Sprintf("- %s\n", c.Name))
	}
	return sb.String()
}

func mergeOutlineCharacterSuggestions(heuristic, ai []OutlineCharacterSuggestion) []OutlineCharacterSuggestion {
	seen := make(map[string]bool)
	var out []OutlineCharacterSuggestion
	add := func(s OutlineCharacterSuggestion) {
		name := strings.TrimSpace(StripNameMarks(s.Name))
		if name == "" || seen[name] {
			return
		}
		seen[name] = true
		s.Name = name
		out = append(out, s)
	}
	for _, s := range heuristic {
		add(s)
	}
	for _, s := range ai {
		add(s)
	}
	return out
}

func detectUnregisteredCharactersHeuristic(state *Progress, settings *ProjectSettings) []OutlineCharacterSuggestion {
	registered := RegisteredCharacterNameSet(settings)
	var suggestions []OutlineCharacterSuggestion
	for _, ch := range state.Chapters {
		for _, stub := range characterStubsForChapter(ch) {
			if registered[stub.Name] {
				continue
			}
			suggestions = append(suggestions, OutlineCharacterSuggestion{
				Name:        stub.Name,
				ChapterNum:  ch.Num,
				Description: stub.Description,
			})
		}
	}
	return suggestions
}

func CheckOutlineCharacterConsistency(ctx context.Context, apiCfg *config.APIConfig, cfg *config.Config, state *Progress, settings *ProjectSettings, logger *sse.LogBroadcaster) (*OutlineCharacterReport, error) {
	heuristic := detectUnregisteredCharactersHeuristic(state, settings)

	lang := cfg.Language
	userPrompt := config.RenderPrompt(cfg.Prompts.OutlineCharacterCheck, map[string]string{
		"Title":                preferUserValue(cfg.Story.Title, state.Title),
		"Outline":              buildFullOutlineText(state, lang),
		"RegisteredCharacters": formatRegisteredCharactersForCheck(settings, lang),
		"AcceptedSummaries":    buildAcceptedSummariesText(state, lang),
	})
	systemPrompt := i18n.SystemPromptFor(lang, "outline_character_checker_json")

	rawResp := llm.CallAPIWithRetryLog(ctx, apiCfg, systemPrompt, userPrompt, logger)
	if rawResp == "" {
		if len(heuristic) > 0 {
			return &OutlineCharacterReport{
				HasSuggestions: true,
				Suggestions:    heuristic,
				Summary:        "检测到未登记的大纲人物（启发式扫描）",
			}, nil
		}
		return nil, fmt.Errorf("API 调用失败或被取消")
	}

	var report OutlineCharacterReport
	jsonStr := llm.ExtractJSON(cleanJSONResponse(rawResp))
	if jsonStr == "" {
		if len(heuristic) > 0 {
			return &OutlineCharacterReport{
				HasSuggestions: true,
				Suggestions:    heuristic,
				Summary:        "检测到未登记的大纲人物（启发式扫描）",
			}, nil
		}
		return nil, fmt.Errorf("无法解析大纲人物检查结果")
	}
	if err := json.Unmarshal([]byte(jsonStr), &report); err != nil {
		return nil, fmt.Errorf("解析大纲人物检查JSON失败: %w", err)
	}

	report.Suggestions = mergeOutlineCharacterSuggestions(heuristic, report.Suggestions)
	report.HasSuggestions = len(report.Suggestions) > 0
	return &report, nil
}

func applyOutlineCharacterReport(state *Progress, report *OutlineCharacterReport) {
	if report == nil {
		return
	}
	state.LastOutlineCharacterReport = report
}

func RunOutlineCharacterCheckAndSave(ctx context.Context, apiCfg *config.APIConfig, cfg *config.Config, state *Progress, settings *ProjectSettings, progressPath string, logger *sse.LogBroadcaster) {
	report, err := CheckOutlineCharacterConsistency(ctx, apiCfg, cfg, state, settings, logger)
	if err != nil {
		logger.WarnKey("log.outline_character_check_failed", err)
		return
	}
	applyOutlineCharacterReport(state, report)
	if err := SaveProgress(progressPath, state); err != nil {
		logger.WarnKey("log.outline_character_report_save_failed", err)
		return
	}
	if report.HasSuggestions {
		logger.OutlineCharacterSuggestions(report.Suggestions)
	} else {
		logger.InfoKey("log.outline_character_check_pass")
	}
}

func runOutlinePostProcessChecks(ctx context.Context, apiCfg *config.APIConfig, cfg *config.Config, state *Progress, settings *ProjectSettings, progressPath string, logger *sse.LogBroadcaster) {
	RunForeshadowOutlineCheckAndSave(ctx, apiCfg, cfg, state, progressPath, logger)
	RunOutlineCharacterCheckAndSave(ctx, apiCfg, cfg, state, settings, progressPath, logger)
}
