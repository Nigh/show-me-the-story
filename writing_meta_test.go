package main

import "testing"

func TestStripChapterMetaProse(t *testing.T) {
	in := "第1章 初入江湖\n\n她推开门。\n\n本章完"
	got := stripChapterMetaProse(in, LangZH)
	want := "她推开门。"
	if got != want {
		t.Fatalf("stripChapterMetaProse() = %q, want %q", got, want)
	}

	enIn := "Chapter 1: Arrival\n\nShe opened the door.\n\nEnd of chapter"
	enGot := stripChapterMetaProse(enIn, LangEN)
	enWant := "She opened the door."
	if enGot != enWant {
		t.Fatalf("stripChapterMetaProse(en) = %q, want %q", enGot, enWant)
	}
}
