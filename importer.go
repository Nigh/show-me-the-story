package main

// v3 import pipeline: paste a published book, split it into chapters locally
// (no AI, instant preview), then process chapters one by one with AI (outline
// + summary), checkpointing after every chapter so the task can be stopped
// and resumed. Long imports (>= importArcThreshold chapters) are grouped into
// arcs and get arc summaries, plugging straight into the M2 context layering.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	importArcThreshold = 40   // group into arcs at or above this many chapters
	importArcSize      = 30   // chapters per auto-created arc
	importChunkRunes   = 6000 // fallback chunk size when no headings found
	importHeadRunes    = 6000 // max chapter content passed to per-chapter AI
)

// —— local splitting ——

var importHeadingRe = regexp.MustCompile(`(?m)^[ \t]*(第[0-9一二三四五六七八九十百千万零两]+[章回]|Chapter[ \t]*\d+|#{1,3}[ \t]*Chapter[ \t]*\d+|序章|楔子|尾声|Prologue|Epilogue)[^\n]*`)

type ImportChapterPreview struct {
	Num       int    `json:"num"`
	Title     string `json:"title"`
	WordCount int    `json:"word_count"`
	Preview   string `json:"preview"`
}

type importedChapter struct {
	Title   string
	Content string
}

// splitImportContent splits pasted text into chapters by heading lines.
// Returns the imported chapters and the preamble text before the first
// heading (often a synopsis — fed to meta analysis, not stored as a chapter).
func splitImportContent(content string) (chapters []importedChapter, preamble string) {
	matches := importHeadingRe.FindAllStringIndex(content, -1)
	if len(matches) == 0 {
		return chunkBySize(content), ""
	}
	preamble = strings.TrimSpace(content[:matches[0][0]])
	for i, m := range matches {
		end := len(content)
		if i+1 < len(matches) {
			end = matches[i+1][0]
		}
		seg := strings.TrimSpace(content[m[0]:end])
		if seg == "" {
			continue
		}
		title := seg
		if nl := strings.IndexByte(seg, '\n'); nl >= 0 {
			title = seg[:nl]
		}
		title = strings.TrimSpace(strings.TrimLeft(title, "# \t"))
		if r := []rune(title); len(r) > 50 {
			title = string(r[:50])
		}
		chapters = append(chapters, importedChapter{Title: title, Content: seg})
	}
	return chapters, preamble
}

// chunkBySize splits heading-less text into ~importChunkRunes chunks aligned
// to paragraph boundaries. Titles are placeholders the AI pass will not touch.
func chunkBySize(content string) []importedChapter {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil
	}
	paras := strings.Split(content, "\n\n")
	if len(paras) == 1 {
		paras = strings.Split(content, "\n")
	}
	var out []importedChapter
	var buf strings.Builder
	bufRunes := 0
	flush := func() {
		if bufRunes == 0 {
			return
		}
		out = append(out, importedChapter{
			Title:   fmt.Sprintf("第%d章", len(out)+1),
			Content: strings.TrimSpace(buf.String()),
		})
		buf.Reset()
		bufRunes = 0
	}
	for _, p := range paras {
		if buf.Len() > 0 {
			buf.WriteString("\n\n")
		}
		buf.WriteString(p)
		bufRunes += len([]rune(p))
		if bufRunes >= importChunkRunes {
			flush()
		}
	}
	flush()
	return out
}

func buildImportPreview(chapters []importedChapter) []ImportChapterPreview {
	out := make([]ImportChapterPreview, len(chapters))
	for i, ch := range chapters {
		body := ch.Content
		if nl := strings.IndexByte(body, '\n'); nl >= 0 {
			body = strings.TrimSpace(body[nl:])
		}
		preview := body
		if r := []rune(preview); len(r) > 80 {
			preview = string(r[:80]) + "…"
		}
		out[i] = ImportChapterPreview{
			Num:       i + 1,
			Title:     ch.Title,
			WordCount: countProseUnits(ch.Content),
			Preview:   strings.ReplaceAll(preview, "\n", " "),
		}
	}
	return out
}

// —— import state (checkpoint file) ——

type ImportState struct {
	Active   bool   `json:"active"`
	Total    int    `json:"total"`
	Cursor   int    `json:"cursor"` // chapters fully processed so far
	MetaDone bool   `json:"meta_done"`
	Preamble string `json:"preamble,omitempty"`
}

func ImportStatePath(projectDir string) string {
	return filepath.Join(projectDir, "import.json")
}

func LoadImportState(path string) *ImportState {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var st ImportState
	if json.Unmarshal(data, &st) != nil || !st.Active {
		return nil
	}
	return &st
}

func SaveImportState(path string, st *ImportState) error {
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(path, data)
}

// —— pipeline ——

