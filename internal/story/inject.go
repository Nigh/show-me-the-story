package story

import (
	"encoding/json"
	"fmt"
	"showmethestory/internal/config"
	"showmethestory/internal/i18n"
	"sort"
	"strings"
	"unicode/utf8"
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
	recentStart := max(0, idx-historyRecentChapters)
	for i := recentStart; i < idx && i < len(state.Chapters); i++ {
		ch := state.Chapters[i]
		if strings.TrimSpace(ch.Outline) == "" {
			continue
		}
		past.WriteString(formatChapterLine(ch.Num, ch.Title, ch.Outline, lang))
	}
	old := []knowledgeDocument{}
	if idx > recentStart {
		query := chapterKnowledgeQuery(state.Chapters[idx])
		for i := 0; i < recentStart; i++ {
			ch := state.Chapters[i]
			old = append(old, knowledgeDocument{text: formatChapterLine(ch.Num, ch.Title, ch.Outline, lang)})
		}
		for _, i := range packKnowledge(old, rankKnowledge(old, query), 5000) {
			past.WriteString(old[i].text)
		}
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
	return truncateRunesExact(sb.String(), 12000)
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
	startIdx := max(0, idx-historyRecentChapters)
	parts := []string{}
	if idx > 0 {
		parts = append(parts, formatCheckpointHistory(state, state.Chapters[startIdx].Num, historyPromptRunes/2, lang))
	}
	for i := startIdx; i < idx; i++ {
		if state.Chapters[i].Summary != "" {
			if i18n.NormalizeLanguage(lang) == i18n.LangEN {
				parts = append(parts, fmt.Sprintf("[Chapter %d summary]: %s", state.Chapters[i].Num, state.Chapters[i].Summary))
			} else {
				parts = append(parts, fmt.Sprintf("[第%d章摘要]: %s", state.Chapters[i].Num, state.Chapters[i].Summary))
			}
		}
	}
	history := boundedHistoryFallback(parts, historyPromptRunes)
	if history == "" {
		if i18n.NormalizeLanguage(lang) == i18n.LangEN {
			history = "This is the opening of the story; no prior context."
		} else {
			history = "当前为故事开端，无历史前情。"
		}
	}
	return history
}

// buildCharacterContextForLang retrieves chapter-relevant original settings.
func buildCharacterContextForLang(settings *ProjectSettings, ch ChapterState, lang string) string {
	snapshot := settingsAtChapter(settings, ch.Num)
	selected, _ := retrieveSettings(snapshot, chapterKnowledgeQuery(ch), settingsContextRunes)
	selected.Worldview, selected.Organizations = nil, nil
	data, _ := json.Marshal(settingsEntities(selected))
	derived := buildOutlineDerivedCharacterContext(ch, snapshot, lang)
	if utf8.RuneCountInString(derived) > 1000 {
		derived = ""
	}
	return knowledgeSelectionNotice(lang) + string(data) + "\n" + derived
}

func buildWorldviewContextForLang(settings *ProjectSettings, chapterOutline, lang string) string {
	selected, _ := retrieveSettings(settings, chapterOutline, settingsContextRunes)
	selected.Characters, selected.Relations = nil, nil
	data, _ := json.Marshal(settingsEntities(selected))
	return knowledgeSelectionNotice(lang) + string(data)
}

func chapterWorldview(settings *ProjectSettings, ch ChapterState, lang string) string {
	return buildWorldviewContextForLang(settingsAtChapter(settings, ch.Num), chapterKnowledgeQuery(ch), lang)
}

func buildChapterSettingsContexts(settings *ProjectSettings, ch ChapterState, lang string) (string, string) {
	snapshot := settingsAtChapter(settings, ch.Num)
	selected, _ := retrieveSettings(snapshot, chapterKnowledgeQuery(ch), settingsContextRunes)
	characters := *selected
	characters.Worldview, characters.Organizations = nil, nil
	world := *selected
	world.Characters, world.Relations = nil, nil
	charData, _ := json.Marshal(settingsEntities(&characters))
	worldData, _ := json.Marshal(settingsEntities(&world))
	derived := buildOutlineDerivedCharacterContext(ch, snapshot, lang)
	if utf8.RuneCountInString(derived) > 1000 {
		derived = ""
	}
	notice := knowledgeSelectionNotice(lang)
	return notice + string(charData) + "\n" + derived, notice + string(worldData)
}

// An explicit notice prevents a retrieved subset being mistaken for the full registry.
func knowledgeSelectionNotice(lang string) string {
	if i18n.NormalizeLanguage(lang) == i18n.LangEN {
		return "\n[Retrieved original records; this is a budgeted subset, not the complete registry. Missing records do not imply absence of a fact/entity. Do not infer new identities or overwrite unseen fields. Evidence snippets may be excerpts.]\n"
	}
	return "\n【检索到的原始条目：这是预算内的相关子集，并非完整资料库。未命中不代表事实或实体不存在，不得据此猜测新身份或覆盖未提供的字段；来源片段可能是节选。】\n"
}

// The optional query includes freshly generated prose during fact checking.
func buildMemoryForLang(state *Progress, idx int, lang string, query ...string) string {
	if idx < 0 || idx >= len(state.Chapters) || len(state.MemoryEntries) == 0 {
		return ""
	}
	ch := state.Chapters[idx]
	if len(query) > 0 {
		ch.Content = query[0]
	}
	budget := memoryContextRunes
	if state.MemoryMaxTokens > 0 {
		budget = min(budget, state.MemoryMaxTokens*2/3)
	}
	notice := knowledgeSelectionNotice(lang)
	remaining := budget - utf8.RuneCountInString(notice) - 2
	if remaining <= 0 {
		return ""
	}
	entries := retrieveMemories(state, ch, remaining, true)
	data, _ := json.Marshal(entries)
	return notice + string(data)
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
			sb.WriteString(fmt.Sprintf("#%d [%s]: %s\n", m.ID, m.Category, m.Content))
		} else {
			sb.WriteString(fmt.Sprintf("#%d [%s]: %s\n", m.ID, m.Category, m.Content))
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
	sort.SliceStable(active, func(i, j int) bool {
		due := func(fs Foreshadow) int {
			if fs.TargetChapter <= 0 {
				return 1 << 30
			}
			return fs.TargetChapter
		}
		return due(active[i]) < due(active[j])
	})
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
			events := fs.Events[max(0, len(fs.Events)-3):]
			if en {
				sb.WriteString("   Progress so far:\n")
				for _, ev := range events {
					sb.WriteString(fmt.Sprintf("   - Chapter %d: %s\n", ev.Chapter, ev.Note))
				}
			} else {
				sb.WriteString("   已有进展:\n")
				for _, ev := range events {
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

	return truncateRunesExact(sb.String(), 6000)
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
		if fs.Status == ForeshadowResolved || fs.Status == ForeshadowAbandoned {
			continue
		}
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
			events := fs.Events[max(0, len(fs.Events)-3):]
			if en {
				sb.WriteString("   Progress so far:\n")
				for _, ev := range events {
					sb.WriteString(fmt.Sprintf("   - Chapter %d: %s\n", ev.Chapter, ev.Note))
				}
			} else {
				sb.WriteString("   已有进展:\n")
				for _, ev := range events {
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

	if sb.Len() == 0 {
		if en {
			return "(none)"
		}
		return "无"
	}
	return truncateRunesExact(sb.String(), 6000)
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

func memoryRetryPrompt(lang string, err error) string {
	if i18n.NormalizeLanguage(lang) == i18n.LangEN {
		return "\nThe previous response failed validation: " + err.Error() + ". Return the complete corrected JSON. Only reuse IDs from the supplied existing memories and copy their content exactly. For new or changed facts use id=0. Do not invent IDs or merge existing facts.\n"
	}
	return "\n上次输出校验失败：" + err.Error() + "。请返回完整的修正后 JSON。只能复用所提供已有记忆的 ID，并逐字复制其 content；新事实或发生变化的事实用 id=0。不得编造 ID 或合并已有事实。\n"
}

func settingUpdatePrompt(lang string) string {
	schema := "\nJSON: {\"changes\":[{\"kind\":\"characters|worldview|organizations|relations\",\"entity\":{\"id\":\"existing ID, or empty for new\"},\"block_id\":1,\"evolution\":false,\"conflict\":false,\"reason\":\"evidence explanation\"}]}\n"
	if i18n.NormalizeLanguage(lang) == i18n.LangEN {
		schema += "Keep fields concise: describe enduring attributes and current state, not a chapter-by-chapter plot log. Preserve existing independent constraints when updating a field. Do not append paraphrases of known facts. Event history and original evidence are stored separately.\n"
	} else {
		schema += "字段保持精简：只描述稳定属性与当前状态，不写逐章剧情流水账。更新字段必须保留原有独立约束，不追加已有事实的同义复述。事件历史和原文证据由系统另行保存。\n"
	}
	if i18n.NormalizeLanguage(lang) == i18n.LangEN {
		return "Extract setting changes evidenced in the accepted chapter blocks. Reuse existing entity IDs; never guess identities. Supply only changed fields for existing entities, complete required fields for new entities. Mark contradictions, uncertain identities or inferences conflict=true. evolution=true only for explicit chronological developments (e.g. allies becoming enemies), never for factual contradictions. No deletions. New characters: name; worldview: name/category/description; organizations: name/type/description/members; relations: source_id/source_type/target_id/target_type/label (types character/worldview/organization). For new entities use local IDs starting with $ (e.g. $alice); list new characters/worldview first, organizations next, relations last. References may use existing IDs or earlier unambiguous new $IDs. Never link uncertain new entities. The server allocates permanent IDs. Use exact block IDs as evidence. Return changes:[] when nothing changes. Below: existing entities, then chapter blocks." + schema
	}
	return "从已确认正文段落中提取有证据的设定变化。已有实体复用 ID，只提交变化字段；新实体提供必填字段。禁止猜测身份。矛盾、身份含糊或推断必须 conflict=true。只有正文明确发生的时间演变（如盟友变敌人）才设 evolution=true，事实矛盾不算演变。禁止删除。新人物：name；世界观：name/category/description；组织：name/type/description/members；关系：source_id/source_type/target_id/target_type/label（类型 character/worldview/organization）。新实体使用 $ 开头的临时 ID，如 $alice，先输出人物和世界观，再组织，再关系；引用可用已有 ID 或之前输出的无歧义新实体 $ID，禁止关联尚有歧义的新实体。服务端分配正式 ID。block_id 必须使用真实段落证据。没有变化返回 changes:[]。下方依次为已有设定和本章段落。" + schema
}

func BookSynopsis(cfg *config.Config, state *Progress) string {
	return BatchSynopses(state, cfg.Language)
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
	return ""
}

func batchScopeTemplate(lang string) string {
	if i18n.NormalizeLanguage(lang) == i18n.LangEN {
		return "\n[Required batch synopsis: chapters {{.StartNum}}–{{.EndNum}}]\n{{.OutlineSynopsis}}\n[Long-term direction, optional]\n{{.LongTermDirection}}\nGenerate exactly {{.NewChapterCount}} consecutive chapters in this range, constrained by this batch synopsis. Start the story when there are no existing chapters; otherwise continue the existing plot. Do not treat this batch as the whole book. When the supplied novel title is blank, include an inferred non-empty top-level JSON field named title."
	} else {
		return "\n【本批大纲梗概：第 {{.StartNum}}–{{.EndNum}} 章，必须遵循】\n{{.OutlineSynopsis}}\n【长期方向（可选）】\n{{.LongTermDirection}}\n严格生成上述范围内连续的 {{.NewChapterCount}} 章，由本批梗概约束。没有已有章节时从故事开篇开始，否则承接已有剧情。不得把本批梗概当成全书计划。传入的小说标题为空时，须推断标题并在顶层 JSON 的 title 字段返回非空标题。"
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
