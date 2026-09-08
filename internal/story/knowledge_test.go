package story

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"showmethestory/internal/config"
	"showmethestory/internal/sse"
	"strings"
	"testing"
)

func factChapter(num int, text string) ChapterState {
	ch := ChapterState{Num: num, Content: text, Status: StatusAccepted, KnowledgeTracked: true}
	SyncChapterBlocks(&ch)
	return ch
}

func TestMemoryIdentityRetry(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		for _, invalid := range []string{
			`{"new_memories":[{"id":99,"content":"Alice has a scar","category":"character","block_ids":[1]}]}`,
			`{"new_memories":[{"id":7,"content":"Alice has no scar","category":"character","block_ids":[1]}]}`,
			`{"new_memories":[{"id":0,"content":"New fact","category":"event","block_ids":[1]},{"id":99,"content":"Bad ID","category":"event","block_ids":[1]}]}`,
			`{"new_memories":`,
		} {
			t.Run(lang+invalid, func(t *testing.T) {
				for _, recover := range []bool{true, false} {
					state := &Progress{Chapters: []ChapterState{factChapter(1, "Alice has a scar.")}, MemoryEntries: []MemoryEntry{{ID: 7, Content: "Alice has a scar", Category: "character", Chapter: 1}}, NextMemoryID: 8}
					before, _ := json.Marshal(state.MemoryEntries)
					calls := 0
					api := batchAPI(t, func(prompt string) string {
						calls++
						if calls == 2 {
							feedback := "The previous response failed validation:"
							if lang == "zh" {
								feedback = "上次输出校验失败："
							}
							if !strings.Contains(prompt, feedback) {
								t.Error("retry lacks correction feedback")
							}
							live, _ := json.Marshal(state.MemoryEntries)
							if string(live) != string(before) || state.NextMemoryID != 8 {
								t.Error("failed attempt changed live facts or consumed IDs")
							}
							if recover {
								return `{"new_memories":[{"id":7,"content":"Alice has a scar","category":"character","block_ids":[1]}]}`
							}
						}
						return invalid
					})
					path := filepath.Join(t.TempDir(), "progress.json")
					err := SyncChapterMemory(context.Background(), api, config.DefaultConfigForLang(lang), state, 0, path, sse.NewLogBroadcaster())
					if calls != 2 || (err == nil) != recover {
						t.Fatalf("calls=%d recover=%v err=%v", calls, recover, err)
					}
					loaded, err := LoadProgress(path)
					if err != nil {
						t.Fatal(err)
					}
					if recover {
						if len(loaded.MemoryEntries) != 1 || loaded.MemoryEntries[0].ID != 7 || len(loaded.MemoryEntries[0].References) != 1 || loaded.Chapters[0].MemoryRevision == "" {
							t.Fatal("corrected identity or evidence was not persisted")
						}
					} else {
						after, _ := json.Marshal(loaded.MemoryEntries)
						if string(after) != string(before) || loaded.NextMemoryID != 8 || loaded.Chapters[0].MemoryRevision != "" || !loaded.Chapters[0].KnowledgeTracked {
							t.Fatal("failed retries changed stored facts or lost pending marker")
						}
					}
				}
			})
		}
	}
}
func TestEndingControls(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		state := &Progress{OutlineBatches: []OutlineBatch{{ID: 1, StartCh: 1, EndCh: 2, EndingIntent: "final", PlannedFinal: true}}}
		req := OutlineBatchRequest{ChapterCount: 1, Synopsis: "plot", EndingIntent: "sequel", EndingStyle: "open"}
		if ValidateOutlineBatch(state, req, lang) == nil {
			t.Fatal("continuing past ending needs confirmation")
		}
		req.ConfirmContinue = true
		if err := ValidateOutlineBatch(state, req, lang); err != nil {
			t.Fatal(err)
		}
		req.EndingStyle = "custom"
		if ValidateOutlineBatch(state, req, lang) == nil {
			t.Fatal("custom requires instructions")
		}
		req.EndingRequirements = "END_REQUIREMENT"
		req.EndingStyle = "open"
		cfg := config.DefaultConfigForLang(lang)
		cfg.Prompts.ContinuationOutlineGeneration = "CUSTOM"
		api := batchAPI(t, func(prompt string) string {
			if strings.Contains(prompt, "CUSTOM") {
				if !strings.Contains(prompt, "END_REQUIREMENT") {
					t.Error("missing ending instruction")
				}
				return batchAnswer(1, 1)
			}
			return "{}"
		})
		// Empty book with a final batch exercises persistence without an old-format migration.
		state = &Progress{}
		path := filepath.Join(t.TempDir(), "progress.json")
		if err := GenerateOutlineBatch(context.Background(), api, cfg, state, nil, req, path, sse.NewLogBroadcaster()); err != nil {
			t.Fatal(err)
		}
		if !state.OutlineBatches[0].PlannedFinal || state.BookStatus == BookStatusCompleted {
			t.Fatal("planned ending must not complete book")
		}
		loaded, err := LoadProgress(path)
		if err != nil {
			t.Fatal(err)
		}
		if loaded.OutlineBatches[0].EndingStyle != "open" {
			t.Fatal("ending not persisted")
		}
		for _, s := range []string{chapterEnding(state, 1, lang), buildOutlineConstraintsForLang(state, 0, lang), factProtection(state, 1, lang)} {
			if !strings.Contains(s, "END_REQUIREMENT") {
				t.Fatal("ending missing in writing/revision context")
			}
		}
	}
}
func TestFactLinksStableAndFailedExtractionSafe(t *testing.T) {
	state := &Progress{Chapters: []ChapterState{factChapter(1, "Alice has a scar.\n\nThe scar is visible."), factChapter(2, "Alice hides the scar.")}}
	cfg := config.DefaultConfigForLang("en")
	answer := `{"new_memories":[{"id":0,"content":"Alice has a scar","category":"character","block_ids":[1,2]}]}`
	api := batchAPI(t, func(string) string { return answer })
	path := filepath.Join(t.TempDir(), "progress.json")
	logger := sse.NewLogBroadcaster()
	if err := SyncChapterMemory(context.Background(), api, cfg, state, 0, path, logger); err != nil {
		t.Fatal(err)
	}
	id := state.MemoryEntries[0].ID
	answer = `{"new_memories":[{"id":1,"content":"Alice has a scar","category":"character","block_ids":[1]}]}`
	if err := SyncChapterMemory(context.Background(), api, cfg, state, 1, path, logger); err != nil {
		t.Fatal(err)
	}
	if len(state.MemoryEntries) != 1 || state.MemoryEntries[0].ID != id || len(state.MemoryEntries[0].References) != 3 {
		t.Fatal("cross chapter identity lost")
	}
	before := state.Chapters[0]
	after := before
	after.Blocks = append([]Block(nil), before.Blocks...)
	if err := UpdateBlock(&after, 1, "Alice has no scar."); err != nil {
		t.Fatal(err)
	}
	if ValidateFactEdit(state, before, after, FactEditOptions{}, "en") == nil {
		t.Fatal("unconfirmed edit allowed")
	}
	opts := FactEditOptions{ContentRev: ChapterRevision(before), ConfirmFactImpact: true}
	if err := ValidateFactEdit(state, before, after, opts, "en"); err != nil {
		t.Fatal(err)
	}
	opts.ContentRev = "outdated"
	if ValidateFactEdit(state, before, after, opts, "en") == nil {
		t.Fatal("stale confirmation allowed")
	}
	state.Chapters[0] = after
	prior, _ := json.Marshal(state.MemoryEntries)
	answer = "invalid JSON"
	if SyncChapterMemory(context.Background(), api, cfg, state, 0, path, logger) == nil {
		t.Fatal("expected extraction failure")
	}
	current, _ := json.Marshal(state.MemoryEntries)
	if string(prior) != string(current) {
		t.Fatal("failed extraction changed facts")
	}
	refs := FactsForChapter(state, 1, 0)[0].References
	if !refs[0].Stale || refs[1].Stale {
		t.Fatal("wrong changed passage detection")
	}
	answer = `{"new_memories":[]}`
	if err := SyncChapterMemory(context.Background(), api, cfg, state, 0, path, logger); err != nil {
		t.Fatal(err)
	}
	if state.MemoryEntries[0].ID != id || !memoryHasLiveReference(state, state.MemoryEntries[0]) {
		t.Fatal("other chapter evidence lost")
	}
	loaded, err := LoadProgress(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.MemoryEntries[0].References) != 3 {
		t.Fatal("references not persisted")
	}
	if ProgressView(state).MemoryEntries[0].References != nil {
		t.Fatal("progress leaks full evidence")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if SyncChapterMemory(ctx, api, cfg, state, 0, path, logger) == nil {
		t.Fatal("cancellation ignored")
	}
}
func TestSettingsSyncEvidenceConflictsAndHistory(t *testing.T) {
	ch := factChapter(1, "Alice and Bob join the Guild.\n\nThey become allies.")
	s := &ProjectSettings{}
	deltas := []settingDelta{
		{Kind: "characters", Entity: map[string]any{"id": "$alice", "name": "Alice", "age": "20"}, BlockID: 1},
		{Kind: "characters", Entity: map[string]any{"id": "$bob", "name": "Bob"}, BlockID: 1},
		{Kind: "organizations", Entity: map[string]any{"id": "$guild", "name": "Guild", "members": []any{"$alice", "$bob"}}, BlockID: 1},
		{Kind: "relations", Entity: map[string]any{"source_id": "$alice", "source_type": "character", "target_id": "$bob", "target_type": "character", "label": "allies"}, BlockID: 2},
	}
	if err := applySettingDeltas(s, ch, deltas); err != nil {
		t.Fatal(err)
	}
	if len(s.Characters) != 2 || len(s.Relations) != 1 || s.Relations[0].SourceID != s.Characters[0].ID || len(s.Organizations[0].Members) != 2 {
		t.Fatal("new entity references not resolved")
	}
	ch2 := factChapter(2, "They become enemies.")
	relationID := s.Relations[0].ID
	if err := applySettingDeltas(s, ch2, []settingDelta{{Kind: "relations", Entity: map[string]any{"id": relationID, "label": "enemies"}, Evolution: true, BlockID: 1}}); err != nil {
		t.Fatal(err)
	}
	if s.Relations[0].Label != "enemies" || settingsAtChapter(s, 1).Relations[0].Label != "allies" {
		t.Fatal("future relation leaked into past")
	}
	// An unrelated author edit must not prevent reversal of an automatic field.
	s.Characters[0].Notes = "author note"
	if err := applySettingDeltas(s, ch2, []settingDelta{{Kind: "characters", Entity: map[string]any{"id": s.Characters[1].ID, "age": "22"}, BlockID: 1}}); err != nil {
		t.Fatal(err)
	}
	s.Characters[1].Notes = "another author note"
	past := settingsAtChapter(s, 1)
	if past.Characters[1].Age != "" || past.Characters[1].Notes != "another author note" {
		t.Fatal("field-level history lost author edit or leaked future age")
	}
	count := len(s.StoryChanges)
	if err := applySettingDeltas(s, ch2, []settingDelta{{Kind: "relations", Entity: map[string]any{"id": relationID, "label": "enemies"}, Evolution: true, BlockID: 1}}); err != nil {
		t.Fatal(err)
	}
	if len(s.StoryChanges) != count {
		t.Fatal("duplicate update")
	}
	// Author changes are never silently replaced, even when model labels them evolution.
	s.Characters[0].Age = "author age"
	if err := applySettingDeltas(s, ch2, []settingDelta{{Kind: "characters", Entity: map[string]any{"id": s.Characters[0].ID, "age": "30"}, Evolution: true, BlockID: 1}}); err != nil {
		t.Fatal(err)
	}
	if s.Characters[0].Age != "author age" || s.StoryChanges[len(s.StoryChanges)-1].Status != "pending" {
		t.Fatal("overwrote author setting")
	}
	state := &Progress{Chapters: []ChapterState{ch, ch2}}
	state.Chapters[1].Content = "Different chapter."
	SyncChapterBlocks(&state.Chapters[1])
	invalidateSettingSources(s, state)
	if s.Relations[0].Label != "allies" {
		t.Fatal("changed source did not withdraw relation evolution")
	}
	if s.Characters[0].Age != "author age" {
		t.Fatal("withdrawal overwrote author value")
	}
	before := cloneSettings(s)
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := SaveProjectSettings(path, s); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadProjectSettings(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, loaded) {
		t.Fatal("setting provenance not persisted")
	}
}

func TestSettingsSyncIgnoresUnknownModelFields(t *testing.T) {
	ch := factChapter(1, "Alice arrives.")
	s := &ProjectSettings{}
	err := applySettingDeltas(s, ch, []settingDelta{{
		Kind: "characters", Entity: map[string]any{"name": "Alice", "gender": "female"}, BlockID: 1,
	}})
	if err != nil || len(s.Characters) != 1 || s.Characters[0].Name != "Alice" {
		t.Fatalf("safe setting fields were not applied: %v", err)
	}
}
func TestKnowledgeSyncFailureAndNoLegacyBackfill(t *testing.T) {
	calls := 0
	cfg := config.DefaultConfigForLang("en")
	api := batchAPI(t, func(prompt string) string {
		calls++
		if strings.Contains(prompt, "new_memories") {
			return `{"new_memories":[]}`
		}
		return `{"changes":[{"kind":"characters","entity":{"name":"Alice"},"block_id":1}]}`
	})
	state := &Progress{Chapters: []ChapterState{factChapter(1, "Alice arrives."), {Num: 2, Content: "Old untouched chapter", Status: StatusAccepted}}}
	settings := &ProjectSettings{}
	path := filepath.Join(t.TempDir(), "progress.json")
	logger := sse.NewLogBroadcaster()
	if err := SyncPendingKnowledge(context.Background(), api, cfg, state, settings, path, logger); err != nil {
		t.Fatal(err)
	}
	if calls != 2 || len(settings.Characters) != 1 || settings.StorySynced[2] != "" {
		t.Fatal("unexpected backfill or sync")
	}
	if err := SyncPendingKnowledge(context.Background(), api, cfg, state, settings, path, logger); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatal("idempotent sync called model again")
	}
	// Settings write failure must leave in-memory settings unchanged.
	next := factChapter(3, "Alice again.")
	state.Chapters = append(state.Chapters, next)
	settingsFile := filepath.Join(filepath.Dir(path), "settings.json")
	if err := os.Remove(settingsFile); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(settingsFile, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(settingsFile, "keep"), []byte("occupied"), 0644); err != nil {
		t.Fatal(err)
	}
	prior := cloneSettings(settings)
	if SyncPendingKnowledge(context.Background(), api, cfg, state, settings, path, logger) == nil {
		t.Fatal("expected save failure")
	}
	if !reflect.DeepEqual(prior, settings) {
		t.Fatal("failed save committed setting state")
	}
}

