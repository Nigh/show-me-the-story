package main

import (
	"strings"
	"testing"
)

func TestCalcChapterLengthRange(t *testing.T) {
	tests := []struct {
		target  int
		wantMin int
		wantMax int
	}{
		{5000, 4000, 6000},
		{2500, 1500, 3500},
		{10000, 8500, 11500},
		{0, 1500, 3500}, // defaults to 2500
	}
	for _, tt := range tests {
		minLen, maxLen := calcChapterLengthRange(tt.target)
		if minLen != tt.wantMin || maxLen != tt.wantMax {
			t.Errorf("calcChapterLengthRange(%d) = (%d,%d), want (%d,%d)",
				tt.target, minLen, maxLen, tt.wantMin, tt.wantMax)
		}
	}
}

func TestChapterLengthInRange(t *testing.T) {
	minLen, maxLen := calcChapterLengthRange(5000)
	if !chapterLengthInRange(strings.Repeat("字", 5000), minLen, maxLen) {
		t.Fatal("5000 prose units should be in range for 5000 target")
	}
	if chapterLengthInRange(strings.Repeat("字", 15000), minLen, maxLen) {
		t.Fatal("15000 prose units should be out of range for 5000 target")
	}
}
