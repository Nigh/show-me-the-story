package story

import "testing"

func TestCalcOutlineLengthRange(t *testing.T) {
	tests := []struct {
		perChapter int
		wantMin    int
		wantMax    int
	}{
		{2500, 125, 312},
		{1000, 80, 150},
		{0, 125, 312},
	}
	for _, tt := range tests {
		minLen, maxLen := calcOutlineLengthRange(tt.perChapter)
		if minLen != tt.wantMin || maxLen != tt.wantMax {
			t.Errorf("calcOutlineLengthRange(%d) = (%d,%d), want (%d,%d)",
				tt.perChapter, minLen, maxLen, tt.wantMin, tt.wantMax)
		}
	}
}

func TestExtractFirstAppearanceStubs(t *testing.T) {
	outline := "张三与李四在码头会面。王五（首次登场）是码头管事，告知密信下落。"
	stubs := extractFirstAppearanceStubs(outline)
	if len(stubs) != 1 || stubs[0].Name != "王五" {
		t.Fatalf("extractFirstAppearanceStubs() = %+v, want 王五", stubs)
	}
}

func TestCharacterStubsPreferStructuredOverProse(t *testing.T) {
	// Prose regex would greedily capture a long clause before「首次登场」; structured cast wins.
	ch := ChapterState{
		Outline: "想起班主任吕红梅（首次登场）匆匆走进教室。",
		Characters: []OutlineChapterCharacter{
			{Name: "吕红梅", FirstAppearance: true, Note: "班主任"},
			{Name: "亚历山大·伊万诺夫"},
		},
	}
	stubs := characterStubsForChapter(ch)
	if len(stubs) != 2 {
		t.Fatalf("got %d stubs, want 2: %+v", len(stubs), stubs)
	}
	if stubs[0].Name != "吕红梅" || stubs[0].Description != "班主任" {
		t.Fatalf("first stub = %+v", stubs[0])
	}
	if stubs[1].Name != "亚历山大·伊万诺夫" {
		t.Fatalf("second stub = %+v", stubs[1])
	}
}

func TestNormalizeOutlineCharacters(t *testing.T) {
	got := normalizeOutlineCharacters([]OutlineChapterCharacter{
		{Name: "  吕红梅  ", FirstAppearance: true, Note: " 班主任 "},
		{Name: "吕红梅", Note: "duplicate ignored"},
		{Name: "「王五」"},
		{Name: "   "},
	})
	if len(got) != 2 || got[0].Name != "吕红梅" || !got[0].FirstAppearance || got[0].Note != "班主任" {
		t.Fatalf("got %+v", got)
	}
	if got[1].Name != "王五" {
		t.Fatalf("marks not stripped: %+v", got[1])
	}
}

func TestValidateOutlineChapterLengths(t *testing.T) {
	chapters := []OutlineChapter{
		{Num: 1, Outline: stringsRepeat("情节", 50)},
		{Num: 2, Outline: "太短"},
	}
	short := validateOutlineChapterLengths(chapters, 80)
	if len(short) != 1 || short[0] != 2 {
		t.Fatalf("validateOutlineChapterLengths() = %v, want [2]", short)
	}
}

func stringsRepeat(s string, n int) string {
	out := ""
	for i := 0; i < n; i++ {
		out += s
	}
	return out
}
