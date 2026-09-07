package story

import (
	"fmt"
	"showmethestory/internal/config"
	"showmethestory/internal/i18n"
	"strings"
)

// Helper functions that produce language-specific text fragments injected
// into prompt templates. These are NOT the prompt templates themselves
// (those live in prompts.go / prompts_en.go) — these are the runtime
// context blocks built from project state.

// buildOutlineConstraintsForLang returns the "全书章节脉络" reverse-constraint block
// in the requested language.
func buildOutlineConstraintsForLang(state *Progress, idx int, lang string) string {
	var past, future strings.Builder
	if idx >= 0 && idx < len(state.Chapters) {
		past.WriteString(chapterEnding(state, state.Chapters[idx].Num, lang))
	}
	// Arc-aware compression: chapters inside a summarized arc collapse to one
	// arc-summary line, so 1000-chapter books don't inject every past outline.
	summarized := func(num int) *Arc {
		arc := arcForChapterNum(state, num)
		if arc != nil && arc.Summary != "" {
			return arc
		}
		return nil
	}
	var lastArc *Arc
	for i := 0; i < idx && i < len(state.Chapters); i++ {
		ch := state.Chapters[i]
		if arc := summarized(ch.Num); arc != nil {
			if arc != lastArc {
				if i18n.NormalizeLanguage(lang) == i18n.LangEN {
					past.WriteString(fmt.Sprintf("[Arc \"%s\" (ch.%d-%d) summary] %s\n", arc.Title, arc.StartCh, arc.EndCh, arc.Summary))
				} else {
					past.WriteString(fmt.Sprintf("【《%s》卷（第%d~%d章）卷摘要】%s\n", arc.Title, arc.StartCh, arc.EndCh, arc.Summary))
				}
				lastArc = arc
			}
			continue
		}
		if strings.TrimSpace(ch.Outline) == "" {
			continue
		}
		past.WriteString(formatChapterLine(ch.Num, ch.Title, ch.Outline, lang))
	}
	end := idx + 1 + futureOutlineWindow
	if end > len(state.Chapters) {
		end = len(state.Chapters)
	}
	for i := idx + 1; i < end; i++ {
		ch := state.Chapters[i]
		if strings.TrimSpace(ch.Outline) == "" {
			continue
		}
		future.WriteString(formatChapterLine(ch.Num, ch.Title, ch.Outline, lang))
	}
	if past.Len() == 0 && future.Len() == 0 {
		return ""
	}
	var sb strings.Builder
	if i18n.NormalizeLanguage(lang) == i18n.LangEN {
		sb.WriteString("[Full-novel chapter arc (reverse constraint, must be obeyed strictly)]\n")
		if future.Len() > 0 {
			sb.WriteString("- Upcoming chapters — the following character debuts, first meetings, identity reveals etc. are already assigned to specific later chapters. This chapter MUST NOT make them happen early, nor hint at or spoil them:\n")
			sb.WriteString(future.String())
		}
		if past.Len() > 0 {
			sb.WriteString("- Already happened — the events below have already occurred. This chapter must not re-enact them as new events (especially one-time events like first meetings or identity reveals — only continue them as established facts):\n")
			sb.WriteString(past.String())
		}
	} else {
		sb.WriteString("【全书章节脉络（反向约束，必须严格遵守）】\n")
		if future.Len() > 0 {
			sb.WriteString("◆ 后续章节安排——以下人物登场、初遇、身份揭示等事件已安排在对应章节，本章严禁提前发生，也不得以任何形式暗示或剧透：\n")
			sb.WriteString(future.String())
		}
		if past.Len() > 0 {
			sb.WriteString("◆ 前文已发生——以下事件已经发生，本章不得将其作为新事件重复发生（尤其是初次见面、身份揭示等一次性事件，只能作为既成事实延续）：\n")
			sb.WriteString(past.String())
		}
	}
	sb.WriteString("\n")
	return sb.String()
}

func formatChapterLine(num int, title, outline, lang string) string {
	if i18n.NormalizeLanguage(lang) == i18n.LangEN {
		return fmt.Sprintf("Chapter %d \"%s\": %s\n", num, title, outline)
	}
	return fmt.Sprintf("第%d章《%s》：%s\n", num, title, outline)
}

