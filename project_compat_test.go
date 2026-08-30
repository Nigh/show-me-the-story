package main

import (
	"context"
	"crypto/sha256"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

type canonicalSnapshot map[string][32]byte

func requireMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func requireWriteFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeLegacyFixture(t *testing.T, root, name, title string) string {
	t.Helper()
	projectDir := filepath.Join(root, "storys", name)
	requireMkdirAll(t, projectDir)
	requireWriteFile(t, filepath.Join(projectDir, "config.json"), []byte(`{
  "story": {
    "type":"悬疑","title":"`+title+`","chapter_count":12,
    "target_words_per_chapter":2400,"writing_style":"现实主义",
    "writing_pov":"第三人称限知"
  },
  "prompts":{"outline_generation":"保留的旧提示词"}
}`))
	requireWriteFile(t, filepath.Join(projectDir, "progress.json"), []byte(`{
  "phase":"writing","current_chapter_index":1,
  "chapters":[
    {"num":3,"title":"已确认章","content":"旧正文","status":"accepted"},
    {"num":4,"title":"审核章","content":"待审正文","status":"review"}
  ]
}`))
	requireWriteFile(t, filepath.Join(projectDir, "settings.json"), []byte(`{"characters":[],"worldview":[],"organizations":[],"relations":[]}`))
	requireWriteFile(t, filepath.Join(projectDir, "Chapter_03.md"), []byte("# 第三章\n\n旧正文\n"))
	return projectDir
}

func snapshotCanonicalFiles(t *testing.T, projectDir string) canonicalSnapshot {
	t.Helper()
	snapshot := canonicalSnapshot{}
	for _, name := range []string{"config.json", "progress.json", "settings.json", "Chapter_03.md"} {
		data, err := os.ReadFile(filepath.Join(projectDir, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		snapshot[name] = sha256.Sum256(data)
	}
	return snapshot
}

func TestLegacyProjectLoadsWithoutLosingProgress(t *testing.T) {
	root := t.TempDir()
	projectDir := writeLegacyFixture(t, root, "旧项目", "旧书")

	cfg, err := LoadConfig(filepath.Join(projectDir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	state, err := LoadProgress(filepath.Join(projectDir, "progress.json"))
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Story.Title != "旧书" || cfg.Language != "zh" || cfg.Story.ChapterCount != 12 {
		t.Fatalf("legacy config changed: %#v", cfg)
	}
	if state.Phase != "writing" || state.CurrentChapterIndex != 1 {
		t.Fatalf("legacy progress changed: %#v", state)
	}
	if len(state.Chapters) != 2 || state.Chapters[0].Status != StatusAccepted || state.Chapters[1].Status != StatusReview {
		t.Fatalf("review states changed: %#v", state.Chapters)
	}
}

func TestProjectReadEndpointsDoNotMutateCanonicalFiles(t *testing.T) {
	root := t.TempDir()
	projectDir := writeLegacyFixture(t, root, "旧项目", "旧书")
	logger := NewLogBroadcaster()
	defer logger.Close()
	h := NewHandlers(DefaultAPIConfig(), "", logger, root, "test")
	if err := h.switchProject("旧项目"); err != nil {
		t.Fatal(err)
	}
	before := snapshotCanonicalFiles(t, projectDir)

	requests := []struct {
		name    string
		handler http.HandlerFunc
		path    string
	}{
		{name: "current project", handler: h.GetProjectCurrent, path: "/api/projects/current"},
		{name: "config", handler: h.GetConfig, path: "/api/config"},
		{name: "progress", handler: h.GetProgress, path: "/api/progress"},
	}
	for _, tc := range requests {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			tc.handler(recorder, httptest.NewRequest(http.MethodGet, tc.path, nil))
			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
			}
		})
	}

	after := snapshotCanonicalFiles(t, projectDir)
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("read endpoints mutated canonical files: before=%v after=%v", before, after)
	}
}

func TestProjectSwitchKeepsEachProjectIsolated(t *testing.T) {
	root := t.TempDir()
	projectA := writeLegacyFixture(t, root, "项目甲", "甲书")
	projectB := writeLegacyFixture(t, root, "项目乙", "乙书")
	logger := NewLogBroadcaster()
	defer logger.Close()
	h := NewHandlers(DefaultAPIConfig(), "", logger, root, "test")

	if err := h.switchProject("项目甲"); err != nil {
		t.Fatal(err)
	}
	if h.cfg.Story.Title != "甲书" || h.state.CurrentChapterIndex != 1 {
		t.Fatalf("project A not loaded: cfg=%#v state=%#v", h.cfg, h.state)
	}
	beforeA := snapshotCanonicalFiles(t, projectA)

	if err := h.switchProject("项目乙"); err != nil {
		t.Fatal(err)
	}
	if h.cfg.Story.Title != "乙书" || h.state.CurrentChapterIndex != 1 {
		t.Fatalf("project B not loaded: cfg=%#v state=%#v", h.cfg, h.state)
	}
	beforeB := snapshotCanonicalFiles(t, projectB)

	if err := h.switchProject("项目甲"); err != nil {
		t.Fatal(err)
	}
	if h.cfg.Story.Title != "甲书" {
		t.Fatalf("project A title after return = %q", h.cfg.Story.Title)
	}
	if !reflect.DeepEqual(snapshotCanonicalFiles(t, projectA), beforeA) {
		t.Fatal("switching mutated project A")
	}
	if !reflect.DeepEqual(snapshotCanonicalFiles(t, projectB), beforeB) {
		t.Fatal("switching mutated project B")
	}
}

func TestLegacyReviewConfirmTransitionsPersist(t *testing.T) {
	projectDir := t.TempDir()
	progressPath := filepath.Join(projectDir, "progress.json")
	state := &Progress{
		Phase: "outline",
		Title: "旧书",
		Chapters: []ChapterState{
			{Num: 1, Title: "第一章", Outline: "开端", Status: StatusPending},
		},
	}

	if err := ConfirmOutlineAction(state, progressPath); err != nil {
		t.Fatal(err)
	}
	reloaded, err := LoadProgress(progressPath)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Phase != "writing" || reloaded.Chapters[0].Status != StatusPending {
		t.Fatalf("outline confirmation not persisted: %#v", reloaded)
	}

	state.Chapters[0].Content = "待审核正文"
	state.Chapters[0].Summary = "摘要"
	state.Chapters[0].Status = StatusReview
	SaveChapterMarkdown(projectDir, state.Chapters[0], state.Title)
	if err := SaveProgress(progressPath, state); err != nil {
		t.Fatal(err)
	}
	if err := ConfirmChapterAction(state, progressPath); err != nil {
		t.Fatal(err)
	}
	reloaded, err = LoadProgress(progressPath)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Chapters[0].Status != StatusAccepted || reloaded.CurrentChapterIndex != 1 {
		t.Fatalf("chapter confirmation not persisted: %#v", reloaded)
	}
	if data, err := os.ReadFile(ChapterMarkdownPath(projectDir, 1)); err != nil || len(data) == 0 {
		t.Fatalf("chapter markdown missing after confirmation: bytes=%d err=%v", len(data), err)
	}
}

func TestTaskCancellationLeavesCanonicalCheckpointUntouched(t *testing.T) {
	root := t.TempDir()
	projectDir := writeLegacyFixture(t, root, "旧项目", "旧书")
	logger := NewLogBroadcaster()
	defer logger.Close()
	h := NewHandlers(DefaultAPIConfig(), "", logger, root, "test")
	if err := h.switchProject("旧项目"); err != nil {
		t.Fatal(err)
	}
	before := snapshotCanonicalFiles(t, projectDir)

	ctx, cancel := context.WithCancel(context.Background())
	h.taskMu.Lock()
	h.taskRunning = true
	h.activeWork = 1
	h.taskCtx = ctx
	h.taskCancel = cancel
	h.taskMu.Unlock()
	defer h.endTask()

	recorder := httptest.NewRecorder()
	h.PostTaskStop(recorder, httptest.NewRequest(http.MethodPost, "/api/task/stop", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if ctx.Err() != context.Canceled {
		t.Fatalf("task context error = %v, want context.Canceled", ctx.Err())
	}
	if !reflect.DeepEqual(snapshotCanonicalFiles(t, projectDir), before) {
		t.Fatal("task cancellation mutated canonical checkpoint")
	}
}
