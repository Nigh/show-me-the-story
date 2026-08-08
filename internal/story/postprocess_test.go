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

func TestPlanExecuteBatchesAuthorCoversAllChapters(t *testing.T) {
	chapters := []ChapterState{{Num: 1}, {Num: 2}, {Num: 3}}
	roadmap := []RoadmapItem{
		{ChapterNum: 2, Type: RoadmapTypeStyle, Feedback: "修第二章", Selected: true, Status: RoadmapStatusPending},
		{ChapterNum: 2, Type: RoadmapTypeLogic, Feedback: "逻辑", Selected: false, Status: RoadmapStatusPending},
	}
	batches := planExecuteBatches(chapters, roadmap, "统一称呼")
	if len(batches) != 3 {
		t.Fatalf("want 3 batches, got %d", len(batches))
	}
	if batches[0].ChapterNum != 1 || len(batches[0].Indices) != 0 {
		t.Fatalf("ch1: %+v", batches[0])
	}
	if batches[1].ChapterNum != 2 || len(batches[1].Indices) != 1 || batches[1].Indices[0] != 0 {
		t.Fatalf("ch2 should merge one selected ticket, got %+v", batches[1])
	}
	if batches[2].ChapterNum != 3 || len(batches[2].Indices) != 0 {
		t.Fatalf("ch3: %+v", batches[2])
	}

	noAuthor := planExecuteBatches(chapters, roadmap, "")
	if len(noAuthor) != 1 || noAuthor[0].ChapterNum != 2 {
		t.Fatalf("without author req, only selected tickets: %+v", noAuthor)
	}
}

func TestMergeChapterRoadmapFeedbackAuthorOnly(t *testing.T) {
	polishOnly, feedback := mergeChapterRoadmapFeedback(nil, nil, false, "全书统一用「大人」", i18n.LangZH)
	if polishOnly || !strings.Contains(feedback, "大人") {
		t.Fatalf("author-only chapter: polishOnly=%v feedback=%q", polishOnly, feedback)
	}
}
