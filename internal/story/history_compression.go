package story

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"reflect"
	"showmethestory/internal/config"
	"showmethestory/internal/i18n"
	"showmethestory/internal/llm"
	"showmethestory/internal/sse"
	"sort"
	"strings"
)

const (
	historyLeafChapters   = 20
	historyRecentChapters = 20
	historyStartThreshold = 40
	historyFanout         = 10
	historyLeafRunes      = 800
	historyParentRunes    = 1200
	historyPromptRunes    = 10000
)

func truncateRunesExact(s string, limit int) string {
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	if limit <= 0 {
		return ""
	}
	return string(runes[:limit])
}

func checkpointHash(parts []string) string {
	h := sha256.New()
	for _, part := range parts {
		h.Write([]byte(part))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

func boundedHistoryFallback(parts []string, limit int) string {
	if len(parts) == 0 || limit <= 0 {
		return ""
	}
	per := max(16, limit/len(parts))
	var out strings.Builder
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if runes := []rune(part); len(runes) > per {
			part = string(runes[:per])
		}
		out.WriteString(part)
		out.WriteByte('\n')
	}
	return truncateRunesExact(strings.TrimSpace(out.String()), limit)
}

func compressHistory(ctx context.Context, api *config.APIConfig, cfg *config.Config, parts []string, limit int) (string, bool, error) {
	if err := ctx.Err(); err != nil {
		return "", false, err
	}
	fallback := boundedHistoryFallback(parts, limit)
	if api == nil || cfg == nil || strings.TrimSpace(api.BaseURL) == "" {
		return fallback, true, nil
	}
	prompt := config.RenderPrompt(cfg.Prompts.HistoryCompression, map[string]string{
		"History": strings.Join(parts, "\n"), "MaxRunes": fmt.Sprint(limit),
	})
	shortAPI := *api
	shortAPI.MaxTokens = 1200
	if api.MaxTokens > 0 {
		shortAPI.MaxTokens = min(shortAPI.MaxTokens, api.MaxTokens)
	}
	result, err := llm.CallAPI(ctx, &shortAPI, i18n.SystemPromptFor(cfg.Language, "author_default"), prompt)
	if err := ctx.Err(); err != nil {
		return "", false, err
	}
	if err != nil || strings.TrimSpace(result) == "" {
		return fallback, true, nil
	}
	return truncateRunesExact(strings.TrimSpace(result), limit), false, nil
}

// EnsureNarrativeCheckpoints creates 20-chapter cold-history summaries and
// rolls every ten siblings into a higher level. Source hashes make old edits
// and deletions rebuild only the affected chain.
func EnsureNarrativeCheckpoints(ctx context.Context, api *config.APIConfig, cfg *config.Config, state *Progress, path string, logger *sse.LogBroadcaster) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	accepted := make([]ChapterState, 0, len(state.Chapters))
	for _, ch := range state.Chapters {
		if ch.Status == StatusAccepted {
			accepted = append(accepted, ch)
		}
	}
	old := map[string]NarrativeCheckpoint{}
	for _, cp := range state.NarrativeCheckpoints {
		old[fmt.Sprintf("%d:%d:%d", cp.Level, cp.StartChapter, cp.EndChapter)] = cp
	}
	all := []NarrativeCheckpoint{}
	if len(accepted) > historyStartThreshold {
		cold := (len(accepted) - historyRecentChapters) / historyLeafChapters * historyLeafChapters
		for start := 0; start < cold; start += historyLeafChapters {
			group := accepted[start : start+historyLeafChapters]
			parts := make([]string, len(group))
			revisions := make([]string, len(group))
			for i, ch := range group {
				parts[i] = fmt.Sprintf("[%d] %s — %s", ch.Num, ch.Summary, ch.Title)
				revisions[i] = ChapterRevision(ch)
			}
			cp, err := checkpointFor(ctx, api, cfg, old, 1, group[0].Num, group[len(group)-1].Num, parts, revisions, historyLeafRunes)
			if err != nil {
				return err
			}
			all = append(all, cp)
		}
	}
	levelNodes := append([]NarrativeCheckpoint(nil), all...)
	for level := 2; len(levelNodes) >= historyFanout; level++ {
		next := []NarrativeCheckpoint{}
		for start := 0; start+historyFanout <= len(levelNodes); start += historyFanout {
			group := levelNodes[start : start+historyFanout]
			parts := make([]string, len(group))
			revisions := make([]string, len(group))
			degraded := false
			for i, cp := range group {
				parts[i] = fmt.Sprintf("[%d-%d] %s", cp.StartChapter, cp.EndChapter, cp.Summary)
				revisions[i] = fmt.Sprintf("%s:%t", cp.SourceHash, cp.Degraded)
				degraded = degraded || cp.Degraded
			}
			cp, err := checkpointFor(ctx, api, cfg, old, level, group[0].StartChapter, group[len(group)-1].EndChapter, parts, revisions, historyParentRunes)
			if err != nil {
				return err
			}
			cp.Degraded = cp.Degraded || degraded
			next = append(next, cp)
		}
		all = append(all, next...)
		levelNodes = next
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].Level != all[j].Level {
			return all[i].Level < all[j].Level
		}
		return all[i].StartChapter < all[j].StartChapter
	})
	if err := ctx.Err(); err != nil {
		return err
	}
	if logger != nil {
		for _, cp := range all {
			if cp.Degraded {
				logger.WarnKey("log.history_degraded")
				break
			}
		}
	}
	if reflect.DeepEqual(state.NarrativeCheckpoints, all) {
		return nil
	}
	if path != "" {
		next := *state
		next.NarrativeCheckpoints = all
		if err := SaveProgress(path, &next); err != nil {
			return err
		}
	}
	state.NarrativeCheckpoints = all
	return nil
}