// ImportStartAction splits the text, stores all chapters as accepted content
// (checkpoint zero), then runs the per-chapter pipeline.
func ImportStartAction(ctx context.Context, apiCfg *APIConfig, cfg *Config, state *Progress, settings *ProjectSettings, content, progressPath, cfgPath, importPath string, logger *LogBroadcaster) error {
	if err := validateAPIConfig(apiCfg); err != nil {
		return err
	}
	if len(state.Chapters) > 0 {
		return fmt.Errorf("项目已有章节，无法导入。请在空项目中导入")
	}
	chapters, preamble := splitImportContent(content)
	if len(chapters) == 0 {
		return fmt.Errorf("未能从文本中切分出任何章节")
	}

	state.Chapters = make([]ChapterState, len(chapters))
	for i, ch := range chapters {
		state.Chapters[i] = ChapterState{
			Num:     i + 1,
			Title:   ch.Title,
			Content: ch.Content,
			Status:  StatusAccepted,
		}
	}
	state.CurrentChapterIndex = len(chapters)
	state.Phase = "outline"
	if err := SaveProgress(progressPath, state); err != nil {
		return fmt.Errorf("保存进度失败: %w", err)
	}
	st := &ImportState{Active: true, Total: len(chapters), Preamble: truncate(preamble, 2000)}
	if err := SaveImportState(importPath, st); err != nil {
		return fmt.Errorf("保存导入状态失败: %w", err)
	}
	logger.InfoKey("log.import_split_done", len(chapters))

	return runImportPipeline(ctx, apiCfg, cfg, state, st, progressPath, cfgPath, importPath, logger)
}

// ImportResumeAction continues an interrupted import from its checkpoint.
func ImportResumeAction(ctx context.Context, apiCfg *APIConfig, cfg *Config, state *Progress, importPath, progressPath, cfgPath string, logger *LogBroadcaster) error {
	if err := validateAPIConfig(apiCfg); err != nil {
		return err
	}
	st := LoadImportState(importPath)
	if st == nil {
		return fmt.Errorf("没有待恢复的导入任务")
	}
	logger.InfoKey("log.import_resuming", st.Cursor, st.Total)
	return runImportPipeline(ctx, apiCfg, cfg, state, st, progressPath, cfgPath, importPath, logger)
}

func runImportPipeline(ctx context.Context, apiCfg *APIConfig, cfg *Config, state *Progress, st *ImportState, progressPath, cfgPath, importPath string, logger *LogBroadcaster) error {
	// Step 1: metadata analysis (once).
	if !st.MetaDone {
		if err := importMetaAnalysis(ctx, apiCfg, cfg, state, st, progressPath, cfgPath, logger); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			// Meta is nice-to-have; chapters are the real work.
			logger.WarnKey("log.import_meta_failed", err)
		}
		st.MetaDone = true
		SaveImportState(importPath, st)
	}

	// Step 2: per-chapter outline + summary, checkpoint after each.
	minLen, maxLen := calcOutlineLengthRange(cfg.Story.TargetWordsPerChapter)
	for i := range state.Chapters {
		ch := &state.Chapters[i]
		if ch.Summary != "" && ch.Outline != "" {
			continue
		}
		if ctx.Err() != nil {
			logger.WarnKey("log.import_cancelled", st.Cursor, st.Total)
			return ctx.Err()
		}
		body := ch.Content
		if r := []rune(body); len(r) > importHeadRunes {
			// ponytail: head-only truncation; outline quality on ultra-long
			// chapters could improve with head+tail, upgrade if it matters.
			body = string(r[:importHeadRunes]) + "\n……（后文略）"
		}
		userPrompt := RenderPrompt(cfg.Prompts.ImportChapterAnalysis, map[string]string{
			"Title":           preferUserValue(cfg.Story.Title, state.Title),
			"ChapterNum":      fmt.Sprintf("%d", ch.Num),
			"ChapterTitle":    ch.Title,
			"ChapterContent":  body,
			"OutlineMinWords": fmt.Sprintf("%d", minLen),
			"OutlineMaxWords": fmt.Sprintf("%d", maxLen),
		})
		rawResp := CallAPIWithRetryLog(ctx, apiCfg, SystemPromptFor(cfg.Language, "content_analyst_json"), userPrompt, logger)
		if rawResp == "" {
			if ctx.Err() != nil {
				logger.WarnKey("log.import_cancelled", st.Cursor, st.Total)
				return ctx.Err()
			}
			return fmt.Errorf("第 %d 章分析失败：API 调用失败", ch.Num)
		}
		var resp struct {
			Outline string `json:"outline"`
			Summary string `json:"summary"`
		}
		if err := json.Unmarshal([]byte(cleanJSONResponse(rawResp)), &resp); err != nil {
			return fmt.Errorf("第 %d 章分析结果解析失败: %w", ch.Num, err)
		}
		if resp.Outline != "" {
			ch.Outline = strings.TrimSpace(resp.Outline)
		}
		if resp.Summary != "" {
			ch.Summary = strings.TrimSpace(resp.Summary)
		}
		if err := SaveProgress(progressPath, state); err != nil {
			return fmt.Errorf("保存进度失败: %w", err)
		}
		st.Cursor = i + 1
		SaveImportState(importPath, st)
		logger.InfoKey("log.import_chapter_done", ch.Num, st.Total)
	}

	// Step 3: arc grouping + summaries for long imports.
	if len(state.Chapters) >= importArcThreshold && len(state.Arcs) == 0 {
		createImportArcs(state)
		if err := SaveProgress(progressPath, state); err != nil {
			return fmt.Errorf("保存进度失败: %w", err)
		}
		logger.InfoKey("log.import_arcs_created", len(state.Arcs))
		EnsureArcSummaries(ctx, apiCfg, cfg, state, progressPath, logger)
		if ctx.Err() != nil {
			logger.WarnKey("log.import_cancelled", st.Cursor, st.Total)
			return ctx.Err()
		}
	}

	deleteFile(importPath)
	logger.SuccessKey("log.import_done", st.Total)
	return nil
}

