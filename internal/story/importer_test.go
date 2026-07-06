package story

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestSplitImportContentHeadings(t *testing.T) {
	content := "作品简介：一个平凡少年的故事。\n\n第一章 觉醒\n\n林凡睁开眼睛。\n\n第二章 出发\n\n他收拾行囊。\n\nChapter 3 The Road\n\nHe walked on."
	chs, preamble := SplitImportContent(content)
	if len(chs) != 3 {
		t.Fatalf("expected 3 chapters, got %d", len(chs))
	}
	if !strings.Contains(preamble, "作品简介") {
		t.Fatalf("preamble missing: %q", preamble)
	}
	if chs[0].Title != "第一章 觉醒" {
		t.Fatalf("title wrong: %q", chs[0].Title)
	}
	if !strings.Contains(chs[1].Content, "行囊") {
		t.Fatalf("content assigned to wrong chapter: %q", chs[1].Content)
	}
	if chs[2].Title != "Chapter 3 The Road" {
		t.Fatalf("english title wrong: %q", chs[2].Title)
	}
}

func TestSplitImportContentNoHeadings(t *testing.T) {
	// Heading-less text falls back to size-based chunks.
	para := strings.Repeat("这是一个没有章节标题的段落。", 30) // ~390 runes
	content := strings.Repeat(para+"\n\n", 40)   // well over one chunk
	chs, _ := SplitImportContent(content)
	if len(chs) < 2 {
		t.Fatalf("expected multiple chunks, got %d", len(chs))
	}
	if chs[0].Title != "第1章" {
		t.Fatalf("placeholder title wrong: %q", chs[0].Title)
	}
	if splitEmpty, _ := SplitImportContent("   "); splitEmpty != nil {
		t.Fatal("empty content should split to nil")
	}
}

func TestBuildImportPreview(t *testing.T) {
	chs := []importedChapter{{Title: "第一章 觉醒", Content: "第一章 觉醒\n林凡睁开眼睛，" + strings.Repeat("很长的内容。", 30)}}
	pv := BuildImportPreview(chs)
	if pv[0].Num != 1 || pv[0].WordCount == 0 {
		t.Fatalf("preview meta wrong: %+v", pv[0])
	}
	if !strings.HasSuffix(pv[0].Preview, "…") {
		t.Fatalf("long preview should be truncated: %q", pv[0].Preview)
	}
	if strings.Contains(pv[0].Preview, "第一章 觉醒") {
		t.Fatalf("preview should skip the heading line: %q", pv[0].Preview)
	}
}

func TestImportStateRoundtrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "import.json")
	if LoadImportState(path) != nil {
		t.Fatal("missing file should load as nil")
	}
	st := &ImportState{Active: true, Total: 10, Cursor: 3, MetaDone: true}
	if err := SaveImportState(path, st); err != nil {
		t.Fatal(err)
	}
	got := LoadImportState(path)
	if got == nil || got.Cursor != 3 || got.Total != 10 || !got.MetaDone {
		t.Fatalf("roundtrip wrong: %+v", got)
	}
	// Inactive state loads as nil (finished import).
	st.Active = false
	SaveImportState(path, st)
	if LoadImportState(path) != nil {
		t.Fatal("inactive state should load as nil")
	}
}

func TestCreateImportArcs(t *testing.T) {
	state := &Progress{Chapters: make([]ChapterState, 65)}
	for i := range state.Chapters {
		state.Chapters[i].Num = i + 1
	}
	createImportArcs(state)
	// 65 chapters: 1-30, 31-65 (trailing 5 merged into second arc, no runt).
	if len(state.Arcs) != 2 {
		t.Fatalf("expected 2 arcs, got %d: %+v", len(state.Arcs), state.Arcs)
	}
	if state.Arcs[0].StartCh != 1 || state.Arcs[0].EndCh != 30 {
		t.Fatalf("arc 1 range wrong: %+v", state.Arcs[0])
	}
	if state.Arcs[1].StartCh != 31 || state.Arcs[1].EndCh != 65 {
		t.Fatalf("arc 2 range wrong: %+v", state.Arcs[1])
	}
}
