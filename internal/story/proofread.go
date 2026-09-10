package story

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"path/filepath"
	"showmethestory/internal/config"
	"showmethestory/internal/llm"
	"showmethestory/internal/sse"
	"sort"
	"strings"
	"time"
)

type ProofreadAnchor struct {
	ChapterNum int    `json:"chapter_num"`
	BlockID    int    `json:"block_id"`
	Excerpt    string `json:"excerpt"`
	ContentRev string `json:"content_rev"`
}

type ProofreadIssue struct {
	ID         string            `json:"id"`
	Category   string            `json:"category"`
	Severity   string            `json:"severity"`
	Title      string            `json:"title"`
	Detail     string            `json:"detail"`
	Suggestion string            `json:"suggestion"`
	Status     string            `json:"status"`
	Anchors    []ProofreadAnchor `json:"anchors"`
}

type ProofreadBlockChange struct {
	BlockID int    `json:"block_id"`
	Before  string `json:"before"`
	After   string `json:"after"`
}

type ProofreadRevision struct {
	ChapterNum int                    `json:"chapter_num"`
	AfterRev   string                 `json:"after_rev"`
	AppliedAt  string                 `json:"applied_at"`
	Changes    []ProofreadBlockChange `json:"changes"`
}

func proofreadIssueID(issue ProofreadIssue) string {
	h := fnv.New64a()
	fmt.Fprintf(h, "%s\x00%s", issue.Category, strings.ToLower(strings.TrimSpace(issue.Title)))
	for _, a := range issue.Anchors {
		fmt.Fprintf(h, "\x00%d:%d", a.ChapterNum, a.BlockID)
	}
	return fmt.Sprintf("issue_%x", h.Sum64())
}

func proofreadBlocks(ch ChapterState) string {
	var b strings.Builder
	for _, block := range ch.Blocks {
		fmt.Fprintf(&b, "<block id=\"%d\" type=\"%s\">%s</block>\n", block.ID, block.Type, block.Text)
	}
	return b.String()
}

func proofreadCall(ctx context.Context, api *config.APIConfig, system, prompt string, out any, logger *sse.LogBroadcaster) error {
	for attempt := 0; attempt < 2; attempt++ {
		raw := llm.CallAPIWithRetryLog(ctx, api, system, prompt, logger)
		if raw == "" {
			return fmt.Errorf("API 调用失败或被取消")
		}
		j := llm.ExtractJSON(raw)
		if j != "" && json.Unmarshal([]byte(j), out) == nil {
			return nil
		}
		prompt += "\n\nYour previous response was invalid. Return one complete JSON object only, using the required schema."
	}
	return fmt.Errorf("模型两次返回无效 JSON")
}

func AnalyzeProofread(ctx context.Context, api *config.APIConfig, cfg *config.Config, settings *ProjectSettings, state *Progress, pp *PostProcessState, logger *sse.LogBroadcaster) error {
	old := map[string]string{}
	for _, issue := range pp.Issues {
		old[issue.ID] = issue.Status
	}
	var issues []ProofreadIssue
	settingsText := buildAllSettingsText(cfg, settings, state)
	validCategory := map[string]bool{"logic": true, "fact": true, "structure": true, "rhythm": true, "foreshadow": true, "character": true, "other": true}
	validSeverity := map[string]bool{"critical": true, "important": true, "suggestion": true}
	for n := range state.Chapters {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		ch := &state.Chapters[n]
		SyncChapterBlocks(ch)
		logger.StepInfo(n+1, len(state.Chapters), fmt.Sprintf("正在检查第 %d 章的人工校订问题...", ch.Num))
		prompt := fmt.Sprintf(`Review this completed chapter only for issues that require human judgment or would require adding, deleting, splitting, merging, or moving paragraphs. Use the supplied settings, prior summaries, linked facts, and foreshadows to identify contradictions, but cite only blocks in the current chapter. Do not report ordinary spelling, grammar, wording, dialogue-naturalness, or style issues: a separate automatic proofreader handles those. Do not rewrite prose.

Return JSON: {"issues":[{"category":"logic|fact|structure|rhythm|foreshadow|character|other","severity":"critical|important|suggestion","title":"short title","detail":"what is wrong","suggestion":"human action","block_ids":[1]}]}. Every issue must cite one or more real block IDs below. Return {"issues":[]} when none.

Title: %s
Chapter %d: %s
Outline: %s
Summary: %s
Settings and foreshadows: %s
Prior summaries: %s
Linked facts: %s
Blocks:
		%s`, state.Title, ch.Num, ch.Title, ch.Outline, ch.Summary, settingsText, buildHistorySummaryForLang(state, n, cfg.Language), factProtection(state, ch.Num, cfg.Language), proofreadBlocks(*ch))
		var result struct {
			Issues []struct {
				Category   string `json:"category"`
				Severity   string `json:"severity"`
				Title      string `json:"title"`
				Detail     string `json:"detail"`
				Suggestion string `json:"suggestion"`
				BlockIDs   []int  `json:"block_ids"`
			} `json:"issues"`
		}
		if err := proofreadCall(ctx, api, "You are a conservative final-proofreading reviewer. Follow the requested JSON schema exactly and never rewrite the manuscript.", prompt, &result, logger); err != nil {
			return err
		}
		if result.Issues == nil {
			return fmt.Errorf("模型响应缺少 issues 数组")
		}
		byID := map[int]Block{}
		for _, b := range ch.Blocks {
			byID[b.ID] = b
		}
		for _, raw := range result.Issues {
			if !validCategory[raw.Category] || !validSeverity[raw.Severity] {
				continue
			}
			issue := ProofreadIssue{Category: raw.Category, Severity: raw.Severity, Title: strings.TrimSpace(raw.Title), Detail: strings.TrimSpace(raw.Detail), Suggestion: strings.TrimSpace(raw.Suggestion), Status: "pending"}
			for _, id := range raw.BlockIDs {
				if b, ok := byID[id]; ok {
					excerpt := []rune(strings.TrimSpace(b.Text))
					if len(excerpt) > 120 {
						excerpt = excerpt[:120]
					}
					issue.Anchors = append(issue.Anchors, ProofreadAnchor{ChapterNum: ch.Num, BlockID: id, Excerpt: string(excerpt), ContentRev: ChapterRevision(*ch)})
				}
			}
			if issue.Title == "" || len(issue.Anchors) == 0 {
				continue
			}
			sort.Slice(issue.Anchors, func(i, j int) bool { return issue.Anchors[i].BlockID < issue.Anchors[j].BlockID })
			issue.ID = proofreadIssueID(issue)
			if status := old[issue.ID]; status == "resolved" || status == "ignored" {
				issue.Status = status
			}
			issues = append(issues, issue)
		}
	}
	sort.SliceStable(issues, func(i, j int) bool { return issues[i].Anchors[0].ChapterNum < issues[j].Anchors[0].ChapterNum })
	pp.Issues, pp.ProofreadAnalyzedAt = issues, time.Now().Format(time.RFC3339)
	return nil
}

