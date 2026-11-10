package story

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"showmethestory/internal/config"
	"showmethestory/internal/devlog"
	"showmethestory/internal/i18n"
	"showmethestory/internal/llm"
	"showmethestory/internal/sse"
	"strings"
)

type MemoryReference struct {
	Chapter    int    `json:"chapter"`
	BlockID    int    `json:"block_id"`
	Quote      string `json:"quote"`
	ContentRev string `json:"content_rev"`
	Stale      bool   `json:"stale,omitempty"`
}
type FactEditOptions struct {
	ContentRev        string `json:"content_rev"`
	ConfirmFactImpact bool   `json:"confirm_fact_impact"`
}

func ChapterRevision(ch ChapterState) string { return fmt.Sprintf("%x", HashContent(ch.Content)) }
func ReferenceLive(state *Progress, ref MemoryReference) bool {
	if ref.Stale {
		return false
	}
	i := FindChapterIdx(state, ref.Chapter)
	if i < 0 {
		return false
	}
	j := FindBlockIdx(&state.Chapters[i], ref.BlockID)
	return j >= 0 && state.Chapters[i].Blocks[j].Text == ref.Quote
}
func memoryHasLiveReference(state *Progress, m MemoryEntry) bool {
	for _, r := range m.References {
		if ReferenceLive(state, r) {
			return true
		}
	}
	return false
}
func FactsForChapter(state *Progress, num, blockID int) []MemoryEntry {
	out := []MemoryEntry{}
	for _, m := range state.MemoryEntries {
		match := false
		refs := append([]MemoryReference(nil), m.References...)
		for i := range refs {
			refs[i].Stale = refs[i].Stale || !ReferenceLive(state, refs[i])
			if refs[i].Chapter == num && (blockID == 0 || refs[i].BlockID == blockID) {
				match = true
			}
		}
		if match {
			m.References = refs
			out = append(out, m)
		}
	}
	return out
}
func ValidateFactEdit(state *Progress, before, after ChapterState, opts FactEditOptions, lang string) error {
	if opts.ContentRev != "" && opts.ContentRev != ChapterRevision(before) {
		return fmt.Errorf("%s", i18n.T(lang, "content_version_conflict"))
	}
	affected := false
	for _, m := range FactsForChapter(state, before.Num, 0) {
		for _, r := range m.References {
			if r.Chapter != before.Num {
				continue
			}
			a, b := FindBlockIdx(&before, r.BlockID), FindBlockIdx(&after, r.BlockID)
			if a >= 0 && (b < 0 || before.Blocks[a].Text != after.Blocks[b].Text) {
				affected = true
			}
		}
	}
	if affected && (!opts.ConfirmFactImpact || opts.ContentRev != ChapterRevision(before)) {
		return fmt.Errorf("%s", i18n.T(lang, "fact_impact_confirm"))
	}
	return nil
}
func factProtection(state *Progress, num int, lang string) string {
	facts := FactsForChapter(state, num, 0)
	live := facts[:0]
	for _, f := range facts {
		if memoryHasLiveReference(state, f) {
			// Revision must preserve every linked fact, but does not need copies
			// of all historical evidence paragraphs attached to those facts.
			f.References = nil
			f.Snippet = ""
			live = append(live, f)
		}
	}
	facts = live
	if len(facts) == 0 {
		return chapterEnding(state, num, lang)
	}
	data, _ := json.Marshal(facts)
	s := "\nPreserve these linked consistency facts unless the author explicitly authorizes changing them. Do not silently alter related passages.\n"
	if i18n.NormalizeLanguage(lang) == i18n.LangZH {
		s = "\n除非作者明确授权改变，否则必须保留以下关联的一致性事实，不得静默改写其他关联段落。\n"
	}
	return chapterEnding(state, num, lang) + s + string(data)
}

// knowledgeValidationError contains metadata only, never model text or entity names.
type knowledgeValidationError struct {
	reason                   string
	chapter                  int
	rev                      string
	block, index             int
	kind, id                 string
	inContext, inCurrentKind bool
}

func (e *knowledgeValidationError) Error() string {
	return fmt.Sprintf("%s: chapter=%d rev=%s stage=validate block=%d change_index=%d kind=%s entity_id=%s in_context=%t in_current_kind=%t", e.reason, e.chapter, e.rev, e.block, e.index, e.kind, e.id, e.inContext, e.inCurrentKind)
}