func TestSourceWithdrawalProposalSurvivesRetry(t *testing.T) {
	ch := factChapter(1, "Alice arrives.")
	s := &ProjectSettings{}
	if err := applySettingDeltas(s, ch, []settingDelta{{Kind: "characters", Entity: map[string]any{"name": "Alice", "age": "20"}, BlockID: 1}}); err != nil {
		t.Fatal(err)
	}
	s.Characters[0].Age = "author override"
	state := &Progress{Chapters: []ChapterState{factChapter(1, "Different text.")}}
	invalidateSettingSources(s, state)
	n := len(s.StoryChanges)
	if n != 2 || s.StoryChanges[1].Status != "pending" {
		t.Fatal("withdrawal proposal missing")
	}
	invalidateSettingSources(s, state)
	if len(s.StoryChanges) != n || s.StoryChanges[1].Status != "pending" {
		t.Fatal("retry discarded pending withdrawal")
	}
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := ResolveSettingChange(s, state, s.StoryChanges[1].ID, false, path); err != nil {
		t.Fatal(err)
	}
	if s.Characters[0].Age != "author override" {
		t.Fatal("dismiss changed author settings")
	}
}

func TestFactInvalidEvidenceAndConfirmSaveFailure(t *testing.T) {
	ch := factChapter(1, "A fact.")
	ch.Status = StatusReview
	state := &Progress{Phase: "writing", Chapters: []ChapterState{ch}}
	path := filepath.Join(t.TempDir(), "missing", "progress.json")
	if err := os.MkdirAll(path, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "keep"), []byte("occupied"), 0644); err != nil {
		t.Fatal(err)
	}
	if ConfirmChapterAction(state, path) == nil {
		t.Fatal("expected save failure")
	}
	if state.Chapters[0].Status != StatusReview || state.CurrentChapterIndex != 0 {
		t.Fatal("failed confirmation mutated state")
	}
	api := batchAPI(t, func(string) string {
		return `{"new_memories":[{"id":0,"content":"bad","category":"event","block_ids":[999]}]}`
	})
	goodPath := filepath.Join(t.TempDir(), "progress.json")
	if SyncChapterMemory(context.Background(), api, config.DefaultConfig(), state, 0, goodPath, sse.NewLogBroadcaster()) == nil {
		t.Fatal("invalid evidence accepted")
	}
	if len(state.MemoryEntries) != 0 || state.NextMemoryID != 0 {
		t.Fatal("invalid evidence committed facts")
	}
}

