package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"showmethestory/internal/story"
	"strconv"
	"strings"
)

func (h *Handlers) knowledgeVersion() string {
	// ponytail: linear scan of tracked prose; cache revisions if task startup becomes costly on very large books.
	var out strings.Builder
	if h.state != nil {
		for _, ch := range h.state.Chapters {
			if ch.KnowledgeTracked {
				fmt.Fprintf(&out, "%d:%s:%s;", ch.Num, ch.Status, story.ChapterRevision(ch))
			}
		}
	}
	return out.String()
}

func (h *Handlers) startKnowledgeSync() bool {
	if !h.tryStartTask() {
		return false
	}
	h.forceKnowledgeSync = true
	go h.endTask()
	return true
}
func (h *Handlers) PostKnowledgeSync(w http.ResponseWriter, r *http.Request) {
	if !h.startKnowledgeSync() {
		h.writeErrorReq(w, r, http.StatusConflict, "task_running_wait")
		return
	}
	h.writeJSON(w, http.StatusAccepted, map[string]string{"status": "started"})
}
func (h *Handlers) GetKnowledge(w http.ResponseWriter, r *http.Request) {
	num, _ := strconv.Atoi(r.URL.Query().Get("chapter"))
	id, _ := strconv.Atoi(r.URL.Query().Get("fact"))
	facts := []story.MemoryEntry{}
	if num > 0 {
		facts = story.FactsForChapter(h.state, num, 0)
	} else if id > 0 {
		for _, m := range h.state.MemoryEntries {
			if m.ID == id {
				facts = append(facts, m)
			}
		}
		for i := range facts {
			refs := append([]story.MemoryReference(nil), facts[i].References...)
			for j := range refs {
				refs[j].Stale = !story.ReferenceLive(h.state, refs[j])
			}
			facts[i].References = refs
		}
	}
	pending := []int{}
	seen := map[int]bool{}
	for _, ch := range h.state.Chapters {
		if ch.KnowledgeTracked && ch.Content != "" && (ch.Status == story.StatusReview || ch.Status == story.StatusAccepted) && (ch.MemoryRevision != story.ChapterRevision(ch) || (ch.Status == story.StatusAccepted && h.settings.StorySynced[ch.Num] != story.ChapterRevision(ch))) {
			pending = append(pending, ch.Num)
			seen[ch.Num] = true
		}
	}
	changes := []story.SettingChange{}
	for _, c := range h.settings.StoryChanges {
		if c.Status == "pending" {
			changes = append(changes, c)
		}
		if c.Status == "applied" && !story.ReferenceLive(h.state, c.Source) && !seen[c.Source.Chapter] {
			pending = append(pending, c.Source.Chapter)
			seen[c.Source.Chapter] = true
		}
	}
	if h.state.BookStatus == story.BookStatusCompleted && h.postprocess != nil && h.postprocess.ContentModified {
		pending = nil
		changes = nil
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"facts": facts, "pending_chapters": pending, "changes": changes})
}
func (h *Handlers) PostSettingChange(w http.ResponseWriter, r *http.Request) {
	if h.rejectIfTaskRunning(w, r) {
		return
	}
	var body struct {
		ID     int   `json:"id"`
		Accept *bool `json:"accept"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.writeErrorReq(w, r, http.StatusBadRequest, "invalid_json", err)
		return
	}
	if body.Accept == nil || body.ID <= 0 {
		h.writeErrorReq(w, r, http.StatusBadRequest, "invalid_json", "id and accept required")
		return
	}
	if err := story.ResolveSettingChange(h.settings, h.state, body.ID, *body.Accept, h.settingsPath); err != nil {
		if err.Error() == "content_version_conflict" || err.Error() == "setting_has_dependents" {
			h.writeErrorReq(w, r, http.StatusConflict, err.Error())
			return
		}
		h.writeErrorReq(w, r, http.StatusConflict, "invalid_json", err)
		return
	}
	h.logger.SettingsUpdated()
	h.GetSettings(w, r)
}