func knowledgeSyncEvent(ctx context.Context, level, event, phase, stage string, ch ChapterState, attempt int, err error) {
	attrs := []any{"task_id", devlog.TaskID(ctx), "phase", phase, "stage", stage, "chapter", ch.Num, "rev", ChapterRevision(ch), "attempt", attempt}
	var validation *knowledgeValidationError
	if errors.As(err, &validation) {
		attrs = append(attrs, "reason", validation.reason, "block", validation.block, "change_index", validation.index, "kind", validation.kind, "entity_id", validation.id, "in_context", validation.inContext, "in_current_kind", validation.inCurrentKind)
	} else if err != nil {
		attrs = append(attrs, "error_type", fmt.Sprintf("%T", err))
	}
	devlog.Event(level, event, attrs...)
}

func SyncChapterMemory(ctx context.Context, api *config.APIConfig, cfg *config.Config, state *Progress, idx int, path string, logger *sse.LogBroadcaster) (resultErr error) {
	if idx < 0 || idx >= len(state.Chapters) {
		return fmt.Errorf("chapter index out of range")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	ch := state.Chapters[idx]
	if ch.Content == "" {
		return nil
	}
	SyncChapterBlocks(&ch)
	rev := ChapterRevision(ch)
	if ch.MemoryRevision == rev {
		return nil
	}
	stage, currentAttempt := "config", 0
	defer func() {
		if resultErr != nil {
			knowledgeSyncEvent(ctx, "error", "knowledge_sync_failed", "facts", stage, ch, currentAttempt, resultErr)
			resultErr = fmt.Errorf("%w: phase=facts stage=%s chapter=%d rev=%s attempt=%d", resultErr, stage, ch.Num, rev, currentAttempt)
		}
	}()
	if err := llm.ValidateConfig(api); err != nil {
		return err
	}
	next := *state
	next.Chapters = append([]ChapterState(nil), state.Chapters...)
	next.Chapters[idx] = ch
	next.Chapters[idx].KnowledgeTracked = true
	// Persist the retry marker before invoking the model.
	stage = "save_pending"
	if err := SaveProgress(path, &next); err != nil {
		return err
	}
	*state = next
	if next.MemoryMaxTokens <= 0 {
		next.MemoryMaxTokens = calcMemoryMaxTokens(len(state.Chapters), cfg.Story.TargetWordsPerChapter)
	}
	existing := retrieveMemories(state, ch, memoryContextRunes, false)
	visibleFacts := map[int]bool{}
	for _, m := range existing {
		visibleFacts[m.ID] = true
	}
	blocks, _ := json.Marshal(ch.Blocks)
	prompt := config.RenderPrompt(cfg.Prompts.MemoryUpdate, map[string]string{
		"Title": state.Title, "ChapterNum": fmt.Sprint(ch.Num), "ChapterTitle": ch.Title, "ChapterOutline": ch.Outline,
		"ChapterContent": ch.Content, "ExistingMemory": knowledgeSelectionNotice(cfg.Language) + formatMemoryForUpdatePrompt(existing, cfg.Language),
		"MemoryMaxTokens": fmt.Sprint(next.MemoryMaxTokens),
	})
	prompt += memoryLinkPrompt(cfg.Language) + "\n" + string(blocks)
	var syncErr error
	for attempt := 0; attempt < 2; attempt++ {
		stage, currentAttempt = "request", attempt+1
		raw := llm.CallAPIWithRetryLog(ctx, api, i18n.SystemPromptFor(cfg.Language, "memory_manager"), prompt, logger)
		if err := ctx.Err(); err != nil {
			return err
		}
		if raw == "" {
			return fmt.Errorf("empty fact response")
		}
		next = *state
		if next.MemoryMaxTokens <= 0 {
			next.MemoryMaxTokens = calcMemoryMaxTokens(len(state.Chapters), cfg.Story.TargetWordsPerChapter)
		}
		stage = "validate"
		syncErr = applyChapterMemoryResult(&next, ch, visibleFacts, raw)
		if syncErr == nil {
			break
		}
		knowledgeSyncEvent(ctx, "warn", "knowledge_validation_failed", "facts", stage, ch, currentAttempt, syncErr)
		prompt += memoryRetryPrompt(cfg.Language, syncErr)
	}
	if syncErr != nil {
		return syncErr
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	stage = "save"
	next.Chapters = append([]ChapterState(nil), state.Chapters...)
	next.Chapters[idx].MemoryRevision = rev
	if err := SaveProgress(path, &next); err != nil {
		return err
	}
	*state = next
	knowledgeSyncEvent(ctx, "info", "knowledge_sync_succeeded", "facts", "complete", ch, currentAttempt, nil)
	return nil
}

// Validate a fresh copy for each attempt; no partial facts reach live state.
func applyChapterMemoryResult(next *Progress, ch ChapterState, visibleFacts map[int]bool, raw string) (resultErr error) {
	rev := ChapterRevision(ch)
	detail := &knowledgeValidationError{chapter: ch.Num, rev: rev, index: -1, kind: "fact"}
	defer func() {
		if resultErr != nil {
			detail.reason = resultErr.Error()
			resultErr = detail
		}
	}()
	var result struct {
		NewMemories *[]struct {
			ID       int    `json:"id"`
			Content  string `json:"content"`
			Category string `json:"category"`
			BlockIDs []int  `json:"block_ids"`
		} `json:"new_memories"`
	}
	if err := json.Unmarshal([]byte(cleanJSONResponse(raw)), &result); err != nil {
		return fmt.Errorf("invalid fact JSON")
	}
	if result.NewMemories == nil {
		return fmt.Errorf("missing new_memories array")
	}
	next.MemoryEntries = append([]MemoryEntry(nil), next.MemoryEntries...)
	for j := range next.MemoryEntries {
		next.MemoryEntries[j].References = append([]MemoryReference(nil), next.MemoryEntries[j].References...)
		for k := range next.MemoryEntries[j].References {
			if next.MemoryEntries[j].References[k].Chapter == ch.Num {
				next.MemoryEntries[j].References[k].Stale = true
			}
		}
	}
	next.NextMemoryID = max(next.NextMemoryID, nextMemoryID(next.MemoryEntries))
	seenFacts := map[int]bool{}
	// Old references remain as historical evidence until their replacement is valid.
	for index, nm := range *result.NewMemories {
		detail.index, detail.id = index, fmt.Sprint(nm.ID)
		detail.block, detail.inContext, detail.inCurrentKind = 0, visibleFacts[nm.ID], false
		detail.kind = "invalid"
		switch nm.Category {
		case "character", "location", "item", "event", "promise", "other":
			detail.kind = nm.Category
		}
		if len(nm.BlockIDs) > 0 {
			detail.block = nm.BlockIDs[0]
		}
		for _, m := range next.MemoryEntries {
			if nm.ID > 0 && m.ID == nm.ID {
				detail.inCurrentKind = m.Category == nm.Category
			}
		}
		if nm.ID > 0 && !visibleFacts[nm.ID] {
			return fmt.Errorf("invalid existing fact identity: id=%d was not supplied; use id=0 for new facts", nm.ID)
		}
		if nm.ID < 0 || strings.TrimSpace(nm.Content) == "" || len(nm.BlockIDs) == 0 {
			return fmt.Errorf("invalid fact evidence")
		}
		if detail.kind == "invalid" {
			return fmt.Errorf("invalid fact category")
		}
		target := -1
		for j, m := range next.MemoryEntries {
			if nm.ID > 0 && m.ID == nm.ID {
				target = j
				break
			}
			if nm.ID == 0 && m.Content == nm.Content {
				target = j
				break
			}
		}
		if nm.ID > 0 && (target < 0 || next.MemoryEntries[target].Content != nm.Content) {
			return fmt.Errorf("invalid existing fact identity: id=%d must preserve the supplied content exactly", nm.ID)
		}
		refs := []MemoryReference{}
		seen := map[int]bool{}
		for _, id := range nm.BlockIDs {
			detail.block = id
			bi := FindBlockIdx(&ch, id)
			if bi < 0 {
				return fmt.Errorf("invalid fact block")
			}
			if seen[id] {
				continue
			}
			seen[id] = true
			refs = append(refs, MemoryReference{Chapter: ch.Num, BlockID: id, Quote: ch.Blocks[bi].Text, ContentRev: rev})
		}
		if target < 0 {
			next.MemoryEntries = append(next.MemoryEntries, MemoryEntry{ID: next.NextMemoryID, Content: nm.Content, Category: nm.Category})
			next.NextMemoryID++
			target = len(next.MemoryEntries) - 1
		}
		m := next.MemoryEntries[target]
		if seenFacts[m.ID] {
			return fmt.Errorf("duplicate fact identity")
		}
		seenFacts[m.ID] = true
		kept := []MemoryReference{}
		for _, r := range m.References {
			if r.Chapter != ch.Num {
				kept = append(kept, r)
			}
		}
		m.References = append(kept, refs...)
		next.MemoryEntries[target] = m
	}
	return nil
}
