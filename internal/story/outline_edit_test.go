package story

import "testing"

func TestEditChapterOutlineStatuses(t *testing.T) {
	mk := func(status string) *Progress {
		return &Progress{Chapters: []ChapterState{{Num: 1, Title: "旧", Outline: "旧大纲", Status: status}}}
	}
	for _, status := range []string{StatusPending, StatusWriting, StatusReview} {
		state := mk(status)
		if err := EditChapterOutline(state, 1, "新标题", "新大纲", nil); err != nil {
			t.Fatalf("status %s: unexpected err: %v", status, err)
		}
		if state.Chapters[0].Title != "新标题" || state.Chapters[0].Outline != "新大纲" {
			t.Fatalf("status %s: edit not applied: %+v", status, state.Chapters[0])
		}
	}
	state := mk(StatusAccepted)
	if err := EditChapterOutline(state, 1, "x", "y", nil); err == nil {
		t.Fatal("accepted chapter outline should be rejected")
	}
	if state.Chapters[0].Title != "旧" {
		t.Fatal("accepted chapter must not change on failed edit")
	}
}

func TestEditChapterOutlineCharacters(t *testing.T) {
	state := &Progress{Chapters: []ChapterState{{
		Num: 1, Title: "旧", Outline: "旧大纲", Status: StatusPending,
		Characters: []OutlineChapterCharacter{{Name: "旧角"}},
	}}}
	if err := EditChapterOutline(state, 1, "新", "新大纲", nil); err != nil {
		t.Fatal(err)
	}
	if len(state.Chapters[0].Characters) != 1 || state.Chapters[0].Characters[0].Name != "旧角" {
		t.Fatalf("nil characters pointer must leave cast unchanged: %+v", state.Chapters[0].Characters)
	}
	chars := []OutlineChapterCharacter{
		{Name: "吕红梅", FirstAppearance: true, Note: "班主任"},
		{Name: "  "},
	}
	if err := EditChapterOutline(state, 1, "新", "新大纲", &chars); err != nil {
		t.Fatal(err)
	}
	if len(state.Chapters[0].Characters) != 1 || state.Chapters[0].Characters[0].Name != "吕红梅" {
		t.Fatalf("characters not applied: %+v", state.Chapters[0].Characters)
	}
	empty := []OutlineChapterCharacter{}
	if err := EditChapterOutline(state, 1, "新", "新大纲", &empty); err != nil {
		t.Fatal(err)
	}
	if state.Chapters[0].Characters != nil {
		t.Fatalf("empty slice should clear cast, got %+v", state.Chapters[0].Characters)
	}
}

func TestContinuationOutlineAllowed(t *testing.T) {
	cases := []struct {
		phase string
		n     int
		want  bool
	}{
		{"outline", 1, true},
		{"writing", 10, true},
		{"outline", 0, true},
		{"writing", 0, true},
		{"", 5, true},
		{"done", 5, false},
	}
	for _, c := range cases {
		if got := ContinuationOutlineAllowed(c.phase, c.n); got != c.want {
			t.Fatalf("ContinuationOutlineAllowed(%q, %d) = %v, want %v", c.phase, c.n, got, c.want)
		}
	}
}
