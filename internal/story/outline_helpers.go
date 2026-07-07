package story

import (
	"fmt"
	"regexp"
	"showmethestory/internal/config"
	"showmethestory/internal/i18n"
	"strings"
	"unicode/utf8"
)

const outlineGenMaxAttempts = 2

// calcOutlineLengthRange returns recommended per-chapter outline length bounds (characters)
// scaled to planned chapter prose length. ponytail: linear heuristic; tune divisors if field feels too short/long.
func calcOutlineLengthRange(targetWordsPerChapter int) (minLen, maxLen int) {
	if targetWordsPerChapter < 1 {
		targetWordsPerChapter = 2500
	}
	minLen = targetWordsPerChapter / 20
	if minLen < 80 {
		minLen = 80
	}
	maxLen = targetWordsPerChapter / 8
	if maxLen < 150 {
		maxLen = 150
	}
	if maxLen < minLen+20 {
		maxLen = minLen + 20
	}
	return minLen, maxLen
}

func outlineRuneLen(s string) int {
	return utf8.RuneCountInString(s)
}

func validateOutlineChapterLengths(chapters []OutlineChapter, minLen int) []int {
	var short []int
	for _, ch := range chapters {
		if outlineRuneLen(strings.TrimSpace(ch.Outline)) < minLen {
			short = append(short, ch.Num)
		}
	}
	return short
}

func formatCharacterListForOutline(settings *ProjectSettings, lang string) string {
	en := i18n.NormalizeLanguage(lang) == i18n.LangEN
	if settings == nil || len(settings.Characters) == 0 {
		if en {
			return "(No characters registered yet. You may introduce new characters only when necessary; mark each debut with \"first appearance\" and include a one-line role or relationship to the protagonist.)"
		}
		return "（尚未在角色管理中登记角色。仅因剧情需要方可引入新人物；须在其首次出场章节标注「首次登场」，并附一行身份或与主角关系说明。）"
	}

	var sb strings.Builder
	if en {
		sb.WriteString("Registered characters (prefer these; avoid duplicates):\n")
	} else {
		sb.WriteString("已登记角色（优先使用，避免功能重复）：\n")
	}
	for _, c := range settings.Characters {
		sb.WriteString(fmt.Sprintf("- %s", c.Name))
		if c.Personality != "" {
			if en {
				sb.WriteString(fmt.Sprintf(" — personality: %s", c.Personality))
			} else {
				sb.WriteString(fmt.Sprintf(" — 性格：%s", c.Personality))
			}
		} else if c.Background != "" {
			if en {
				sb.WriteString(fmt.Sprintf(" — background: %s", truncateRunes(c.Background, 40)))
			} else {
				sb.WriteString(fmt.Sprintf(" — 背景：%s", truncateRunes(c.Background, 40)))
			}
		}
		sb.WriteString("\n")
	}
	if en {
		sb.WriteString("\nOnly add unlisted characters when the plot requires it; mark \"first appearance\" in their debut chapter with a one-line description.")
	} else {
		sb.WriteString("\n仅当剧情需要时方可新增未登记角色；在其首次出场章节标注「首次登场」并附一行说明。")
	}
	return sb.String()
}

func formatOutlineLengthRequirementBlock(minLen, maxLen int, lang string) string {
	if i18n.NormalizeLanguage(lang) == i18n.LangEN {
		return fmt.Sprintf("Each chapter outline must be %d–%d characters (not counting the chapter title). Outlines shorter than %d characters are unacceptable.", minLen, maxLen, minLen)
	}
	return fmt.Sprintf("每章 outline 字段正文须为 %d–%d 字（不含章节标题）。低于 %d 字视为不合格。", minLen, maxLen, minLen)
}

func formatOutlineStructureRequirementBlock(lang string) string {
	if i18n.NormalizeLanguage(lang) == i18n.LangEN {
		return "Each chapter outline must cover, in order: opening scene/location; core conflict or goal; key turning point or revelation; characters appearing (with roles); how the chapter ends or what hook it leaves."
	}
	return "每章大纲须依次包含：开场场景/地点；本章核心冲突或目标；关键转折或信息点；出场人物（及作用）；章末走向或悬念钩子。"
}

func buildOutlinePromptExtras(cfg *config.Config, settings *ProjectSettings) map[string]string {
	minLen, maxLen := calcOutlineLengthRange(cfg.Story.TargetWordsPerChapter)
	return map[string]string{
		"OutlineMinWords": fmt.Sprintf("%d", minLen),
		"OutlineMaxWords": fmt.Sprintf("%d", maxLen),
		"CharacterList":   formatCharacterListForOutline(settings, cfg.Language),
	}
}

func mergeOutlinePromptData(base map[string]string, cfg *config.Config, settings *ProjectSettings) map[string]string {
	merged := make(map[string]string, len(base)+3)
	for k, v := range base {
		merged[k] = v
	}
	for k, v := range buildOutlinePromptExtras(cfg, settings) {
		merged[k] = v
	}
	return merged
}

