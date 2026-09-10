package story

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"showmethestory/internal/config"
)

func TestNarrativeCheckpointRetriesDegradedHierarchy(t *testing.T) {
	state := acceptedHistory(220)
	cfg := config.DefaultConfigForLang("en")
	path := filepath.Join(t.TempDir(), "progress.json")
	calls := 0
	answer := ""
	api := batchAPI(t, func(string) string { calls++; return answer })
	if err := EnsureNarrativeCheckpoints(context.Background(), api, cfg, state, path, nil); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadProgress(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, cp := range loaded.NarrativeCheckpoints {
		if !cp.Degraded || !strings.Contains(cp.Summary, "Alice") || strings.Contains(cp.Summary, "hash=") {
			t.Fatalf("fallback lost provenance or narrative: %+v", cp)
		}
	}
	failedCalls := calls
	answer = "Alice keeps her promise."
	if err := EnsureNarrativeCheckpoints(context.Background(), api, cfg, loaded, path, nil); err != nil {
		t.Fatal(err)
	}
	if calls != failedCalls+11 {
		t.Fatalf("expected retry of 10 leaves and parent, calls=%d", calls)
	}
	for _, cp := range loaded.NarrativeCheckpoints {
		if cp.Degraded || cp.Summary != answer {
			t.Fatalf("checkpoint not recovered: %+v", cp)
		}
	}
	if err := EnsureNarrativeCheckpoints(context.Background(), api, cfg, loaded, path, nil); err != nil {
		t.Fatal(err)
	}
	if calls != failedCalls+11 {
		t.Fatal("healthy checkpoints should be reused")
	}
}

func TestNarrativeCheckpointCancellationPreservesState(t *testing.T) {
	for _, cancelBefore := range []bool{true, false} {
		t.Run(fmt.Sprint(cancelBefore), func(t *testing.T) {
			state := acceptedHistory(60)
			state.NarrativeCheckpoints = []NarrativeCheckpoint{{StartChapter: 1, EndChapter: 20, Summary: "old"}}
			before := append([]NarrativeCheckpoint(nil), state.NarrativeCheckpoints...)
			path := filepath.Join(t.TempDir(), "progress.json")
			if err := SaveProgress(path, state); err != nil {
				t.Fatal(err)
			}
			bytesBefore, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			calls := 0
			api := batchAPI(t, func(string) string {
				calls++
				if calls == 2 {
					cancel()
				}
				return "new"
			})
			if cancelBefore {
				cancel()
			}
			if err := EnsureNarrativeCheckpoints(ctx, api, config.DefaultConfigForLang("en"), state, path, nil); !errors.Is(err, context.Canceled) {
				t.Fatalf("got %v", err)
			}
			bytesAfter, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(bytesAfter) != string(bytesBefore) || !reflect.DeepEqual(before, state.NarrativeCheckpoints) {
				t.Fatal("cancelled checkpoints were committed")
			}
			if (!cancelBefore && calls != 2) || (cancelBefore && calls != 0) {
				t.Fatalf("continued after cancellation: %d calls", calls)
			}
		})
	}
}

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