func buildPreviousChapterTailForLang(state *Progress, idx int, lang string) string {
	if idx <= 0 || idx >= len(state.Chapters) {
		return ""
	}
	prev := state.Chapters[idx-1]
	if prev.Content == "" {
		return ""
	}
	tail := tailAtParagraph(prev.Content, prevTailMaxRunes)
	if tail == "" {
		return ""
	}
	if i18n.NormalizeLanguage(lang) == i18n.LangEN {
		return fmt.Sprintf("[Previous chapter ending (for seamless scene/mood continuation only — do NOT recap or rewrite)]\n%s\n\n", tail)
	}
	return fmt.Sprintf("【上一章结尾原文（仅供无缝承接场景与情绪，禁止复述或改写）】\n%s\n\n", tail)
}

func buildHistorySummaryForLang(state *Progress, idx int, lang string) string {
	startIdx := 0
	if idx > 5 {
		startIdx = idx - 5
	}
	var history string
	for i := startIdx; i < idx; i++ {
		if state.Chapters[i].Summary != "" {
			if i18n.NormalizeLanguage(lang) == i18n.LangEN {
				history += fmt.Sprintf("[Chapter %d summary]: %s\n", state.Chapters[i].Num, state.Chapters[i].Summary)
			} else {
				history += fmt.Sprintf("[第%d章摘要]: %s\n", state.Chapters[i].Num, state.Chapters[i].Summary)
			}
		}
	}
	if history == "" {
		if i18n.NormalizeLanguage(lang) == i18n.LangEN {
			history = "This is the opening of the story; no prior context."
		} else {
			history = "当前为故事开端，无历史前情。"
		}
	}
	return history
}

// buildCharacterContextForLang returns structured character details injected into writing prompts.
func buildCharacterContextForLang(settings *ProjectSettings, ch ChapterState, lang string) string {
	settings = settingsAtChapter(settings, ch.Num)
	var sb strings.Builder
	if settings != nil {
		for _, r := range settings.Relations {
			fmt.Fprintf(&sb, "[%s → %s] %s\n", r.SourceID, r.TargetID, r.Label)
		}
	}
	chapterOutline := ch.Outline

	if settings != nil && len(settings.Characters) > 0 {
		castNames := make(map[string]bool)
		for _, c := range normalizeOutlineCharacters(ch.Characters) {
			castNames[c.Name] = true
		}
		var relevant []Character
		for _, c := range settings.Characters {
			name := StripNameMarks(c.Name)
			if strings.Contains(chapterOutline, name) || castNames[name] {
				relevant = append(relevant, c)
			}
		}
		if len(relevant) == 0 {
			relevant = settings.Characters
		}

		en := i18n.NormalizeLanguage(lang) == i18n.LangEN
		for _, c := range relevant {
			sb.WriteString(fmt.Sprintf("【%s】", c.Name))
			if c.Age != "" {
				if en {
					sb.WriteString(fmt.Sprintf(" Age: %s", c.Age))
				} else {
					sb.WriteString(fmt.Sprintf(" 年龄:%s", c.Age))
				}
			}
			sb.WriteString("\n")
			write := func(label, val string) {
				if val == "" {
					return
				}
				sb.WriteString(fmt.Sprintf("  %s: %s\n", label, val))
			}
			if en {
				write("Appearance", c.Appearance)
				write("Personality", c.Personality)
				write("Background", c.Background)
				write("Motivation", c.Motivation)
				write("Abilities", c.Abilities)
				write("Notes", c.Notes)
			} else {
				write("外貌", c.Appearance)
				write("性格", c.Personality)
				write("背景", c.Background)
				write("动机", c.Motivation)
				write("能力", c.Abilities)
				write("备注", c.Notes)
			}
			sb.WriteString("\n")
		}
	}

	if derived := buildOutlineDerivedCharacterContext(ch, settings, lang); derived != "" {
		sb.WriteString(derived)
	}
	return sb.String()
}

