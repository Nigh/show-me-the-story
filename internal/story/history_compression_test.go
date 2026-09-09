package story

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"showmethestory/internal/config"
)

func acceptedHistory(count int) *Progress {
	state := &Progress{CurrentChapterIndex: count}
	for n := 1; n <= count; n++ {
		text := fmt.Sprintf("chapter %d Alice advances the silver bridge promise", n)
		state.Chapters = append(state.Chapters, ChapterState{Num: n, Title: fmt.Sprintf("C%d", n), Outline: strings.Repeat(text, 12), Content: text, Summary: text, Status: StatusAccepted})
	}
	return state
}

func TestNarrativeCheckpointBoundariesAndHierarchy(t *testing.T) {
	for _, tc := range []struct{ chapters, leaves, parents int }{{40, 0, 0}, {41, 1, 0}, {60, 2, 0}, {220, 10, 1}, {2020, 100, 11}} {
		state := acceptedHistory(tc.chapters)
		if err := EnsureNarrativeCheckpoints(context.Background(), nil, config.DefaultConfigForLang("en"), state, "", nil); err != nil {
			t.Fatal(err)
		}
		leaves, parents := 0, 0
		for _, cp := range state.NarrativeCheckpoints {
			if cp.Level == 1 {
				leaves++
			} else {
				parents++
			}
			if cp.SourceHash == "" || cp.Summary == "" {
				t.Fatal("incomplete checkpoint", cp)
			}
		}
		if leaves != tc.leaves || parents != tc.parents {
			t.Fatalf("%d chapters: got %d leaves/%d parents", tc.chapters, leaves, parents)
		}
	}
}

func TestNarrativeCheckpointInvalidationAndPromptBounds(t *testing.T) {
	state := acceptedHistory(600)
	cfg := config.DefaultConfigForLang("en")
	if err := EnsureNarrativeCheckpoints(context.Background(), nil, cfg, state, "", nil); err != nil {
		t.Fatal(err)
	}
	oldHash := state.NarrativeCheckpoints[0].SourceHash
	state.Chapters[0].Summary = "changed durable fact"
	if err := EnsureNarrativeCheckpoints(context.Background(), nil, cfg, state, "", nil); err != nil {
		t.Fatal(err)
	}
	if state.NarrativeCheckpoints[0].SourceHash == oldHash {
		t.Fatal("stale checkpoint was reused")
	}
	if got := utf8.RuneCountInString(buildHistorySummaryForLang(state, 599, "en")); got > historyPromptRunes {
		t.Fatalf("history uses %d runes", got)
	}
	if got := utf8.RuneCountInString(buildOutlineConstraintsForLang(state, 599, "en")); got > 12000 {
		t.Fatalf("outline constraints use %d runes", got)
	}
	if got := utf8.RuneCountInString(BuildPlanningHistory(state, "Alice silver bridge", "en")); got > historyPromptRunes {
		t.Fatalf("planning history uses %d runes", got)
	}
}
