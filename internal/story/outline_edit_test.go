package story

import "testing"

func TestEditChapterOutlineStatuses(t *testing.T) {
	mk := func(status string) *Progress {
		return &Progress{Chapters: []ChapterState{{Num: 1, Title: "旧", Outline: "旧大纲", Status: status}}}
	}
	for _, status := range []string{StatusPending, StatusWriting, StatusReview} {
		state := mk(status)
		if err := EditChapterOutline(state, 1, "新标题", "新大纲"); err != nil {
			t.Fatalf("status %s: unexpected err: %v", status, err)
		}
		if state.Chapters[0].Title != "新标题" || state.Chapters[0].Outline != "新大纲" {
			t.Fatalf("status %s: edit not applied: %+v", status, state.Chapters[0])
		}
	}
	state := mk(StatusAccepted)
	if err := EditChapterOutline(state, 1, "x", "y"); err == nil {
		t.Fatal("accepted chapter outline should be rejected")
	}
	if state.Chapters[0].Title != "旧" {
		t.Fatal("accepted chapter must not change on failed edit")
	}
}
