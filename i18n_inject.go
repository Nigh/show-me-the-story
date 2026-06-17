package main

import (
	"fmt"
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
	for i := 0; i < idx && i < len(state.Chapters); i++ {
		ch := state.Chapters[i]
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
	if NormalizeLanguage(lang) == LangEN {
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
	if NormalizeLanguage(lang) == LangEN {
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
	if NormalizeLanguage(lang) == LangEN {
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
			if NormalizeLanguage(lang) == LangEN {
				history += fmt.Sprintf("[Chapter %d summary]: %s\n", state.Chapters[i].Num, state.Chapters[i].Summary)
			} else {
				history += fmt.Sprintf("[第%d章摘要]: %s\n", state.Chapters[i].Num, state.Chapters[i].Summary)
			}
		}
	}
	if history == "" {
		if NormalizeLanguage(lang) == LangEN {
			history = "This is the opening of the story; no prior context."
		} else {
			history = "当前为故事开端，无历史前情。"
		}
	}
	return history
}

// buildCharacterContextForLang returns structured character details injected into writing prompts.
func buildCharacterContextForLang(settings *ProjectSettings, chapterOutline, lang string) string {
	if settings == nil || len(settings.Characters) == 0 {
		return ""
	}

	var relevant []Character
	for _, c := range settings.Characters {
		if strings.Contains(chapterOutline, c.Name) {
			relevant = append(relevant, c)
		}
	}
	if len(relevant) == 0 {
		relevant = settings.Characters
	}

	en := NormalizeLanguage(lang) == LangEN
	var sb strings.Builder
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

	return sb.String()
}

func buildWorldviewContextForLang(settings *ProjectSettings, chapterOutline, lang string) string {
	if settings == nil {
		return ""
	}

	en := NormalizeLanguage(lang) == LangEN
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

	en := NormalizeLanguage(lang) == LangEN
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
		if NormalizeLanguage(lang) == LangEN {
			return "(none)"
		}
		return "无"
	}

	en := NormalizeLanguage(lang) == LangEN
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
