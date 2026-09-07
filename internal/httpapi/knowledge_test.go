package httpapi

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"showmethestory/internal/config"
	"showmethestory/internal/sse"
	"showmethestory/internal/story"
	"strings"
	"testing"
)

func factHandlers(t *testing.T) *Handlers {
	t.Helper()
	h := NewHandlers(&config.APIConfig{}, "", sse.NewLogBroadcaster(), t.TempDir(), "test")
	h.progressPath = filepath.Join(t.TempDir(), "progress.json")
	h.state.Chapters = []story.ChapterState{{Num: 1, Content: "Alice has a scar.", Status: story.StatusReview, KnowledgeTracked: true}}
	story.SyncChapterBlocks(&h.state.Chapters[0])
	h.state.MemoryEntries = []story.MemoryEntry{{ID: 1, Chapter: 1, Content: "Alice has a scar", References: []story.MemoryReference{{Chapter: 1, BlockID: 1, Quote: "Alice has a scar.", ContentRev: story.ChapterRevision(h.state.Chapters[0])}}}}
	return h
}
func TestFactEditHTTPGuards(t *testing.T) {
	for _, method := range []string{"PUT", "DELETE", "POST"} {
		h := factHandlers(t)
		body := `{"text":"Alice has no scar.","feedback":"Remove the scar."}`
		req := httptest.NewRequest(method, "/api/chapters/1/blocks/1", strings.NewReader(body))
		req.SetPathValue("num", "1")
		req.SetPathValue("id", "1")
		req.Header.Set("X-UI-Locale", "en")
		res := httptest.NewRecorder()
		switch method {
		case "PUT":
			h.PutChapterBlock(res, req)
		case "DELETE":
			h.DeleteChapterBlock(res, req)
		case "POST":
			h.PostChapterBlockRevise(res, req)
		}
		if res.Code != http.StatusConflict {
			t.Fatalf("%s: %d %s", method, res.Code, res.Body)
		}
		if h.state.Chapters[0].Content != "Alice has a scar." || h.isTaskRunning() {
			t.Fatal("rejected edit mutated state or leaked task lock")
		}
		req = httptest.NewRequest(method, "/api/chapters/1/blocks/1", strings.NewReader(body))
		req.SetPathValue("num", "1")
		req.SetPathValue("id", "1")
		req.Header.Set("X-Content-Rev", "stale")
		req.Header.Set("X-Confirm-Fact-Impact", "true")
		res = httptest.NewRecorder()
		switch method {
		case "PUT":
			h.PutChapterBlock(res, req)
		case "DELETE":
			h.DeleteChapterBlock(res, req)
		case "POST":
			h.PostChapterBlockRevise(res, req)
		}
		if res.Code != http.StatusConflict {
			t.Fatal("outdated confirmation accepted")
		}
	}
}
func TestRejectedBatchDoesNotRunPendingKnowledge(t *testing.T) {
	h := factHandlers(t)
	before := h.state.Chapters[0].MemoryRevision
	req := httptest.NewRequest("POST", "/api/outline/generate-continuation", strings.NewReader(`{"chapter_count":0}`))
	res := httptest.NewRecorder()
	h.PostOutlineGenerateContinuation(res, req)
	if res.Code != 400 || h.state.Chapters[0].MemoryRevision != before || h.isTaskRunning() {
		t.Fatal("invalid batch ran pending work")
	}
}
func TestKnowledgeQueryReturnsEvidenceAndPending(t *testing.T) {
	h := factHandlers(t)
	req := httptest.NewRequest("GET", "/api/knowledge?chapter=1", nil)
	res := httptest.NewRecorder()
	h.GetKnowledge(res, req)
	if res.Code != 200 || !strings.Contains(res.Body.String(), `"block_id":1`) || !strings.Contains(res.Body.String(), `"pending_chapters":[1]`) {
		t.Fatal(res.Body.String())
	}
}
