package story

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStorageRoundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "progress.json")

	p := &Progress{
		Phase: "writing",
		Title: "测试书",
		Chapters: []ChapterState{
			{Num: 1, Title: "第一章", Outline: "开局", Content: "第一段。\n\n第二段。", Summary: "摘要1", Status: StatusAccepted},
			{Num: 2, Title: "第二章", Outline: "冲突", Content: "", Status: StatusPending},
		},
		CurrentChapterIndex: 1,
		MemoryEntries: []MemoryEntry{
			{ID: 1, Content: "主角捡到怀表", Category: "item", Chapter: 1, Position: 2},
		},
	}

	if err := SaveProgress(path, p); err != nil {
		t.Fatalf("SaveProgress: %v", err)
	}

	// Metadata file must not contain prose content.
	meta, _ := os.ReadFile(path)
	if strings.Contains(string(meta), "第一段。") {
		t.Fatal("progress.json 不应包含正文")
	}

	// Chapter file exists with content.
	if _, err := os.Stat(chapterFilePath(path, 1)); err != nil {
		t.Fatalf("章节文件缺失: %v", err)
	}

	// Roundtrip restores content.
	resetCache(path)
	loaded, err := LoadProgress(path)
	if err != nil {
		t.Fatalf("LoadProgress: %v", err)
	}
	if loaded.Chapters[0].Content != "第一段。\n\n第二段。" {
		t.Fatalf("正文丢失: %q", loaded.Chapters[0].Content)
	}
	if loaded.Chapters[0].WordCount == 0 {
		t.Fatal("WordCount 未计算")
	}
	if loaded.Chapters[1].Content != "" {
		t.Fatal("空章节不应有正文")
	}
}

func TestSaveProgressDirtyCheck(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "progress.json")
	p := &Progress{Phase: "writing", Chapters: []ChapterState{
		{Num: 1, Title: "A", Content: "原文", Status: StatusAccepted},
	}}
	if err := SaveProgress(path, p); err != nil {
		t.Fatal(err)
	}
	ch1 := chapterFilePath(path, 1)
	st1, _ := os.Stat(ch1)

	// Save again without changes: chapter file must not be rewritten.
	// Corrupt-proof check: delete the file; unchanged content should be skipped
	// only when hash cache says so — so instead compare mtime via rewrite marker.
	os.Chtimes(ch1, st1.ModTime(), st1.ModTime())
	if err := SaveProgress(path, p); err != nil {
		t.Fatal(err)
	}
	st2, _ := os.Stat(ch1)
	if !st2.ModTime().Equal(st1.ModTime()) {
		t.Fatal("未变更的章节文件被重写")
	}

	// Change content: file must be rewritten.
	p.Chapters[0].Content = "改过的正文"
	if err := SaveProgress(path, p); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(ch1)
	var cf chapterFile
	json.Unmarshal(data, &cf)
	if cf.Content != "改过的正文" {
		t.Fatalf("章节文件未更新: %q", cf.Content)
	}
}

func TestSaveProgressOrphanCleanup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "progress.json")
	p := &Progress{Phase: "outline", Chapters: []ChapterState{
		{Num: 1, Content: "a", Status: StatusPending},
		{Num: 2, Content: "b", Status: StatusPending},
	}}
	if err := SaveProgress(path, p); err != nil {
		t.Fatal(err)
	}
	p.Chapters = p.Chapters[:1]
	if err := SaveProgress(path, p); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(chapterFilePath(path, 2)); !os.IsNotExist(err) {
		t.Fatal("孤儿章节文件未清理")
	}
}

func TestProgressViewStripsContent(t *testing.T) {
	p := &Progress{Chapters: []ChapterState{
		{Num: 1, Content: "正文内容", Status: StatusAccepted},
	}}
	v := ProgressView(p)
	if v.Chapters[0].Content != "" {
		t.Fatal("view 不应包含正文")
	}
	if v.Chapters[0].ContentRev == "" || v.Chapters[0].WordCount == 0 {
		t.Fatal("view 应包含 content_rev 与 word_count")
	}
	if p.Chapters[0].Content == "" {
		t.Fatal("原始 state 被破坏")
	}
}