func checkpointFor(ctx context.Context, api *config.APIConfig, cfg *config.Config, old map[string]NarrativeCheckpoint, level, start, end int, parts, revisions []string, limit int) (NarrativeCheckpoint, error) {
	if err := ctx.Err(); err != nil {
		return NarrativeCheckpoint{}, err
	}
	// Version the hash to rebuild legacy checkpoints whose fallback provenance is unknown.
	hash := checkpointHash([]string{"v2", checkpointHash(parts), checkpointHash(revisions)})
	key := fmt.Sprintf("%d:%d:%d", level, start, end)
	if cp, ok := old[key]; ok && !cp.Degraded && cp.SourceHash == hash && strings.TrimSpace(cp.Summary) != "" {
		return cp, nil
	}
	summary, degraded, err := compressHistory(ctx, api, cfg, parts, limit)
	return NarrativeCheckpoint{StartChapter: start, EndChapter: end, Level: level, SourceHash: hash, Summary: summary, Degraded: degraded}, err
}

func checkpointCover(state *Progress, beforeChapter int) []NarrativeCheckpoint {
	byStart := map[int][]NarrativeCheckpoint{}
	first := beforeChapter
	for _, cp := range state.NarrativeCheckpoints {
		if cp.EndChapter < beforeChapter && cp.SourceHash != "" {
			byStart[cp.StartChapter] = append(byStart[cp.StartChapter], cp)
			first = min(first, cp.StartChapter)
		}
	}
	cover := []NarrativeCheckpoint{}
	for start := first; start < beforeChapter; {
		choices := byStart[start]
		if len(choices) == 0 {
			break
		}
		best := choices[0]
		for _, cp := range choices[1:] {
			if cp.EndChapter > best.EndChapter {
				best = cp
			}
		}
		cover = append(cover, best)
		start = best.EndChapter + 1
	}
	return cover
}

func formatCheckpointHistory(state *Progress, beforeChapter, limit int, lang string) string {
	parts := []string{}
	for _, cp := range checkpointCover(state, beforeChapter) {
		if i18n.NormalizeLanguage(lang) == i18n.LangEN {
			parts = append(parts, fmt.Sprintf("[Compressed history, chapters %d-%d] %s", cp.StartChapter, cp.EndChapter, cp.Summary))
		} else {
			parts = append(parts, fmt.Sprintf("【压缩历史：第%d-%d章】%s", cp.StartChapter, cp.EndChapter, cp.Summary))
		}
	}
	return boundedHistoryFallback(parts, limit)
}

func BuildPlanningHistory(state *Progress, query, lang string) string {
	var parts []string
	if len(state.OutlineBatches) > 0 {
		start := max(0, len(state.OutlineBatches)-2)
		for _, b := range state.OutlineBatches[start:] {
			parts = append(parts, fmt.Sprintf("[%d-%d] %s", b.StartCh, b.EndCh, b.Synopsis))
		}
	}
	parts = append(parts, formatCheckpointHistory(state, maxChapterNum(state)+1, historyPromptRunes/2, lang))
	recentStart := max(0, len(state.Chapters)-historyRecentChapters)
	for _, ch := range state.Chapters[recentStart:] {
		parts = append(parts, formatChapterLine(ch.Num, ch.Title, ch.Outline, lang)+ch.Summary)
	}
	docs := []knowledgeDocument{}
	for _, ch := range state.Chapters[:recentStart] {
		docs = append(docs, knowledgeDocument{text: formatChapterLine(ch.Num, ch.Title, ch.Outline, lang) + ch.Summary})
	}
	for _, i := range packKnowledge(docs, rankKnowledge(docs, query), 3000) {
		parts = append(parts, docs[i].text)
	}
	return boundedHistoryFallback(parts, historyPromptRunes)
}
