package story

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"showmethestory/internal/config"
	"showmethestory/internal/sse"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestKnowledgeRetrievalLargeBook(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		t.Run(lang, func(t *testing.T) {
			name, fact, query := "Alice", "Alice cannot cross the silver bridge without the jade key.", "Alice returns to the silver bridge with the jade key."
			if lang == "zh" {
				name, fact, query = "林青", "林青没有玉钥匙就无法通过银桥。", "林青带着玉钥匙重返银桥。"
			}
			state := &Progress{MemoryMaxTokens: 2000}
			settings := &ProjectSettings{Characters: []Character{{ID: "c_1", Name: name, Notes: fact}, {ID: "c_2", Name: "Keeper", Notes: "Knows the bridge password."}}, Relations: []Relation{{ID: "r_1", SourceID: "c_1", TargetID: "c_2", Label: "trust"}}}
			state.Chapters = append(state.Chapters, factChapter(1, fact))
			state.MemoryEntries = append(state.MemoryEntries, MemoryEntry{ID: 1, Content: fact, Category: "character", References: []MemoryReference{{Chapter: 1, BlockID: 1, Quote: fact}}})
			for n := 2; n <= 600; n++ {
				text := fmt.Sprintf("Unrelated merchant %d sells vegetables in a distant village.", n)
				state.Chapters = append(state.Chapters, factChapter(n, text))
				state.MemoryEntries = append(state.MemoryEntries, MemoryEntry{ID: n, Content: text, Category: "event", References: []MemoryReference{{Chapter: n, BlockID: 1, Quote: text}}})
				settings.Characters = append(settings.Characters, Character{ID: fmt.Sprintf("c_%d", n+1), Name: fmt.Sprintf("Merchant%d", n), Background: strings.Repeat(text, 10)})
			}
			ch := ChapterState{Num: 600, Outline: query}
			state.Chapters[599] = ch
			before, _ := json.Marshal(settings)
			selected, visible := retrieveSettings(settings, query, settingsContextRunes)
			data, _ := json.Marshal(settingsEntities(selected))
			if !visible["c_1"] || !visible["c_2"] || !visible["r_1"] {
				t.Fatal("lost direct entity or neighbor", visible)
			}
			if visible["c_500"] || utf8.RuneCount(data) > settingsContextRunes {
				t.Fatal("unrelated settings or budget overflow")
			}
			memories := buildMemoryForLang(state, 599, lang)
			if !strings.Contains(memories, fact) {
				t.Fatal("early relevant fact lost behind recent history")
			}
			if utf8.RuneCountInString(memories)*3/2 > state.MemoryMaxTokens {
				t.Fatal("memory budget exceeded")
			}
			after, _ := json.Marshal(settings)
			if string(before) != string(after) || len(state.MemoryEntries) != 600 {
				t.Fatal("retrieval changed persistent data")
			}
		})
	}
}

func TestKnowledgeRetrievalBoundaries(t *testing.T) {
	if knowledgeNameMatch("Joanne returns", "Ann") || !knowledgeNameMatch("ANN returns", "Ann") {
		t.Fatal("English name boundaries")
	}
	if !knowledgeNameMatch("林青回来了", "林青") {
		t.Fatal("Chinese name matching")
	}
	settings := &ProjectSettings{Characters: []Character{{ID: "c_1", Name: "Alice", Notes: strings.Repeat("x", settingsContextRunes+1)}}}
	selected, ids := retrieveSettings(settings, "Alice", settingsContextRunes)
	if ids["c_1"] || len(selected.Characters) != 1 || !strings.Contains(selected.Characters[0].Notes, "EXCERPT") {
		t.Fatal("oversized entity lost its identity or excerpt was treated as complete")
	}
	selected, ids = retrieveSettings(settings, "quartz", settingsContextRunes)
	if len(ids) != 0 {
		t.Fatal("no-match fell back to full registry")
	}
	state := &Progress{Chapters: []ChapterState{factChapter(1, "old evidence"), factChapter(2, "future evidence")}, MemoryEntries: []MemoryEntry{
		{ID: 1, Content: "Alice future-only evidence", References: []MemoryReference{{Chapter: 2, BlockID: 1, Quote: "future evidence"}}},
		{ID: 2, Content: "Alice stale evidence", References: []MemoryReference{{Chapter: 1, BlockID: 1, Quote: "wrong"}}},
		{ID: 3, Content: "Alice future fact", References: []MemoryReference{{Chapter: 2, BlockID: 1, Quote: "future evidence"}}},
	}}
	if got := retrieveMemories(state, ChapterState{Num: 1, Outline: "Alice"}, 1000, true); len(got) != 0 {
		t.Fatal("future/stale fact leaked", got)
	}
	if got := retrieveMemories(state, ChapterState{Num: 2, Outline: "Alice"}, 0, true); len(got) != 0 {
		t.Fatal("zero budget exceeded")
	}
}

