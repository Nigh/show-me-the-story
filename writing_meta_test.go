package main

import "testing"

func TestStripChapterMetaProse(t *testing.T) {
	tests := []struct {
		name string
		in   string
		lang string
		want string
	}{
		{
			name: "zh basic",
			in:   "第1章 初入江湖\n\n她推开门。\n\n本章完",
			lang: LangZH,
			want: "她推开门。",
		},
		{
			name: "en basic",
			in:   "Chapter 1: Arrival\n\nShe opened the door.\n\nEnd of chapter",
			lang: LangEN,
			want: "She opened the door.",
		},
		{
			name: "zh preamble revised",
			in:   "以下为修订后的第3章完整正文：\n\n她走进了房间。\n\n本章完",
			lang: LangZH,
			want: "她走进了房间。",
		},
		{
			name: "zh preamble simple",
			in:   "第2章 风起云涌\n\n风吹过山岗。",
			lang: LangZH,
			want: "风吹过山岗。",
		},
		{
			name: "zh paren chapter",
			in:   "（第5章正文）\n\n夜色深沉。\n\n（完）",
			lang: LangZH,
			want: "夜色深沉。",
		},
		{
			name: "en preamble here is",
			in:   "Here is the revised Chapter 3:\n\nThe door opened.\n\nEnd of chapter",
			lang: LangEN,
			want: "The door opened.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripChapterMetaProse(tt.in, tt.lang)
			if got != tt.want {
				t.Fatalf("stripChapterMetaProse() = %q, want %q", got, tt.want)
			}
		})
	}
}
