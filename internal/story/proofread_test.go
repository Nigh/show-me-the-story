package story

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProofreadAnchorRefreshAndUndo(t *testing.T) {
	ch := ChapterState{Num: 1, Content: "Old wording.", Status: StatusAccepted}
	SyncChapterBlocks(&ch)
	state := &Progress{Chapters: []ChapterState{ch}, MemoryEntries: []MemoryEntry{{ID: 1, References: []MemoryReference{{Chapter: 1, BlockID: ch.Blocks[0].ID, Quote: ch.Content}}}}}
	settings := &ProjectSettings{StorySynced: map[int]string{}, StoryChanges: []SettingChange{{ID: 1, Source: MemoryReference{Chapter: 1, BlockID: ch.Blocks[0].ID, Quote: ch.Content}}}}
	state.Chapters[0].Blocks[0].Text = "Better wording."
	rebuildContentFromBlocks(&state.Chapters[0])
	RefreshProofreadAnchors(state, settings, &state.Chapters[0])
	if !ReferenceLive(state, state.MemoryEntries[0].References[0]) || settings.StoryChanges[0].Source.Quote != "Better wording." {
		t.Fatal("proofreading did not refresh stable-block anchors")
	}
	missing := MemoryReference{Chapter: 1, BlockID: 999, Quote: "deleted"}
	state.MemoryEntries = append(state.MemoryEntries, MemoryEntry{ID: 2, References: []MemoryReference{missing}})
	RefreshProofreadAnchors(state, settings, &state.Chapters[0])
	if !state.MemoryEntries[1].References[0].Stale {
		t.Fatal("deleted-block reference was not marked stale")
	}
	pp := NewProofreadState()
	pp.Revisions = []ProofreadRevision{{ChapterNum: 1, AfterRev: ChapterRevision(state.Chapters[0]), Changes: []ProofreadBlockChange{{BlockID: ch.Blocks[0].ID, Before: "Old wording.", After: "Better wording."}}}}
	if err := UndoProofreadChapter(state, settings, pp, 1); err != nil {
		t.Fatal(err)
	}
	if state.Chapters[0].Content != "Old wording." || len(pp.Revisions) != 0 {
		t.Fatalf("undo mismatch: %#v", state.Chapters[0])
	}
}

func TestUnsupportedPostprocessVersionIsRejected(t *testing.T) {
	path := filepath.Join(t.TempDir(), "postprocess.json")
	if err := os.WriteFile(path, []byte(`{"diagnosis_report":"legacy","roadmap":[{"id":"old"}]}`), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadPostProcess(path); err == nil {
		t.Fatal("unsupported postprocess schema was accepted")
	}
}