func TestRetrievedSettingsCollisionAndPendingDedup(t *testing.T) {
	settings := &ProjectSettings{Characters: []Character{{ID: "c_1", Name: "Alice", Age: "20"}}}
	ch := factChapter(1, "Alice turns 21.")
	delta := []settingDelta{{Kind: "characters", Entity: map[string]any{"name": "Alice", "age": "21"}, Evolution: true, BlockID: 1}}
	for i := 0; i < 2; i++ {
		if err := applySettingDeltas(settings, ch, delta, map[string]bool{}); err != nil {
			t.Fatal(err)
		}
	}
	if settings.Characters[0].Age != "20" || len(settings.Characters) != 1 || len(settings.StoryChanges) != 1 || settings.StoryChanges[0].Status != "pending" {
		t.Fatal("unseen identity overwritten or duplicate pending change")
	}
}

func TestLongSettingExcerptPreservesOriginal(t *testing.T) {
	notes := strings.Repeat("Alice sells vegetables in the market. ", 300) + "Alice cannot cross the silver bridge without the jade key."
	settings := &ProjectSettings{Characters: []Character{{ID: "c_1", Name: "Alice", Notes: notes}}}
	selected, visible := retrieveSettings(settings, "Alice crosses the silver bridge with the jade key.", settingsContextRunes)
	if len(selected.Characters) != 1 || !strings.Contains(selected.Characters[0].Notes, "cannot cross the silver bridge without the jade key.") || visible["c_1"] {
		t.Fatal("lost relevant complete sentence or failed to mark partial entity")
	}
	ch := factChapter(1, "Alice now has a golden key.")
	settings.StoryChanges = []SettingChange{{ID: 1, Kind: "characters", EntityID: "c_1", Status: "applied", After: entityAt(settings, "characters", "c_1")}}
	if err := applySettingDeltas(settings, ch, []settingDelta{{Kind: "characters", Entity: map[string]any{"id": "c_1", "notes": "Alice has a golden key."}, Evolution: true, BlockID: 1}}, visible); err != nil {
		t.Fatal(err)
	}
	if settings.Characters[0].Notes != notes || settings.StoryChanges[1].Status != "pending" {
		t.Fatal("partial context silently overwrote complete original")
	}
}

func TestRetrievedKnowledgeSyncUsesOriginals(t *testing.T) {
	ch := factChapter(1, "Alice arrives.")
	ch.MemoryRevision = ChapterRevision(ch)
	state := &Progress{Chapters: []ChapterState{ch}}
	settings := &ProjectSettings{Characters: []Character{{ID: "c_1", Name: "Alice", Notes: "ORIGINAL_INDEPENDENT_CONSTRAINT"}, {ID: "c_2", Name: "Zorro", Notes: strings.Repeat("UNRELATED_SENTINEL", 2000)}}}
	before := cloneSettings(settings)
	api := batchAPI(t, func(prompt string) string {
		if !strings.Contains(prompt, "ORIGINAL_INDEPENDENT_CONSTRAINT") || strings.Contains(prompt, "UNRELATED_SENTINEL") {
			t.Error("sync did not retrieve original relevant records")
		}
		return `{"changes":[]}`
	})
	path := filepath.Join(t.TempDir(), "progress.json")
	if err := SyncPendingKnowledge(context.Background(), api, config.DefaultConfigForLang("en"), state, settings, path, sse.NewLogBroadcaster()); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(settings.Characters, before.Characters) || settings.StorySynced[1] != ChapterRevision(ch) {
		t.Fatal("retrieval changed original records or failed sync")
	}
}

func BenchmarkKnowledgeRetrieval600Chapters(b *testing.B) {
	settings := &ProjectSettings{}
	for n := 0; n < 600; n++ {
		settings.Characters = append(settings.Characters, Character{ID: fmt.Sprint(n), Name: fmt.Sprintf("Person%d", n), Background: strings.Repeat("A merchant in a distant village. ", 20)})
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		retrieveSettings(settings, "Person599 returns to the village.", settingsContextRunes)
	}
}
