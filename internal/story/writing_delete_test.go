package story

import "testing"

func TestResolveDeleteChapterTarget(t *testing.T) {
	ch := func(num int, status, content string) ChapterState {
		return ChapterState{Num: num, Status: status, Content: content}
	}

	tests := []struct {
		name    string
		state   Progress
		wantIdx int
		wantErr error
	}{
		{
			name:    "empty",
			state:   Progress{},
			wantErr: ErrNoChaptersToDelete,
		},
		{
			name: "review at frontier",
			state: Progress{
				Phase:               "writing",
				CurrentChapterIndex: 2,
				Chapters: []ChapterState{
					ch(1, StatusAccepted, "a"),
					ch(2, StatusAccepted, "b"),
					ch(3, StatusReview, "c"),
					ch(4, StatusPending, ""),
				},
			},
			wantIdx: 2,
		},
		{
			name: "accepted frontier when next pending",
			state: Progress{
				Phase:               "writing",
				CurrentChapterIndex: 3,
				Chapters: []ChapterState{
					ch(1, StatusAccepted, "a"),
					ch(2, StatusAccepted, "b"),
					ch(3, StatusAccepted, "c"),
					ch(4, StatusPending, ""),
				},
			},
			wantIdx: 2,
		},
		{
			name: "book complete deletes last",
			state: Progress{
				Phase:               "writing",
				CurrentChapterIndex: 3,
				Chapters: []ChapterState{
					ch(1, StatusAccepted, "a"),
					ch(2, StatusAccepted, "b"),
					ch(3, StatusAccepted, "c"),
				},
			},
			wantIdx: 2,
		},
		{
			name: "writing blocks delete",
			state: Progress{
				Phase:               "writing",
				CurrentChapterIndex: 2,
				Chapters: []ChapterState{
					ch(1, StatusAccepted, "a"),
					ch(2, StatusAccepted, "b"),
					ch(3, StatusWriting, "c"),
				},
			},
			wantErr: ErrWritingChapterCannotDelete,
		},
		{
			name: "nothing written yet",
			state: Progress{
				Phase:               "writing",
				CurrentChapterIndex: 0,
				Chapters: []ChapterState{
					ch(1, StatusPending, ""),
					ch(2, StatusPending, ""),
				},
			},
			wantErr: ErrDeleteFrontierUnavailable,
		},
		{
			name: "review at frontier not earlier accepted",
			state: Progress{
				Phase:               "writing",
				CurrentChapterIndex: 2,
				Chapters: []ChapterState{
					ch(1, StatusAccepted, "a"),
					ch(2, StatusAccepted, "b"),
					ch(3, StatusReview, "c"),
				},
			},
			wantIdx: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			idx, err := ResolveDeleteChapterTarget(&tt.state)
			if tt.wantErr != nil {
				if err != tt.wantErr {
					t.Fatalf("err = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if idx != tt.wantIdx {
				t.Fatalf("idx = %d, want %d", idx, tt.wantIdx)
			}
		})
	}
}

func TestDeleteFrontierChapterAdjustsPointer(t *testing.T) {
	state := &Progress{
		Phase:               "writing",
		CurrentChapterIndex: 3,
		Chapters: []ChapterState{
			{Num: 1, Status: StatusAccepted, Content: "a"},
			{Num: 2, Status: StatusAccepted, Content: "b"},
			{Num: 3, Status: StatusAccepted, Content: "c", Summary: "s"},
			{Num: 4, Status: StatusPending},
		},
		MemoryEntries: []MemoryEntry{{ID: 1, Chapter: 3, Content: "detail"}},
	}

	num, err := DeleteFrontierChapter(state, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if num != 3 {
		t.Fatalf("deleted num = %d, want 3", num)
	}
	if state.CurrentChapterIndex != 2 {
		t.Fatalf("pointer = %d, want 2", state.CurrentChapterIndex)
	}
	if state.Chapters[2].Status != StatusPending || state.Chapters[2].Content != "" {
		t.Fatalf("chapter 3 not cleared: %+v", state.Chapters[2])
	}
	if len(state.MemoryEntries) != 0 {
		t.Fatalf("memory not purged: %+v", state.MemoryEntries)
	}
}