func importMetaAnalysis(ctx context.Context, apiCfg *APIConfig, cfg *Config, state *Progress, st *ImportState, progressPath, cfgPath string, logger *LogBroadcaster) error {
	var titles strings.Builder
	for _, ch := range state.Chapters {
		titles.WriteString(fmt.Sprintf("%d. %s\n", ch.Num, ch.Title))
	}
	excerpt := st.Preamble
	if len(state.Chapters) > 0 {
		head := state.Chapters[0].Content
		if r := []rune(head); len(r) > 3000 {
			head = string(r[:3000])
		}
		excerpt += "\n\n" + head
	}
	userPrompt := RenderPrompt(cfg.Prompts.ImportMetaAnalysis, map[string]string{
		"OpeningExcerpt": strings.TrimSpace(excerpt),
		"ChapterTitles":  truncate(titles.String(), 4000),
	})
	rawResp := CallAPIWithRetryLog(ctx, apiCfg, SystemPromptFor(cfg.Language, "content_analyst_json"), userPrompt, logger)
	if rawResp == "" {
		return fmt.Errorf("API 调用失败或被取消")
	}
	var meta struct {
		Title         string `json:"title"`
		StoryType     string `json:"story_type"`
		CorePrompt    string `json:"core_prompt"`
		StorySynopsis string `json:"story_synopsis"`
		WritingStyle  string `json:"writing_style"`
		WritingPOV    string `json:"writing_pov"`
	}
	if err := json.Unmarshal([]byte(cleanJSONResponse(rawResp)), &meta); err != nil {
		return fmt.Errorf("元信息解析失败: %w", err)
	}

	state.Title = meta.Title
	state.CorePrompt = meta.CorePrompt
	state.StorySynopsis = meta.StorySynopsis
	// User-filled config wins (config guard rule): only fill empty fields.
	fill := func(dst *string, v string) {
		if strings.TrimSpace(*dst) == "" {
			*dst = v
		}
	}
	fill(&cfg.Story.Title, meta.Title)
	fill(&cfg.Story.Type, meta.StoryType)
	fill(&cfg.Story.StorySynopsis, meta.StorySynopsis)
	fill(&cfg.Story.WritingStyle, meta.WritingStyle)
	fill(&cfg.Story.WritingPOV, meta.WritingPOV)
	cfg.Story.ChapterCount = len(state.Chapters)
	snapshot := cfg.Story
	state.StoryConfigSnapshot = &snapshot

	if err := SaveProgress(progressPath, state); err != nil {
		return err
	}
	if err := saveConfig(cfgPath, cfg); err != nil {
		return err
	}
	logger.InfoKey("log.import_meta_done", meta.Title)
	return nil
}

// createImportArcs groups chapters into fixed-size arcs with placeholder
// titles; goals stay empty (EnsureArcSummaries provides the real context).
func createImportArcs(state *Progress) {
	n := len(state.Chapters)
	for start := 1; start <= n; start += importArcSize {
		end := start + importArcSize - 1
		if end > n {
			end = n
		}
		// Avoid a runt final arc: merge trailing < 10 chapters into the previous one.
		if n-end < 10 && n-end > 0 {
			end = n
		}
		state.Arcs = append(state.Arcs, Arc{
			ID:      len(state.Arcs) + 1,
			Title:   fmt.Sprintf("第%d卷", len(state.Arcs)+1),
			StartCh: start,
			EndCh:   end,
		})
		if end == n {
			break
		}
	}
}
