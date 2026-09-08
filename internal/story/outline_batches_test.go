package story

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"showmethestory/internal/config"
	"showmethestory/internal/sse"
	"strings"
	"testing"
	"time"
)

func batchAPI(t *testing.T, answer func(string) string) *config.APIConfig {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			http.Error(w, "bad request", 400)
			return
		}
		prompt := request.Messages[len(request.Messages)-1].Content
		content := answer(prompt)
		payload, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"delta": map[string]string{"content": content}, "finish_reason": "stop"}}})
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", payload)
	}))
	t.Cleanup(server.Close)
	return &config.APIConfig{BaseURL: server.URL, APIKey: "test", Model: "test"}
}

func batchAnswer(start, count int) string {
	chapters := make([]OutlineChapter, count)
	for i := range chapters {
		chapters[i] = OutlineChapter{Num: start + i, Title: fmt.Sprint(start + i), Outline: strings.Repeat("具体情节", 100)}
	}
	b, _ := json.Marshal(map[string]any{"chapters": chapters})
	return string(b)
}

func TestOutlineBatchAppendReplaceAndRoundTrip(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		t.Run(lang, func(t *testing.T) {
			cfg := config.DefaultConfigForLang(lang)
			cfg.Story.StorySynopsis = "obsolete whole book"
			// Simulate a saved custom template with none of the new placeholders.
			cfg.Prompts.ContinuationOutlineGeneration = "CUSTOM {{.Title}} {{.ExistingOutline}}"
			state := &Progress{CorePrompt: "permanent writing rules"}
			path := filepath.Join(t.TempDir(), "progress.json")
			start, count, synopsis := 1, 12, "FIRST_BATCH_MARKER"
			api := batchAPI(t, func(prompt string) string {
				if !strings.Contains(prompt, "CUSTOM") {
					return `{}`
				} // postprocessing checks
				if !strings.Contains(prompt, synopsis) || !strings.Contains(prompt, "DIRECTION_MARKER") {
					t.Errorf("missing batch input: %s", prompt)
				}
				if strings.Contains(prompt, "obsolete whole book") {
					t.Error("legacy synopsis leaked into new batch")
				}
				return batchAnswer(start, count)
			})
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			run := func(mode string, id int) {
				t.Helper()
				err := GenerateOutlineBatch(ctx, api, cfg, state, nil, OutlineBatchRequest{ChapterCount: count, Synopsis: synopsis, LongTermDirection: "DIRECTION_MARKER", Mode: mode, BatchID: id}, path, sse.NewLogBroadcaster())
				if err != nil {
					t.Fatal(err)
				}
			}
			run("append", 0)
			start, count, synopsis = 13, 24, "SECOND_BATCH_MARKER"
			run("append", 0)
			if len(state.Chapters) != 36 || len(state.OutlineBatches) != 2 || state.OutlineBatches[1].StartCh != 13 || state.OutlineBatches[1].EndCh != 36 {
				t.Fatalf("bad batches: %+v", state.OutlineBatches)
			}
			if got := ChapterSynopsis(cfg, state, 1); !strings.Contains(got, "FIRST_BATCH_MARKER") || strings.Contains(got, "SECOND_BATCH_MARKER") {
				t.Fatalf("wrong chapter synopsis: %s", got)
			}
			first := state.Chapters[0]
			count, synopsis = 5, "REPLACED_BATCH_MARKER"
			run("replace_last", 2)
			if len(state.Chapters) != 17 || state.Chapters[0].Outline != first.Outline || state.OutlineBatches[1].Revision != 2 {
				t.Fatal("replacement altered prior batch or failed to update revision")
			}
			if state.CorePrompt != "permanent writing rules" || cfg.Story.StorySynopsis != "obsolete whole book" {
				t.Fatal("batch overwrote permanent settings")
			}
			loaded, err := LoadProgress(path)
			if err != nil {
				t.Fatal(err)
			}
			if len(loaded.OutlineBatches) != 2 || loaded.OutlineBatches[1].Synopsis != synopsis {
				t.Fatal("batch metadata not persisted")
			}
		})
	}
}

