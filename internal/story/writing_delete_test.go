package story

import (
	"os"
	"path/filepath"
	"reflect"
	"showmethestory/internal/fsutil"
	"strings"
	"testing"
)

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
		MemoryEntries: []MemoryEntry{{ID: 1, Content: "detail", References: []MemoryReference{{Chapter: 3}}}},
	}

	num, err := DeleteFrontierChapter(state, filepath.Join(t.TempDir(), "progress.json"))
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

func TestRestartChapterPersistsAndPreservesOtherSources(t *testing.T) {
	path := filepath.Join(t.TempDir(), "progress.json")
	state := &Progress{Phase: "writing", Chapters: []ChapterState{{Num: 1, Title: "Title", Outline: "Outline", Status: StatusReview, Content: "Old prose.", Summary: "old", NextBlockID: 8}}, MemoryEntries: []MemoryEntry{
		{ID: 1, References: []MemoryReference{{Chapter: 1}}},
		{ID: 2, References: []MemoryReference{{Chapter: 1}, {Chapter: 2}}},
		{ID: 3, Inherited: true, References: []MemoryReference{{Chapter: 1}}},
	}}
	if err := SaveProgress(path, state); err != nil {
		t.Fatal(err)
	}
	nextID := state.Chapters[0].NextBlockID
	SaveChapterMarkdown(filepath.Dir(path), state.Chapters[0], "Title")
	if _, err := DeleteFrontierChapter(state, path); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadProgress(path)
	if err != nil {
		t.Fatal(err)
	}
	ch := loaded.Chapters[0]
	if ch.Content != "" || ch.Summary != "" || len(ch.Blocks) != 0 || ch.Status != StatusPending || ch.Title != "Title" || ch.Outline != "Outline" || ch.NextBlockID != nextID {
		t.Fatalf("bad reset: %+v", ch)
	}
	if len(loaded.MemoryEntries) != 2 || len(loaded.MemoryEntries[0].References) != 1 || loaded.MemoryEntries[0].References[0].Chapter != 2 {
		t.Fatal("lost other sources or kept deleted references", loaded.MemoryEntries)
	}
	if _, err := os.Stat(ChapterMarkdownPath(filepath.Dir(path), 1)); !os.IsNotExist(err) {
		t.Fatal("derived markdown retained", err)
	}
}

func TestRestartChapterSaveFailurePreservesOriginal(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "progress.json")
	state := &Progress{Phase: "writing", Chapters: []ChapterState{{Num: 1, Content: "Original.", Summary: "Summary", Status: StatusReview}}, MemoryEntries: []MemoryEntry{{ID: 1, References: []MemoryReference{{Chapter: 1}}}}}
	if err := SaveProgress(path, state); err != nil {
		t.Fatal(err)
	}
	SaveChapterMarkdown(dir, state.Chapters[0], "Title")
	old, _ := os.ReadFile(ChapterMarkdownPath(dir, 1))
	// A directory at the metadata path forces a transaction failure after staging prose.
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0755); err != nil {
		t.Fatal(err)
	}
	before := *state
	if _, err := DeleteFrontierChapter(state, path); err == nil {
		t.Fatal("expected save error")
	} else if _, ok := fsutil.AsSaveError(err); !ok {
		t.Fatal("lost storage diagnostics", err)
	}
	if !reflect.DeepEqual(before, *state) {
		t.Fatal("failed reset changed memory")
	}
	got, _ := os.ReadFile(ChapterMarkdownPath(dir, 1))
	if string(got) != string(old) {
		t.Fatal("failed reset removed markdown")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	// Original chapter bytes remain intact even though metadata was deliberately made unavailable.
	data, err := os.ReadFile(filepath.Join(dir, "chapters", "000001.json"))
	if err != nil || !strings.Contains(string(data), "Original.") {
		t.Fatal("original chapter lost", err)
	}
}

func TestConfirmEmptyChapter(t *testing.T) {
	for _, content := range []string{"", " \n\t"} {
		state := &Progress{Phase: "writing", Chapters: []ChapterState{{Num: 1, Status: StatusReview, Content: content}}}
		if err := ConfirmChapterAction(state, filepath.Join(t.TempDir(), "progress.json")); err != ErrEmptyChapter {
			t.Fatal("empty chapter accepted", err)
		}
		if state.CurrentChapterIndex != 0 || state.Chapters[0].Status != StatusReview {
			t.Fatal("failed confirmation changed state")
		}
	}
}