func RefreshProofreadAnchors(state *Progress, settings *ProjectSettings, ch *ChapterState) {
	rev := ChapterRevision(*ch)
	byID := map[int]string{}
	for _, b := range ch.Blocks {
		byID[b.ID] = b.Text
	}
	for i := range state.MemoryEntries {
		for j := range state.MemoryEntries[i].References {
			r := &state.MemoryEntries[i].References[j]
			if r.Chapter == ch.Num {
				if text, ok := byID[r.BlockID]; ok {
					r.Quote, r.ContentRev, r.Stale = text, rev, false
				} else {
					r.Stale = true
				}
			}
		}
	}
	for i := range settings.StoryChanges {
		r := &settings.StoryChanges[i].Source
		if r.Chapter == ch.Num {
			if text, ok := byID[r.BlockID]; ok {
				r.Quote, r.ContentRev, r.Stale = text, rev, false
			} else {
				r.Stale = true
			}
		}
	}
	ch.MemoryRevision = rev
	if settings.StorySynced == nil {
		settings.StorySynced = map[int]string{}
	}
	settings.StorySynced[ch.Num] = rev
}

func applyProofreadChapter(ctx context.Context, api *config.APIConfig, cfg *config.Config, state *Progress, settings *ProjectSettings, idx int, preferences, skills string, logger *sse.LogBroadcaster) ([]ProofreadBlockChange, error) {
	ch := &state.Chapters[idx]
	SyncChapterBlocks(ch)
	prompt := fmt.Sprintf(`Copyedit the completed chapter blocks below. Fix only spelling, grammar, awkward or repetitive wording, dialogue naturalness, and style consistency. Preserve every plot event, action, decision, fact, name, number, clue, reveal, foreshadow, paragraph boundary, and block order. Never add, delete, split, merge, or move content. Style preferences are subordinate to these rules.

Return JSON only: {"changes":[{"block_id":1,"text":"complete replacement text"}]}. Return changed blocks only. Do not modify scene_break blocks and do not put blank-line paragraph separators inside text.

Preferences: %s
Enabled style guidance: %s
Chapter %d: %s
Linked facts: %s
%s`, preferences, skills, ch.Num, ch.Title, factProtection(state, ch.Num, cfg.Language), proofreadBlocks(*ch))
	byID := map[int]int{}
	for i, b := range ch.Blocks {
		byID[b.ID] = i
	}
	for attempt := 0; attempt < 2; attempt++ {
		var result struct {
			Changes *[]struct {
				BlockID int    `json:"block_id"`
				Text    string `json:"text"`
			} `json:"changes"`
		}
		if err := proofreadCall(ctx, api, "You are a conservative final-manuscript copyeditor. Structural and semantic preservation rules are absolute.", prompt, &result, logger); err != nil {
			return nil, err
		}
		seen := map[int]bool{}
		var changes []ProofreadBlockChange
		valid := true
		if result.Changes == nil {
			valid = false
		}
		for _, c := range valueOrEmpty(result.Changes) {
			i, ok := byID[c.BlockID]
			if !ok || seen[c.BlockID] || ch.Blocks[i].Type == BlockSceneBreak || strings.TrimSpace(c.Text) == "" || strings.Contains(c.Text, ch.BlockSep) {
				valid = false
				break
			}
			seen[c.BlockID] = true
			if c.Text != ch.Blocks[i].Text {
				changes = append(changes, ProofreadBlockChange{BlockID: c.BlockID, Before: ch.Blocks[i].Text, After: c.Text})
			}
		}
		if !valid {
			prompt += "\n\nThe previous result changed paragraph structure or used invalid IDs. Correct it without changing block correspondence."
			continue
		}
		for _, c := range changes {
			ch.Blocks[byID[c.BlockID]].Text = c.After
		}
		rebuildContentFromBlocks(ch)
		return changes, nil
	}
	return nil, fmt.Errorf("校订结果两次破坏了段落对应关系")
}