func TestOutlineBatchValidation(t *testing.T) {
	state := &Progress{Chapters: []ChapterState{{Num: 1, Status: StatusPending}}, OutlineBatches: []OutlineBatch{{ID: 1, StartCh: 1, EndCh: 1, Synopsis: "old"}}}
	valid := OutlineBatchRequest{ChapterCount: 12, Synopsis: "new", Mode: "replace_last", BatchID: 1}
	if err := ValidateOutlineBatch(state, valid, "en"); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*OutlineBatchRequest){
		func(r *OutlineBatchRequest) { r.Synopsis = "  " }, func(r *OutlineBatchRequest) { r.ChapterCount = 37 }, func(r *OutlineBatchRequest) { r.ChapterCount = 0 }, func(r *OutlineBatchRequest) { r.Mode = "unknown" }, func(r *OutlineBatchRequest) { r.BatchID = 2 },
	} {
		r := valid
		mutate(&r)
		if ValidateOutlineBatch(state, r, "en") == nil {
			t.Fatalf("accepted invalid request %+v", r)
		}
	}
	for _, status := range []string{StatusAccepted, StatusWriting, StatusReview} {
		state.Chapters[0].Status = status
		if ValidateOutlineBatch(state, valid, "en") == nil {
			t.Fatal("allowed replacing written batch")
		}
	}
}

func TestOutlineBatchFailurePreservesState(t *testing.T) {
	for _, failure := range []string{"wrong_count", "wrong_number", "cancel", "save"} {
		t.Run(failure, func(t *testing.T) {
			cfg := config.DefaultConfigForLang("en")
			cfg.Prompts.ContinuationOutlineGeneration = "CUSTOM"
			state := &Progress{Phase: "writing", CorePrompt: "KEEP", Chapters: []ChapterState{{Num: 1, Status: StatusPending, Outline: "KEEP"}}, OutlineBatches: []OutlineBatch{{ID: 1, Revision: 1, StartCh: 1, EndCh: 1, Synopsis: "KEEP"}}}
			path := filepath.Join(t.TempDir(), "progress.json")
			if err := SaveProgress(path, state); err != nil {
				t.Fatal(err)
			}
			before, _ := json.Marshal(state)
			diskBefore, _ := os.ReadFile(path)
			calls := 0
			api := batchAPI(t, func(string) string {
				calls++
				if failure == "wrong_count" {
					return batchAnswer(1, 1)
				}
				if failure == "wrong_number" {
					return batchAnswer(20, 2)
				}
				return batchAnswer(1, 2)
			})
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if failure == "cancel" {
				cancel()
			}
			target := path
			if failure == "save" {
				target = filepath.Join(path, "blocked.json")
			}
			err := GenerateOutlineBatch(ctx, api, cfg, state, nil, OutlineBatchRequest{ChapterCount: 2, Synopsis: "new", Mode: "replace_last", BatchID: 1}, target, sse.NewLogBroadcaster())
			if err == nil {
				t.Fatal("expected failure")
			}
			after, _ := json.Marshal(state)
			diskAfter, _ := os.ReadFile(path)
			if string(before) != string(after) || string(diskBefore) != string(diskAfter) {
				t.Fatal("failure changed existing state")
			}
			if (failure == "wrong_count" || failure == "wrong_number") && calls != 3 {
				t.Fatalf("expected bounded retries, got %d", calls)
			}
		})
	}
}

func TestLegacyChapterSynopsisFallback(t *testing.T) {
	cfg := config.DefaultConfigForLang("zh")
	cfg.Story.StorySynopsis = "legacy"
	state := &Progress{OutlineBatches: []OutlineBatch{{ID: 1, StartCh: 10, EndCh: 12, Synopsis: "batch"}}}
	if ChapterSynopsis(cfg, state, 1) != "legacy" || !strings.Contains(ChapterSynopsis(cfg, state, 10), "batch") {
		t.Fatal("legacy fallback or batch selection broken")
	}
}

func TestBatchDefaultTemplatesUseCurrentInputs(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		t.Run(lang, func(t *testing.T) {
			cfg := config.DefaultConfigForLang(lang)
			cfg.Story.Title = "SAVED_TITLE"
			cfg.Story.StorySynopsis = "STALE_SYNOPSIS"
			state := &Progress{Title: "STALE_TITLE", StorySynopsis: "STALE_SYNOPSIS", CorePrompt: "CORE_RULES"}
			captured := false
			api := batchAPI(t, func(prompt string) string {
				if !captured {
					captured = true
					for _, value := range []string{"SAVED_TITLE", "BATCH_PLOT", "LONG_DIRECTION", "CORE_RULES"} {
						if !strings.Contains(prompt, value) {
							t.Errorf("missing %s", value)
						}
					}
					if strings.Contains(prompt, "STALE_") {
						t.Error("stale progress data injected")
					}
					return batchAnswer(1, 1)
				}
				return `{}`
			})
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := GenerateOutlineBatch(ctx, api, cfg, state, nil, OutlineBatchRequest{ChapterCount: 1, Synopsis: "BATCH_PLOT", LongTermDirection: "LONG_DIRECTION"}, filepath.Join(t.TempDir(), "progress.json"), sse.NewLogBroadcaster()); err != nil {
				t.Fatal(err)
			}
			if !captured {
				t.Fatal("no generation request")
			}
		})
	}
}
