package httpapi

import (
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"showmethestory/internal/config"
	"showmethestory/internal/sse"
	"showmethestory/internal/story"
)

func TestContinuationProjectCopiesPlanningWithoutProofreadProse(t *testing.T) {
	root := t.TempDir()
	h := NewHandlers(&config.APIConfig{}, "", sse.NewLogBroadcaster(), root, "test")
	h.projectName = "source"
	h.cfg = config.DefaultConfig()
	h.cfg.Story.Title = "Novel"
	h.settings = &story.ProjectSettings{}
	h.state = &story.Progress{
		Title:          "Novel",
		BookStatus:     story.BookStatusCompleted,
		OutlineBatches: []story.OutlineBatch{{ID: 1, StartCh: 1, EndCh: 1, Synopsis: "batch", PlannedFinal: true}},
		Chapters:       []story.ChapterState{{Num: 1, Title: "One", Outline: "outline", Summary: "summary", Content: "proofread prose", Status: story.StatusAccepted}},
		MemoryEntries:  []story.MemoryEntry{{ID: 1, Content: "fact", References: []story.MemoryReference{{Chapter: 1, BlockID: 1}}}},
		Foreshadows:    []story.Foreshadow{{ID: 1, Name: "clue", Status: story.ForeshadowProgressing}},
	}

	res := httptest.NewRecorder()
	h.PostContinuationProject(res, httptest.NewRequest("POST", "/api/projects/continue", strings.NewReader(`{"name":"sequel"}`)))
	if res.Code != 200 {
		t.Fatalf("got %d: %s", res.Code, res.Body.String())
	}
	got, err := story.LoadProgress(filepath.Join(root, "storys", "sequel", "progress.json"))
	if err != nil {
		t.Fatal(err)
	}
	if got.BookStatus != story.BookStatusActive || got.CurrentChapterIndex != 1 || got.Chapters[0].Content != "" || got.Chapters[0].Outline != "outline" || !got.Chapters[0].Inherited {
		t.Fatalf("unexpected continuation state: %+v", got)
	}
	if got.OutlineBatches[0].PlannedFinal || len(got.MemoryEntries[0].References) != 0 || !got.MemoryEntries[0].Inherited || !got.Foreshadows[0].Inherited {
		t.Fatal("continuation retained terminal marker or stale prose anchors")
	}
	if total, accepted := chapterProgress(got.Chapters); total != 0 || accepted != 0 {
		t.Fatalf("inherited chapters counted in continuation progress: %d/%d", accepted, total)
	}
}