func valueOrEmpty[T any](p *[]T) []T {
	if p == nil {
		return nil
	}
	return *p
}

func ApplyProofread(ctx context.Context, api *config.APIConfig, cfg *config.Config, state *Progress, settings *ProjectSettings, pp *PostProcessState, progressPath, settingsPath, postprocessPath string, skills []Skill, logger *sse.LogBroadcaster) error {
	style := FormatSkillsContent(ResolveSkills(skills, cfg.SkillConfig, SkillScopeBookExecute, cfg.Language))
	pp.ApplyErrors = map[int]string{}
	for i := range state.Chapters {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		stateSnapshot, _ := json.Marshal(state)
		settingsSnapshot, _ := json.Marshal(settings)
		postprocessSnapshot, _ := json.Marshal(pp)
		before := state.Chapters[i]
		logger.StepInfo(i+1, len(state.Chapters), fmt.Sprintf("正在校订第 %d 章...", before.Num))
		changes, err := applyProofreadChapter(ctx, api, cfg, state, settings, i, pp.AuthorRequirements, style, logger)
		if err != nil {
			_ = json.Unmarshal(stateSnapshot, state)
			_ = json.Unmarshal(settingsSnapshot, settings)
			pp.ApplyErrors[before.Num] = err.Error()
			continue
		}
		if len(changes) == 0 {
			continue
		}
		RefreshProofreadAnchors(state, settings, &state.Chapters[i])
		if err := SaveProjectSettings(settingsPath, settings); err != nil {
			_ = json.Unmarshal(stateSnapshot, state)
			_ = json.Unmarshal(settingsSnapshot, settings)
			return err
		}
		if err := SaveProgress(progressPath, state); err != nil {
			_ = json.Unmarshal(stateSnapshot, state)
			_ = json.Unmarshal(settingsSnapshot, settings)
			_ = SaveProjectSettings(settingsPath, settings)
			_ = SaveProgress(progressPath, state)
			return err
		}
		rev := ProofreadRevision{ChapterNum: before.Num, AfterRev: ChapterRevision(state.Chapters[i]), AppliedAt: time.Now().Format(time.RFC3339), Changes: changes}
		kept := pp.Revisions[:0]
		for _, old := range pp.Revisions {
			if old.ChapterNum != rev.ChapterNum {
				kept = append(kept, old)
			}
		}
		pp.Revisions = append(kept, rev)
		pp.ContentModified = true
		if err := SavePostProcess(postprocessPath, pp); err != nil {
			_ = json.Unmarshal(stateSnapshot, state)
			_ = json.Unmarshal(settingsSnapshot, settings)
			_ = json.Unmarshal(postprocessSnapshot, pp)
			_ = SaveProjectSettings(settingsPath, settings)
			_ = SaveProgress(progressPath, state)
			return err
		}
		SaveChapterMarkdown(filepath.Dir(progressPath), state.Chapters[i], state.Title)
	}
	pp.ProofreadAppliedAt = time.Now().Format(time.RFC3339)
	return nil
}

func UndoProofreadChapter(state *Progress, settings *ProjectSettings, pp *PostProcessState, chapterNum int) error {
	ri := -1
	for i, r := range pp.Revisions {
		if r.ChapterNum == chapterNum {
			ri = i
			break
		}
	}
	ci := FindChapterIdx(state, chapterNum)
	if ri < 0 || ci < 0 {
		return fmt.Errorf("没有可撤销的校订")
	}
	if ChapterRevision(state.Chapters[ci]) != pp.Revisions[ri].AfterRev {
		return fmt.Errorf("正文已被再次修改")
	}
	byID := map[int]int{}
	for i, b := range state.Chapters[ci].Blocks {
		byID[b.ID] = i
	}
	for _, c := range pp.Revisions[ri].Changes {
		i, ok := byID[c.BlockID]
		if !ok {
			return fmt.Errorf("段落结构已变化")
		}
		state.Chapters[ci].Blocks[i].Text = c.Before
	}
	rebuildContentFromBlocks(&state.Chapters[ci])
	RefreshProofreadAnchors(state, settings, &state.Chapters[ci])
	pp.Revisions = append(pp.Revisions[:ri], pp.Revisions[ri+1:]...)
	return nil
}
