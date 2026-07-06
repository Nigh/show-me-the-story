package story

import (
	"showmethestory/internal/prose"
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

func TestProseLengthScore(t *testing.T) {
	minLen, maxLen := calcChapterLengthRange(5000)
	target := 5000
	inRange := proseLengthScore(5000, minLen, maxLen, target)
	slightlyOver := proseLengthScore(6200, minLen, maxLen, target)
	farOver := proseLengthScore(15000, minLen, maxLen, target)
	if inRange >= slightlyOver || slightlyOver >= farOver {
		t.Fatalf("scores want inRange < slightlyOver < farOver, got %d %d %d", inRange, slightlyOver, farOver)
	}
}

func TestIsSoftLengthDeviation(t *testing.T) {
	_, maxLen := calcChapterLengthRange(5000)
	if !isSoftLengthDeviation(maxLen+200, 4000, maxLen) {
		t.Fatal("200 over max should be soft for 5000 target")
	}
	if isSoftLengthDeviation(maxLen+500, 4000, maxLen) {
		t.Fatal("500 over max should not be soft for 5000 target")
	}
}

func TestShouldAttemptLengthAdjust(t *testing.T) {
	minLen, maxLen := calcChapterLengthRange(5000)
	if shouldAttemptLengthAdjust(maxLen+200, minLen, maxLen) {
		t.Fatal("soft overflow should not attempt adjust")
	}
	if !shouldAttemptLengthAdjust(maxLen+400, minLen, maxLen) {
		t.Fatal("moderate overflow should attempt adjust")
	}
	if shouldAttemptLengthAdjust(maxLen*120/100+1, minLen, maxLen) {
		t.Fatal(">120% max should not attempt adjust")
	}
}

func TestMaybeUpdateBestDraft(t *testing.T) {
	minLen, maxLen := calcChapterLengthRange(5000)
	target := 5000
	var best string
	score := int(^uint(0) >> 1)
	maybeUpdateBestDraft(&best, &score, strings.Repeat("字", 15000), minLen, maxLen, target)
	maybeUpdateBestDraft(&best, &score, strings.Repeat("字", 6200), minLen, maxLen, target)
	if prose.CountProseUnits(best) != 6200 {
		t.Fatalf("best draft = %d units, want 6200", prose.CountProseUnits(best))
	}
}
