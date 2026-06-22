package main

import "testing"

func TestCalcSynopsisLengthRange(t *testing.T) {
	tests := []struct {
		chapters, perChapter, wantMin, wantMax int
	}{
		{5, 1500, 300, 600},    // 7.5k total -> floor
		{30, 2500, 937, 2142},  // 75k total
		{100, 3000, 3750, 5000}, // 300k total -> cap
	}
	for _, tt := range tests {
		minLen, maxLen := calcSynopsisLengthRange(tt.chapters, tt.perChapter)
		if minLen != tt.wantMin || maxLen != tt.wantMax {
			t.Errorf("calcSynopsisLengthRange(%d,%d) = (%d,%d), want (%d,%d)",
				tt.chapters, tt.perChapter, minLen, maxLen, tt.wantMin, tt.wantMax)
		}
	}
}
