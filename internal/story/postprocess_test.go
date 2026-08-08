package story

import (
	"strings"
	"testing"

	"showmethestory/internal/i18n"
)

func TestMergeChapterRoadmapFeedbackAuthorRequirements(t *testing.T) {
	items := []RoadmapItem{{
		ChapterNum: 1,
		Type:       RoadmapTypeStyle,
		Priority:   "P1",
		Feedback:   "收紧开篇比喻",
		Selected:   true,
		Status:     RoadmapStatusPending,
	}}
	_, feedback := mergeChapterRoadmapFeedback(items, nil, false, "统一把「微微一笑」改成具体表情", i18n.LangZH)
	if !strings.Contains(feedback, "收紧开篇比喻") {
		t.Fatalf("expected item feedback, got %q", feedback)
	}
	if !strings.Contains(feedback, "作者补充要求") || !strings.Contains(feedback, "微微一笑") {
		t.Fatalf("expected author requirements block, got %q", feedback)
	}
}

func TestMergeChapterRoadmapFeedbackAuthorForcesReviseOverPolishOnly(t *testing.T) {
	items := []RoadmapItem{{
		ChapterNum: 2,
		Type:       RoadmapTypePolish,
		Priority:   "P2",
		Feedback:   "去 AI 味",
		Selected:   true,
		Status:     RoadmapStatusPending,
	}}
	polishOnly, _ := mergeChapterRoadmapFeedback(items, nil, true, "", i18n.LangZH)
	if !polishOnly {
		t.Fatal("expected polish-only when no author requirements")
	}
	polishOnly, feedback := mergeChapterRoadmapFeedback(items, nil, true, "称呼一律用「大人」", i18n.LangZH)
	if polishOnly {
		t.Fatal("author requirements should force revise path")
	}
	if !strings.Contains(feedback, "称呼一律用「大人」") {
		t.Fatalf("expected author requirements in feedback, got %q", feedback)
	}
}

func TestFormatAuthorRequirementsEmpty(t *testing.T) {
	if formatAuthorRequirementsForRoadmap("  ", i18n.LangZH) != "" {
		t.Fatal("empty roadmap block")
	}
	if formatAuthorRequirementsForExecute("", i18n.LangEN) != "" {
		t.Fatal("empty execute block")
	}
}