func buildWorldviewContextForLang(settings *ProjectSettings, chapterOutline, lang string) string {
	if settings == nil {
		return ""
	}

	en := i18n.NormalizeLanguage(lang) == i18n.LangEN
	var sb strings.Builder

	if len(settings.Worldview) > 0 {
		var relevant []WorldviewEntry
		for _, w := range settings.Worldview {
			if strings.Contains(chapterOutline, w.Name) || strings.Contains(chapterOutline, w.Category) {
				relevant = append(relevant, w)
			}
		}
		if len(relevant) == 0 {
			relevant = settings.Worldview
		}
		for _, w := range relevant {
			sb.WriteString(fmt.Sprintf("【%s】(%s)\n  %s\n\n", w.Name, w.Category, w.Description))
		}
	}

	if len(settings.Organizations) > 0 {
		var relevantOrgs []Organization
		for _, o := range settings.Organizations {
			if strings.Contains(chapterOutline, o.Name) {
				relevantOrgs = append(relevantOrgs, o)
			}
		}
		if len(relevantOrgs) == 0 {
			relevantOrgs = settings.Organizations
		}
		for _, o := range relevantOrgs {
			if en {
				sb.WriteString(fmt.Sprintf("[Organization: %s] (%s)\n  %s\n", o.Name, o.Type, o.Description))
				if len(o.Members) > 0 {
					sb.WriteString(fmt.Sprintf("  Member IDs: %s\n", strings.Join(o.Members, ", ")))
				}
			} else {
				sb.WriteString(fmt.Sprintf("【组织:%s】(%s)\n  %s\n", o.Name, o.Type, o.Description))
				if len(o.Members) > 0 {
					sb.WriteString(fmt.Sprintf("  成员IDs: %s\n", strings.Join(o.Members, ", ")))
				}
			}
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

func chapterWorldview(settings *ProjectSettings, ch ChapterState, lang string) string {
	return buildWorldviewContextForLang(settingsAtChapter(settings, ch.Num), ch.Outline, lang)
}

// buildMemoryForLang renders the memory block for injection into writing/fact-check prompts.
func buildMemoryForLang(state *Progress, idx int, lang string) string {
	if len(state.MemoryEntries) == 0 {
		return ""
	}
	en := i18n.NormalizeLanguage(lang) == i18n.LangEN
	var sb strings.Builder
	if en {
		sb.WriteString("【Story Memory — long-term narrative details from earlier chapters】\n")
	} else {
		sb.WriteString("【叙事记忆——早期章节的关键叙事细节】\n")
	}
	for _, m := range state.MemoryEntries {
		if idx >= 0 && idx < len(state.Chapters) && m.Chapter > state.Chapters[idx].Num {
			continue
		}
		if len(m.References) > 0 && !memoryHasLiveReference(state, m) {
			continue
		}
		if state.MemoryMaxTokens > 0 && sb.Len()*3/2 > state.MemoryMaxTokens {
			break
		}
		snippet := extractSnippet(state, m.Chapter, m.Position, 100)
		if snippet != "" {
			if en {
				sb.WriteString(fmt.Sprintf("[Ch.%d] %s (original: \"%s\")\n", m.Chapter, m.Content, snippet))
			} else {
				sb.WriteString(fmt.Sprintf("[第%d章] %s（原文：「%s」）\n", m.Chapter, m.Content, snippet))
			}
		} else {
			if en {
				sb.WriteString(fmt.Sprintf("[Ch.%d] %s\n", m.Chapter, m.Content))
			} else {
				sb.WriteString(fmt.Sprintf("[第%d章] %s\n", m.Chapter, m.Content))
			}
		}
	}
	return sb.String()
}

// extractSnippet extracts approximately maxRunes characters from the chapter content
// starting at the given paragraph position (1-indexed, split by double newlines).
func extractSnippet(state *Progress, chapterNum, position, maxRunes int) string {
	if position <= 0 || chapterNum <= 0 {
		return ""
	}
	for i := range state.Chapters {
		if state.Chapters[i].Num == chapterNum {
			content := state.Chapters[i].Content
			if content == "" {
				return ""
			}
			paragraphs := strings.Split(content, "\n\n")
			idx := position - 1
			if idx < 0 || idx >= len(paragraphs) {
				return ""
			}
			para := strings.TrimSpace(paragraphs[idx])
			runes := []rune(para)
			if len(runes) > maxRunes {
				return string(runes[:maxRunes]) + "…"
			}
			return para
		}
	}
	return ""
}

// formatMemoryForUpdatePrompt renders the existing memory list for the memory update prompt.
func formatMemoryForUpdatePrompt(entries []MemoryEntry, lang string) string {
	if len(entries) == 0 {
		if i18n.NormalizeLanguage(lang) == i18n.LangEN {
			return "(empty — no memories yet)"
		}
		return "（空——尚无记忆）"
	}
	en := i18n.NormalizeLanguage(lang) == i18n.LangEN
	var sb strings.Builder
	for _, m := range entries {
		if en {
			sb.WriteString(fmt.Sprintf("#%d [%s] Ch.%d: %s\n", m.ID, m.Category, m.Chapter, m.Content))
		} else {
			sb.WriteString(fmt.Sprintf("#%d [%s] 第%d章: %s\n", m.ID, m.Category, m.Chapter, m.Content))
		}
	}
	return sb.String()
}

// formatActiveForeshadowsForChapterLang renders the "active foreshadows" block in the requested language.
func formatActiveForeshadowsForChapterLang(foreshadows []Foreshadow, chapterNum int, lang string) string {
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

	en := i18n.NormalizeLanguage(lang) == i18n.LangEN
	var sb strings.Builder
	if en {
		sb.WriteString("[Active foreshadows (you must advance or pay them off when writing)]\n")
	} else {
		sb.WriteString("【活跃伏笔（写作时必须注意推进或回收）】\n")
	}

	for _, fs := range active {
		if en {
			sb.WriteString(fmt.Sprintf("#%d \"%s\" [planted in chapter %d", fs.ID, fs.Name, fs.PlantChapter))
			if fs.TargetChapter > 0 {
				sb.WriteString(fmt.Sprintf(", expected payoff chapter %d", fs.TargetChapter))
			}
			sb.WriteString("]\n")
			sb.WriteString(fmt.Sprintf("   Description: %s\n", fs.Description))
		} else {
			sb.WriteString(fmt.Sprintf("#%d \"%s\" [第%d章埋设", fs.ID, fs.Name, fs.PlantChapter))
			if fs.TargetChapter > 0 {
				sb.WriteString(fmt.Sprintf("，预计第%d章回收", fs.TargetChapter))
			}
			sb.WriteString("]\n")
			sb.WriteString(fmt.Sprintf("   描述: %s\n", fs.Description))
		}

		if len(fs.Events) > 0 {
			if en {
				sb.WriteString("   Progress so far:\n")
				for _, ev := range fs.Events {
					sb.WriteString(fmt.Sprintf("   - Chapter %d: %s\n", ev.Chapter, ev.Note))
				}
			} else {
				sb.WriteString("   已有进展:\n")
				for _, ev := range fs.Events {
					sb.WriteString(fmt.Sprintf("   - 第%d章: %s\n", ev.Chapter, ev.Note))
				}
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
			if en {
				sb.WriteString(fmt.Sprintf("   ⚠️ This foreshadow is past its expected payoff chapter (%d); this chapter should prioritise paying it off.\n", fs.TargetChapter))
			} else {
				sb.WriteString(fmt.Sprintf("   ⚠️ 该伏笔已超过预计回收章节（第%d章），本章应优先考虑回收\n", fs.TargetChapter))
			}
		} else if fs.TargetChapter > 0 && chapterNum >= fs.TargetChapter-2 {
			if en {
				sb.WriteString(fmt.Sprintf("   → Approaching the expected payoff (chapter %d); start closing it.\n", fs.TargetChapter))
			} else {
				sb.WriteString(fmt.Sprintf("   → 接近预计回收节点（第%d章），可开始收束\n", fs.TargetChapter))
			}
		}

		sb.WriteString("\n")
	}

	return sb.String()
}

// formatForeshadowsForPromptLang renders the foreshadow list given to the update tracker.
func formatForeshadowsForPromptLang(foreshadows []Foreshadow, lang string) string {
	if len(foreshadows) == 0 {
		if i18n.NormalizeLanguage(lang) == i18n.LangEN {
			return "(none)"
		}
		return "无"
	}

	en := i18n.NormalizeLanguage(lang) == i18n.LangEN
	var sb strings.Builder
	for _, fs := range foreshadows {
		sb.WriteString(fmt.Sprintf("#%d [%s] %s\n", fs.ID, fs.Status, fs.Name))
		if en {
			sb.WriteString(fmt.Sprintf("   Description: %s\n", fs.Description))
			sb.WriteString(fmt.Sprintf("   Planted at: chapter %d", fs.PlantChapter))
			if fs.TargetChapter > 0 {
				sb.WriteString(fmt.Sprintf(", expected payoff: chapter %d", fs.TargetChapter))
			}
		} else {
			sb.WriteString(fmt.Sprintf("   描述: %s\n", fs.Description))
			sb.WriteString(fmt.Sprintf("   埋设于: 第%d章", fs.PlantChapter))
			if fs.TargetChapter > 0 {
				sb.WriteString(fmt.Sprintf("，预计回收: 第%d章", fs.TargetChapter))
			}
		}
		sb.WriteString("\n")

		if len(fs.Events) > 0 {
			if en {
				sb.WriteString("   Progress so far:\n")
				for _, ev := range fs.Events {
					sb.WriteString(fmt.Sprintf("   - Chapter %d: %s\n", ev.Chapter, ev.Note))
				}
			} else {
				sb.WriteString("   已有进展:\n")
				for _, ev := range fs.Events {
					sb.WriteString(fmt.Sprintf("   - 第%d章: %s\n", ev.Chapter, ev.Note))
				}
			}
		}

		if fs.Resolution != "" {
			if en {
				sb.WriteString(fmt.Sprintf("   Resolution: %s\n", fs.Resolution))
			} else {
				sb.WriteString(fmt.Sprintf("   回收方式: %s\n", fs.Resolution))
			}
		}

		sb.WriteString("\n")
	}

	return sb.String()
}

func BatchSynopses(state *Progress, lang string) string {
	var out strings.Builder
	for _, b := range state.OutlineBatches {
		out.WriteString(endingPrompt(b, 0, lang))
		if i18n.NormalizeLanguage(lang) == i18n.LangEN {
			fmt.Fprintf(&out, "[Batch %d, chapters %d–%d]\n%s\n\n", b.ID, b.StartCh, b.EndCh, b.Synopsis)
		} else {
			fmt.Fprintf(&out, "【批次 %d，第 %d–%d 章大纲梗概】\n%s\n\n", b.ID, b.StartCh, b.EndCh, b.Synopsis)
		}
	}
	return out.String()
}

func memoryLinkPrompt(lang string) string {
	if i18n.NormalizeLanguage(lang) == i18n.LangEN {
		return "\nRequired output override: return JSON {\"new_memories\":[{\"id\":0,\"content\":\"fact\",\"category\":\"character|location|item|event|promise|other\",\"block_ids\":[1]}]}. Extract all consistency-critical facts, including those in the outline. Link each fact to ALL relevant blocks of THIS chapter using the supplied block IDs, not paragraph positions. For an existing fact reuse its exact ID and content; use id=0 only for a new fact. Include existing facts mentioned again. Do not delete, merge or alter existing facts to meet a token budget. Return an empty array only when no facts apply. Block evidence:\n"
	}
	return "\n输出格式覆盖：返回 JSON {\"new_memories\":[{\"id\":0,\"content\":\"事实\",\"category\":\"character|location|item|event|promise|other\",\"block_ids\":[1]}]}。提取所有影响一致性的关键事实，包括大纲中已有的事实。每个事实关联本章所有相关段落，使用下方真实 Block ID，不是段落序号。复用已有事实时保持其 ID 和 content 原文；只有新事实才用 id=0。本章再次提到的已有事实也要返回。不得为了 token 预算删除、合并或改变既有事实。只有确实没有事实时返回空数组。段落证据：\n"
}

func settingUpdatePrompt(lang string) string {
	schema := "\nJSON: {\"changes\":[{\"kind\":\"characters|worldview|organizations|relations\",\"entity\":{\"id\":\"existing ID, or empty for new\"},\"block_id\":1,\"evolution\":false,\"conflict\":false,\"reason\":\"evidence explanation\"}]}\n"
	if i18n.NormalizeLanguage(lang) == i18n.LangEN {
		return "Extract setting changes evidenced in the accepted chapter blocks. Reuse existing entity IDs; never guess identities. Supply only changed fields for existing entities, complete required fields for new entities. Mark contradictions, uncertain identities or inferences conflict=true. evolution=true only for explicit chronological developments (e.g. allies becoming enemies), never for factual contradictions. No deletions. New characters: name; worldview: name/category/description; organizations: name/type/description/members; relations: source_id/source_type/target_id/target_type/label (types character/worldview/organization). For new entities use local IDs starting with $ (e.g. $alice); list new characters/worldview first, organizations next, relations last. References may use existing IDs or earlier unambiguous new $IDs. Never link uncertain new entities. The server allocates permanent IDs. Use exact block IDs as evidence. Return changes:[] when nothing changes. Below: existing entities, then chapter blocks." + schema
	}
	return "从已确认正文段落中提取有证据的设定变化。已有实体复用 ID，只提交变化字段；新实体提供必填字段。禁止猜测身份。矛盾、身份含糊或推断必须 conflict=true。只有正文明确发生的时间演变（如盟友变敌人）才设 evolution=true，事实矛盾不算演变。禁止删除。新人物：name；世界观：name/category/description；组织：name/type/description/members；关系：source_id/source_type/target_id/target_type/label（类型 character/worldview/organization）。新实体使用 $ 开头的临时 ID，如 $alice，先输出人物和世界观，再组织，再关系；引用可用已有 ID 或之前输出的无歧义新实体 $ID，禁止关联尚有歧义的新实体。服务端分配正式 ID。block_id 必须使用真实段落证据。没有变化返回 changes:[]。下方依次为已有设定和本章段落。" + schema
}

func BookSynopsis(cfg *config.Config, state *Progress) string {
	if len(state.OutlineBatches) > 0 {
		return BatchSynopses(state, cfg.Language)
	}
	return preferUserValue(cfg.Story.StorySynopsis, state.StorySynopsis)
}

func ChapterSynopsis(cfg *config.Config, state *Progress, num int) string {
	for _, b := range state.OutlineBatches {
		if num >= b.StartCh && num <= b.EndCh {
			if i18n.NormalizeLanguage(cfg.Language) == i18n.LangEN {
				return fmt.Sprintf("[Batch synopsis, chapters %d–%d; follow this chapter's outline without advancing later events]\n%s", b.StartCh, b.EndCh, b.Synopsis)
			}
			return fmt.Sprintf("【第 %d–%d 章大纲梗概；仅按本章章纲推进，不得提前展开后续情节】\n%s", b.StartCh, b.EndCh, b.Synopsis)
		}
	}
	return preferUserValue(cfg.Story.StorySynopsis, state.StorySynopsis)
}

func batchScopeTemplate(lang string) string {
	if i18n.NormalizeLanguage(lang) == i18n.LangEN {
		return "\n[Required batch synopsis: chapters {{.StartNum}}–{{.EndNum}}]\n{{.OutlineSynopsis}}\n[Long-term direction, optional]\n{{.LongTermDirection}}\nGenerate exactly {{.NewChapterCount}} consecutive chapters in this range, constrained by this batch synopsis. Start the story when there are no existing chapters; otherwise continue the existing plot. Do not treat this batch as the whole book."
	} else {
		return "\n【本批大纲梗概：第 {{.StartNum}}–{{.EndNum}} 章，必须遵循】\n{{.OutlineSynopsis}}\n【长期方向（可选）】\n{{.LongTermDirection}}\n严格生成上述范围内连续的 {{.NewChapterCount}} 章，由本批梗概约束。没有已有章节时从故事开篇开始，否则承接已有剧情。不得把本批梗概当成全书计划。"
	}
}

func endingPrompt(b OutlineBatch, num int, lang string) string {
	if b.EndingIntent == "" && !b.PlannedFinal {
		return ""
	}
	if i18n.NormalizeLanguage(lang) == i18n.LangEN {
		if !b.PlannedFinal {
			return "\n[Ending control] Continue serialization. Close the local beat while leaving a natural next step; do not finish the entire book.\n"
		}
		s := fmt.Sprintf("\n[Ending control] Plan the batch toward a book ending at chapter %d. Current chapter: %d (0 means batch planning). Do not finish early. At the final chapter, ending requirements override generic cliffhanger rules. Resolve the main conflict.\n", b.EndCh, num)
		if b.EndingIntent == "sequel" {
			s += "Complete this book's arc and preserve a concrete entry point for a sequel.\n"
		}
		if b.EndingStyle == "open" {
			s += "Open ending: establish the main outcome, leaving the future or thematic interpretation open.\n"
		} else if b.EndingStyle != "custom" {
			s += "Closed ending: settle the main conflict and principal character arcs.\n"
		}
		return s + b.EndingRequirements + "\n"
	}
	if !b.PlannedFinal {
		return "\n【结尾控制】继续连载：完成局部情节并留下自然的下一步，不要写成全书完结。\n"
	}
	s := fmt.Sprintf("\n【结尾控制】本批向第 %d 章全书收尾逐步推进。当前章：%d（0 表示批次规划）。不得提前完结；末章结尾要求优先于通用章末钩子规则，须交代主线结果。\n", b.EndCh, num)
	if b.EndingIntent == "sequel" {
		s += "完成本书主线，同时保留明确的续作入口。\n"
	}
	if b.EndingStyle == "open" {
		s += "开放式结局：交代主线结果，人物未来或主题解释可以留白。\n"
	} else if b.EndingStyle != "custom" {
		s += "闭合式结局：收束主线冲突及主要人物弧线。\n"
	}
	return s + b.EndingRequirements + "\n"
}