func finalizeOutlinePrompt(template, rendered string, cfg *config.Config, settings *ProjectSettings) string {
	lang := cfg.Language
	minLen, maxLen := calcOutlineLengthRange(cfg.Story.TargetWordsPerChapter)
	rendered = appendIfMissingPlaceholder(template, rendered, "{{.CharacterList}}", formatCharacterListForOutline(settings, lang))
	if !strings.Contains(template, "{{.OutlineMinWords}}") {
		block := formatOutlineLengthRequirementBlock(minLen, maxLen, lang) + "\n" + formatOutlineStructureRequirementBlock(lang)
		rendered += "\n\n" + block
	}
	return rendered
}

func formatShortOutlineRetryFeedback(shortNums []int, minLen int, lang string) string {
	nums := make([]string, len(shortNums))
	for i, n := range shortNums {
		nums[i] = fmt.Sprintf("%d", n)
	}
	joined := strings.Join(nums, ", ")
	if i18n.NormalizeLanguage(lang) == i18n.LangEN {
		return fmt.Sprintf("\n\nIMPORTANT: Chapters %s have outlines shorter than %d characters. Expand them with concrete plot beats (scene, conflict, turning point, characters, ending hook) and resubmit the full JSON.", joined, minLen)
	}
	return fmt.Sprintf("\n\n重要：第 %s 章大纲不足 %d 字。请补充具体情节（场景、冲突、转折、人物、章末钩子）后重新输出完整 JSON。", joined, minLen)
}

func truncateRunes(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}

func RegisteredCharacterNameSet(settings *ProjectSettings) map[string]bool {
	names := make(map[string]bool)
	if settings == nil {
		return names
	}
	for _, c := range settings.Characters {
		name := strings.TrimSpace(StripNameMarks(c.Name))
		if name != "" {
			names[name] = true
		}
	}
	return names
}

type outlineCharacterStub struct {
	Name        string
	Description string
}

var (
	firstAppearanceZH = regexp.MustCompile(`([^，。；：\n（(【\[]+?)[（(【\[]?首次登场[）)】\]]?`)
	firstAppearanceEN = regexp.MustCompile(`(?i)([\p{Han}A-Za-z·\s]{2,20}?)\s*[\(（]?\s*first appearance\s*[\)）]?`)
)

func extractFirstAppearanceStubs(outline string) []outlineCharacterStub {
	outline = strings.TrimSpace(outline)
	if outline == "" {
		return nil
	}
	var stubs []outlineCharacterStub
	seen := make(map[string]bool)

	addStub := func(name string) {
		name = strings.TrimSpace(StripNameMarks(name))
		name = strings.Trim(name, "：:、 ")
		if name == "" || seen[name] {
			return
		}
		seen[name] = true
		desc := extractStubDescription(outline, name)
		stubs = append(stubs, outlineCharacterStub{Name: name, Description: desc})
	}

	for _, m := range firstAppearanceZH.FindAllStringSubmatch(outline, -1) {
		if len(m) > 1 {
			addStub(m[1])
		}
	}
	for _, m := range firstAppearanceEN.FindAllStringSubmatch(outline, -1) {
		if len(m) > 1 {
			addStub(m[1])
		}
	}
	return stubs
}

func extractStubDescription(outline, name string) string {
	idx := strings.Index(outline, name)
	if idx < 0 {
		return ""
	}
	rest := outline[idx+len(name):]
	rest = strings.TrimLeft(rest, "（(【[")
	if cut := strings.IndexAny(rest, "。.\n"); cut >= 0 && cut < 120 {
		return strings.TrimSpace(rest[:cut])
	}
	return truncateRunes(strings.TrimSpace(rest), 80)
}

func buildOutlineDerivedCharacterContext(chapterOutline string, settings *ProjectSettings, lang string) string {
	registered := RegisteredCharacterNameSet(settings)
	stubs := extractFirstAppearanceStubs(chapterOutline)
	if len(stubs) == 0 {
		return ""
	}

	en := i18n.NormalizeLanguage(lang) == i18n.LangEN
	var sb strings.Builder
	for _, stub := range stubs {
		if registered[stub.Name] {
			continue
		}
		if en {
			sb.WriteString(fmt.Sprintf("[Outline-only character: %s]\n", stub.Name))
			if stub.Description != "" {
				sb.WriteString(fmt.Sprintf("  From outline: %s\n", stub.Description))
			}
		} else {
			sb.WriteString(fmt.Sprintf("【大纲人物：%s】\n", stub.Name))
			if stub.Description != "" {
				sb.WriteString(fmt.Sprintf("  大纲描述：%s\n", stub.Description))
			}
		}
		sb.WriteString("\n")
	}
	return sb.String()
}
