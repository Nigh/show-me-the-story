package story

import "testing"

func TestResolveForceReviewIndex(t *testing.T) {
	state := &Progress{
		CurrentChapterIndex: 1,
		Chapters: []ChapterState{
			{Num: 1, Status: StatusAccepted},
			{Num: 2, Status: StatusWriting, Content: "draft"},
		},
		PendingWritingConflict: &WritingConflict{ChapterIndex: 1, ChapterNum: 2},
	}
	idx, err := ResolveForceReviewIndex(state)
	if err != nil || idx != 1 {
		t.Fatalf("with conflict: idx=%d err=%v", idx, err)
	}

	state.PendingWritingConflict = nil
	idx, err = ResolveForceReviewIndex(state)
	if err != nil || idx != 1 {
		t.Fatalf("orphan writing: idx=%d err=%v", idx, err)
	}

	state.Chapters[1].Status = StatusReview
	if _, err := ResolveForceReviewIndex(state); err == nil {
		t.Fatal("expected error when no conflict and not writing")
	}
}

func TestPromoteWritingToReviewClearsConflict(t *testing.T) {
	state := &Progress{
		Chapters:               []ChapterState{{Num: 1, Status: StatusWriting}},
		PendingWritingConflict: &WritingConflict{ChapterIndex: 0, ChapterNum: 1},
	}
	if err := PromoteWritingToReview(state, 0); err != nil {
		t.Fatal(err)
	}
	if state.Chapters[0].Status != StatusReview {
		t.Fatalf("status=%s, want review", state.Chapters[0].Status)
	}
	if state.PendingWritingConflict != nil {
		t.Fatal("conflict should be cleared")
	}
}
