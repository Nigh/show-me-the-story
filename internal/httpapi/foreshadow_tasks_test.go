package httpapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"showmethestory/internal/config"
	"showmethestory/internal/sse"
	"showmethestory/internal/story"
	"strings"
	"testing"
	"time"
)

func TestForeshadowChecksReserveTaskAndCanBeCancelled(t *testing.T) {
	for _, action := range []string{"edit outline", "confirm foreshadows", "explicit check"} {
		for _, outcome := range []string{"success", "cancel", "model failure"} {
			t.Run(action+"/"+outcome, func(t *testing.T) {
				started, release := make(chan struct{}, 1), make(chan struct{})
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					io.Copy(io.Discard, r.Body)
					started <- struct{}{}
					select {
					case <-release:
					case <-r.Context().Done():
						return
					}
					if outcome == "model failure" {
						http.Error(w, "unauthorized", 401)
						return
					}
					payload, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"delta": map[string]string{"content": `{"has_conflicts":false,"summary":"checked"}`}, "finish_reason": "stop"}}})
					w.Header().Set("Content-Type", "text/event-stream")
					fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", payload)
				}))
				defer server.Close()
				defer func() {
					select {
					case <-release:
					default:
						close(release)
					}
				}()
				h := NewHandlers(&config.APIConfig{BaseURL: server.URL, APIKey: "test", Model: "test"}, "", sse.NewLogBroadcaster(), t.TempDir(), "test")
				h.progressPath = filepath.Join(h.progDir, "progress.json")
				h.state.Chapters = []story.ChapterState{{Num: 1, Title: "old", Outline: "plot", Status: story.StatusPending}}
				h.state.Foreshadows = []story.Foreshadow{{ID: 1, Name: "clue", Status: story.ForeshadowPlanted}}
				events := h.logger.Subscribe()
				defer h.logger.Unsubscribe(events)
				res := httptest.NewRecorder()
				switch action {
				case "edit outline":
					r := httptest.NewRequest("PUT", "/api/outline/1", strings.NewReader(`{"title":"new","outline":"new plot"}`))
					r.SetPathValue("num", "1")
					h.PutChapterOutline(res, r)
				case "confirm foreshadows":
					h.PostForeshadowsConfirm(res, httptest.NewRequest("POST", "/api/foreshadows/confirm", strings.NewReader(`{"foreshadows":[{"name":"new clue"}]}`)))
				case "explicit check":
					h.PostForeshadowOutlineCheck(res, httptest.NewRequest("POST", "/api/foreshadows/outline-check", nil))
				}
				if res.Code != http.StatusOK && res.Code != http.StatusAccepted {
					t.Fatal(res.Code, res.Body.String())
				}
				select {
				case <-started:
				case <-time.After(5 * time.Second):
					t.Fatal("check never started")
				}
				if !h.isTaskRunning() || h.tryStartTask() {
					t.Fatal("background check bypassed task reservation")
				}
				switchRes := httptest.NewRecorder()
				h.PostProjectSelect(switchRes, httptest.NewRequest("POST", "/api/projects/select", strings.NewReader(`{"name":"another"}`)))
				if switchRes.Code != http.StatusConflict {
					t.Fatal("project switch allowed during check")
				}
				if outcome == "cancel" {
					stop := httptest.NewRecorder()
					h.PostTaskStop(stop, httptest.NewRequest("POST", "/api/task/stop", nil))
					if stop.Code != http.StatusOK {
						t.Fatal("cancel rejected", stop.Body.String())
					}
				}
				close(release)
				deadline := time.Now().Add(5 * time.Second)
				for h.isTaskRunning() && time.Now().Before(deadline) {
					time.Sleep(time.Millisecond)
				}
				if h.isTaskRunning() {
					t.Fatal("check leaked task reservation")
				}
				if (h.state.LastForeshadowOutlineReport != nil) != (outcome == "success") {
					t.Fatal("failed/cancelled check published a report")
				}
				foundEnd := false
				for len(events) > 0 {
					event := <-events
					if event.Event == "task_end" {
						data := event.Data.(map[string]interface{})
						if data["task"] == "foreshadow_outline_check" {
							foundEnd = true
							if data["success"] != (outcome == "success") {
								t.Fatal("incorrect task success", data)
							}
						}
					}
				}
				if !foundEnd {
					t.Fatal("missing task end event")
				}
			})
		}
	}
}

func TestAutomaticForeshadowCheckEarlyReturnsReleaseTask(t *testing.T) {
	for _, action := range []string{"edit outline", "confirm foreshadows"} {
		for _, failure := range []string{"bad request", "save failure", "no foreshadows"} {
			t.Run(action+"/"+failure, func(t *testing.T) {
				h := NewHandlers(&config.APIConfig{}, "", sse.NewLogBroadcaster(), t.TempDir(), "test")
				h.progressPath = filepath.Join(h.progDir, "progress.json")
				h.state.Chapters = []story.ChapterState{{Num: 1, Title: "old", Status: story.StatusPending}}
				if failure == "save failure" {
					h.progressPath = t.TempDir()
				}
				body := `{"title":"new","foreshadows":[]}`
				if failure == "bad request" {
					body = `{`
				}
				r := httptest.NewRequest("POST", "/", strings.NewReader(body))
				r.SetPathValue("num", "1")
				res := httptest.NewRecorder()
				if action == "edit outline" {
					h.PutChapterOutline(res, r)
				} else {
					h.PostForeshadowsConfirm(res, r)
				}
				want := http.StatusOK
				if failure == "bad request" {
					want = http.StatusBadRequest
				}
				if failure == "save failure" {
					want = http.StatusInternalServerError
				}
				if res.Code != want || h.isTaskRunning() {
					t.Fatal("wrong response or leaked reservation", res.Code, res.Body.String())
				}
				if failure != "no foreshadows" && h.state.Chapters[0].Title != "old" {
					t.Fatal("failed request changed outline")
				}
			})
		}
	}
}