// Memory synchronization commits a copied chapter slice; completion must update that live slice.
func TestGenerateChapterReviewAfterMemorySync(t *testing.T) {
	for _, memoryReply := range []string{`{"new_memories":[]}`, `invalid JSON`} {
		t.Run(memoryReply, func(t *testing.T) {
			cfg := config.DefaultConfigForLang("en")
			cfg.Story.TargetWordsPerChapter = 1000
			cfg.Prompts.ChapterWriting = "TEST_PROSE"
			cfg.Prompts.ChapterSummary = "TEST_SUMMARY"
			cfg.Prompts.FactCheck = "TEST_FACT_CHECK"
			cfg.Prompts.MemoryUpdate = "TEST_MEMORY"
			content := strings.Repeat("Alice walked home. ", 350)
			memoryCalls := 0
			api := batchAPI(t, func(prompt string) string {
				switch {
				case strings.HasPrefix(prompt, "TEST_PROSE"):
					return content
				case strings.HasPrefix(prompt, "TEST_SUMMARY"):
					return "Alice returned home."
				case strings.HasPrefix(prompt, "TEST_FACT_CHECK"):
					return `{"result":"PASS"}`
				case strings.HasPrefix(prompt, "TEST_MEMORY"):
					memoryCalls++
					return memoryReply
				default:
					t.Errorf("unexpected model request: %.80s", prompt)
					return `{}`
				}
			})
			state := &Progress{Phase: "writing", Chapters: []ChapterState{
				{Num: 1, Title: "Home", Status: StatusPending},
				{Num: 2, Title: "Tomorrow", Status: StatusPending},
			}}
			path := filepath.Join(t.TempDir(), "progress.json")
			if err := GenerateChapterAction(context.Background(), api, cfg, state, path, nil, nil, sse.NewLogBroadcaster()); err != nil {
				t.Fatal(err)
			}
			wantCalls := 1
			if memoryReply == "invalid JSON" {
				wantCalls = 2
			}
			if memoryCalls != wantCalls {
				t.Fatalf("memory calls = %d, want %d", memoryCalls, wantCalls)
			}
			loaded, err := LoadProgress(path)
			if err != nil {
				t.Fatal(err)
			}
			for _, p := range []*Progress{state, loaded} {
				if p.Chapters[0].Status != StatusReview {
					t.Fatalf("completed chapter status = %s, want review", p.Chapters[0].Status)
				}
				if !p.Chapters[0].KnowledgeTracked || p.Chapters[0].Content != strings.TrimSpace(content) || p.Chapters[0].Summary == "" {
					t.Fatal("completion lost prose, summary or knowledge marker")
				}
				if (p.Chapters[0].MemoryRevision != "") != (memoryReply != "invalid JSON") {
					t.Fatal("incorrect memory synchronization revision")
				}
				if p.PendingWritingConflict != nil {
					t.Fatal("unexpected writing conflict")
				}
			}
			if err := ConfirmChapterAction(state, path); err != nil {
				t.Fatalf("automatic confirmation failed: %v", err)
			}
			loaded, err = LoadProgress(path)
			if err != nil {
				t.Fatal(err)
			}
			if loaded.Chapters[0].Status != StatusAccepted || loaded.CurrentChapterIndex != 1 || loaded.Chapters[1].Status != StatusPending {
				t.Fatal("confirmation did not advance to the next planned chapter")
			}
		})
	}
}
