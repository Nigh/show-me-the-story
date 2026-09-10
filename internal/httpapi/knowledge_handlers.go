package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"showmethestory/internal/i18n"
	"showmethestory/internal/story"
	"strconv"
	"strings"
)

type chapterRevisionRequest struct {
	Feedback     string   `json:"feedback"`
	WorldviewIDs []string `json:"worldview_ids"`
}

// Explicit selections are included in full, independently of relevance retrieval.
func (h *Handlers) prepareSettingRevision(w http.ResponseWriter, r *http.Request, num int, body *chapterRevisionRequest) bool {
	if len(body.WorldviewIDs) == 0 {
		return true
	}
	idx := story.FindChapterIdx(h.state, num)
	if idx < 0 {
		h.writeErrorReq(w, r, http.StatusNotFound, "chapter_n_not_found", num)
		return false
	}
	before := h.state.Chapters[idx]
	if r.Header.Get("X-Content-Rev") != story.ChapterRevision(before) {
		h.writeErrorReq(w, r, http.StatusConflict, "content_version_conflict")
		return false
	}
	after := before
	after.Blocks = nil
	opts := story.FactEditOptions{ContentRev: r.Header.Get("X-Content-Rev"), ConfirmFactImpact: r.Header.Get("X-Confirm-Fact-Impact") == "true"}
	if err := story.ValidateFactEdit(h.state, before, after, opts, i18n.FromRequest(r)); err != nil {
		h.writeErrorReq(w, r, http.StatusConflict, "invalid_json", err)
		return false
	}
	selected := []story.WorldviewEntry{}
	seen := map[string]bool{}
	for _, id := range body.WorldviewIDs {
		if seen[id] {
			continue
		}
		found := false
		for _, entry := range h.settings.Worldview {
			if entry.ID == id {
				selected = append(selected, entry)
				found = true
				break
			}
		}
		if !found {
			h.writeErrorReq(w, r, http.StatusNotFound, "worldview_not_found")
			return false
		}
		seen[id] = true
	}
	data, _ := json.Marshal(selected)
	instruction := "\n[Author-selected story settings]\nThese are authoritative rules of this novel, whether fictional or realistic. Correct this chapter and related descriptions to follow them, including conflicting extracted facts. Preserve unrelated plot and style. Do not replace these rules with real-world assumptions.\n"
	if i18n.NormalizeLanguage(h.cfg.Language) == i18n.LangZH {
		instruction = "\n【作者指定的小说设定】\n以下是本小说的明确规则，可以是虚构或现实知识。请据此修正本章及连带描述，包括与之冲突的已提取事实；保留无关剧情和文风，不得用现实常识替换这些规则。\n"
	}
	body.Feedback += instruction + string(data)
	return true
}

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
