package story

import (
	"context"
	"showmethestory/internal/config"
	"showmethestory/internal/fsutil"
	"showmethestory/internal/sse"
	"testing"
)

func TestForeshadowReportSaveFailurePreservesReportAndDiagnostics(t *testing.T) {
	before := &ForeshadowOutlineReport{Summary: "original"}
	state := &Progress{LastForeshadowOutlineReport: before, Foreshadows: []Foreshadow{{ID: 1, Name: "clue"}}}
	api := batchAPI(t, func(string) string { return `{"has_conflicts":true,"summary":"new"}` })
	logger := sse.NewLogBroadcaster()
	events := logger.Subscribe()
	defer logger.Unsubscribe(events)
	err := RunForeshadowOutlineCheckAndSave(context.Background(), api, config.DefaultConfig(), state, t.TempDir(), logger)
	if _, ok := fsutil.AsSaveError(err); !ok || state.LastForeshadowOutlineReport != before {
		t.Fatal("failed save replaced report or lost error", err)
	}
	found := false
	for len(events) > 0 {
		event := <-events
		if event.Event == "storage_error" {
			found = true
		}
		if event.Event == "foreshadow_outline_conflicts" {
			t.Fatal("failed save published new conflicts")
		}
	}
	if !found {
		t.Fatal("save failure did not emit storage diagnostics")
	}
}
