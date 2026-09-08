package httpapi

import (
	"net/http/httptest"
	"showmethestory/internal/config"
	"showmethestory/internal/story"
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
