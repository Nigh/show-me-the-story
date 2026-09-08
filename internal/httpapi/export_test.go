package httpapi

import (
	"net/http/httptest"
	"showmethestory/internal/config"
	"showmethestory/internal/story"
	"strings"
	"testing"
)

func TestBookExportUsesNovelTitleAndMarkdownHeadings(t *testing.T) {
	h := &Handlers{
		cfg: &config.Config{Language: "zh", Story: config.StoryConfig{Title: "小说标题"}},
		state: &story.Progress{
			Title:    "进度标题",
			Chapters: []story.ChapterState{{Num: 3, Title: "新的开始", Content: "正文"}},
		},
	}
	res := httptest.NewRecorder()
	h.GetBookExport(res, httptest.NewRequest("GET", "/api/export/txt", nil))
	want := "# 小说标题\n\n## 第 3 章 新的开始\n\n正文"
	if got := res.Body.String(); got != want {
		t.Fatalf("export mismatch:\n got %q\nwant %q", got, want)
	}
}

func TestOutlineExportIncludesEveryChapter(t *testing.T) {
	h := &Handlers{projectName: "test", cfg: config.DefaultConfig(), state: &story.Progress{Title: "Book", Chapters: []story.ChapterState{{Num: 1, Title: "One", Outline: "First"}, {Num: 2, Title: "Two", Outline: "Second"}}}}
	res := httptest.NewRecorder()
	h.GetOutlineExport(res, httptest.NewRequest("GET", "/api/export/outline", nil))
	got := res.Body.String()
	if !strings.Contains(got, "## 第 1 章 One\n\nFirst") || !strings.Contains(got, "## 第 2 章 Two\n\nSecond") {
		t.Fatal(got)
	}
}
