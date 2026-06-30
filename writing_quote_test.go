package main

import (
	"strings"
	"testing"
)

func TestExtractQuotedSentences(t *testing.T) {
	tests := []struct {
		name           string
		feedback       string
		wantQuotes     []string
		wantCleanNonEmpty bool
	}{
		{
			name:        "no quote lines returns nil and original feedback",
			feedback:    "把主角的剑改成长枪",
			wantQuotes:  nil,
			wantCleanNonEmpty: true,
		},
		{
			name:        "single quote with feedback",
			feedback:    "> 他抬头望向天空。\n把天空改成星空",
			wantQuotes:  []string{"他抬头望向天空。"},
			wantCleanNonEmpty: true,
		},
		{
			name:        "quote only no feedback",
			feedback:    "> 他抬头望向天空。",
			wantQuotes:  []string{"他抬头望向天空。"},
			wantCleanNonEmpty: false,
		},
		{
			name:        "multiple distinct quotes preserve order",
			feedback:    "> 第一句。\n意见A\n> 第二句。\n意见B",
			wantQuotes:  []string{"第一句。", "第二句。"},
			wantCleanNonEmpty: true,
		},
		{
			name:        "duplicate quotes are deduped",
			feedback:    "> 重复句。\n> 重复句。\n意见",
			wantQuotes:  []string{"重复句。"},
			wantCleanNonEmpty: true,
		},
		{
			name:        "leading spaces before gt are tolerated",
			feedback:    "  > 带缩进的引用。\n意见",
			wantQuotes:  []string{"带缩进的引用。"},
			wantCleanNonEmpty: true,
		},
		{
			name:        "empty string returns nil",
			feedback:    "",
			wantQuotes:  nil,
			wantCleanNonEmpty: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			quotes, clean := extractQuotedSentences(tt.feedback)
			if len(quotes) != len(tt.wantQuotes) {
				t.Fatalf("quotes len = %d, want %d (got %v)", len(quotes), len(tt.wantQuotes), quotes)
			}
			for i, q := range quotes {
				if q != tt.wantQuotes[i] {
					t.Fatalf("quotes[%d] = %q, want %q", i, q, tt.wantQuotes[i])
				}
			}
			if tt.wantCleanNonEmpty && clean == "" {
				t.Fatalf("clean feedback unexpectedly empty")
			}
			if !tt.wantCleanNonEmpty && clean != "" && tt.wantQuotes != nil {
				t.Fatalf("clean feedback = %q, want empty", clean)
			}
		})
	}
}

func TestExtractQuotedSentencesCleanFeedbackContent(t *testing.T) {
	quotes, clean := extractQuotedSentences("> 他笑了。\n把笑声描写得更冷")
	if len(quotes) != 1 || quotes[0] != "他笑了。" {
		t.Fatalf("quotes = %v, want [他笑了。]", quotes)
	}
	if clean != "把笑声描写得更冷" {
		t.Fatalf("clean = %q, want %q", clean, "把笑声描写得更冷")
	}
	// 引用行不应残留在 cleanFeedback 中
	for _, q := range quotes {
		if strings.Contains(clean, q) {
			t.Fatalf("clean feedback %q should not contain quote %q", clean, q)
		}
	}
}

func TestFindParagraphsContaining(t *testing.T) {
	contentDoubleNL := "第一段：他笑了。\n\n第二段：她走了。\n\n第三段：天黑了。"
	contentSingleNL := "第一行：他笑了。\n第二行：她走了。\n第三行：天黑了。"

	tests := []struct {
		name          string
		content       string
		quotes        []string
		wantOk        bool
		wantMatchedIdx []int
	}{
		{
			name:           "double newline single quote matches one paragraph",
			content:        contentDoubleNL,
			quotes:         []string{"她走了。"},
			wantOk:         true,
			wantMatchedIdx: []int{1},
		},
		{
			name:           "double newline multiple quotes hit different paragraphs",
			content:        contentDoubleNL,
			quotes:         []string{"她走了。", "天黑了。"},
			wantOk:         true,
			wantMatchedIdx: []int{1, 2},
		},
		{
			name:           "multiple quotes in same paragraph dedup",
			content:        contentDoubleNL,
			quotes:         []string{"他笑了。", "第一段"},
			wantOk:         true,
			wantMatchedIdx: []int{0},
		},
		{
			name:           "single newline fallback split",
			content:        contentSingleNL,
			quotes:         []string{"她走了。"},
			wantOk:         true,
			wantMatchedIdx: []int{1},
		},
		{
			name:           "quote not found returns ok false",
			content:        contentDoubleNL,
			quotes:         []string{"不存在的句子"},
			wantOk:         false,
			wantMatchedIdx: nil,
		},
		{
			name:           "one of multiple quotes missing returns ok false",
			content:        contentDoubleNL,
			quotes:         []string{"她走了。", "不存在的句子"},
			wantOk:         false,
			wantMatchedIdx: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matchedIdx, _, ok := findParagraphsContaining(tt.content, tt.quotes)
			if ok != tt.wantOk {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOk)
			}
			if !ok {
				return
			}
			if len(matchedIdx) != len(tt.wantMatchedIdx) {
				t.Fatalf("matchedIdx len = %d, want %d (got %v)", len(matchedIdx), len(tt.wantMatchedIdx), matchedIdx)
			}
			for i, idx := range matchedIdx {
				if idx != tt.wantMatchedIdx[i] {
					t.Fatalf("matchedIdx[%d] = %d, want %d", i, idx, tt.wantMatchedIdx[i])
				}
			}
			// 验证索引升序
			for i := 1; i < len(matchedIdx); i++ {
				if matchedIdx[i] <= matchedIdx[i-1] {
					t.Fatalf("matchedIdx not strictly ascending: %v", matchedIdx)
				}
			}
		})
	}
}

func TestTrimEmptyEnds(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{"no empty ends", []string{"a", "b"}, []string{"a", "b"}},
		{"leading empty", []string{"  ", "a", "b"}, []string{"a", "b"}},
		{"trailing empty", []string{"a", "b", "\n"}, []string{"a", "b"}},
		{"both ends empty", []string{"", "a", "", "b", ""}, []string{"a", "", "b"}},
		{"all empty", []string{"", " "}, []string{}},
		{"empty slice", []string{}, []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := trimEmptyEnds(tt.in)
			if len(got) != len(tt.want) {
				t.Fatalf("len = %d, want %d (got %v)", len(got), len(tt.want), got)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("got[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}
