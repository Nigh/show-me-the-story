package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"showmethestory/internal/config"
	"showmethestory/internal/i18n"
	"showmethestory/internal/story"
)

func validProjectName(name string) bool {
	if strings.TrimSpace(name) == "" {
		return false
	}
	return !strings.ContainsAny(name, `/\:*?"<>|`)
}

func (h *Handlers) GetOutlineExport(w http.ResponseWriter, r *http.Request) {
	if !h.ensureProject(w, r) {
		return
	}
	title := h.state.Title
	if h.cfg != nil && h.cfg.Story.Title != "" {
		title = h.cfg.Story.Title
	}
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="outlines.md"`)
	if i18n.NormalizeLanguage(h.cfg.Language) == i18n.LangEN {
		fmt.Fprintf(w, "# %s — Chapter Outlines\n\n", title)
		if strings.TrimSpace(h.state.LongTermDirection) != "" {
			fmt.Fprintf(w, "## Long-term direction\n\n%s\n\n", h.state.LongTermDirection)
		}
		for _, batch := range h.state.OutlineBatches {
			fmt.Fprintf(w, "## Chapters %d–%d batch synopsis\n\n%s\n\n", batch.StartCh, batch.EndCh, batch.Synopsis)
		}
		for _, ch := range h.state.Chapters {
			fmt.Fprintf(w, "## Chapter %d: %s\n\n%s\n\n", ch.Num, ch.Title, ch.Outline)
		}
	} else {
		fmt.Fprintf(w, "# %s — 章节大纲\n\n", title)
		if strings.TrimSpace(h.state.LongTermDirection) != "" {
			fmt.Fprintf(w, "## 全书长期走向\n\n%s\n\n", h.state.LongTermDirection)
		}
		for _, batch := range h.state.OutlineBatches {
			fmt.Fprintf(w, "## 第 %d–%d 章批次梗概\n\n%s\n\n", batch.StartCh, batch.EndCh, batch.Synopsis)
		}
		for _, ch := range h.state.Chapters {
			fmt.Fprintf(w, "## 第 %d 章 %s\n\n%s\n\n", ch.Num, ch.Title, ch.Outline)
		}
	}
}

func (h *Handlers) PostContinuationProject(w http.ResponseWriter, r *http.Request) {
	if !h.ensureProject(w, r) || h.rejectIfTaskRunning(w, r) {
		return
	}
	if !story.IsBookFullyAccepted(h.state) {
		h.writeErrorReq(w, r, http.StatusConflict, "book_not_complete")
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil || strings.TrimSpace(req.Name) == "" {
		h.writeErrorReq(w, r, http.StatusBadRequest, "missing_project_name")
		return
	}
	if !validProjectName(req.Name) {
		h.writeErrorReq(w, r, http.StatusBadRequest, "project_name_invalid_chars")
		return
	}
	name := strings.TrimSpace(req.Name)
	dir := filepath.Join(h.storysDir(), name)
	if _, err := os.Stat(dir); err == nil {
		h.writeErrorReq(w, r, http.StatusConflict, "project_exists")
		return
	}
	if err := os.MkdirAll(filepath.Join(dir, "sessions"), 0755); err != nil {
		h.writeErrorReq(w, r, 500, "create_project_dir_failed", err)
		return
	}

	cfg := *h.cfg
	cfg.CreatedWithVersion = h.version
	cfg.ProjectFormatVersion = config.ProjectFormatVersion
	if err := config.SaveConfig(filepath.Join(dir, "config.json"), &cfg); err != nil {
		h.writeErrorReq(w, r, 500, "init_project_config_failed", err)
		return
	}
	settingsData, _ := json.Marshal(h.settings)
	var settings story.ProjectSettings
	_ = json.Unmarshal(settingsData, &settings)
	settings.StoryChanges = nil
	settings.StorySynced = map[int]string{}
	if err := story.SaveProjectSettings(filepath.Join(dir, "settings.json"), &settings); err != nil {
		h.writeErrorReq(w, r, 500, "save_failed", err)
		return
	}

	next := &story.Progress{Phase: "writing", BookStatus: story.BookStatusActive, Title: h.state.Title, CorePrompt: h.state.CorePrompt, LongTermDirection: h.state.LongTermDirection, StoryConfigSnapshot: h.state.StoryConfigSnapshot, NextMemoryID: h.state.NextMemoryID, Foreshadows: append([]story.Foreshadow(nil), h.state.Foreshadows...), NarrativeCheckpoints: append([]story.NarrativeCheckpoint(nil), h.state.NarrativeCheckpoints...)}
	next.OutlineBatches = append([]story.OutlineBatch(nil), h.state.OutlineBatches...)
	for i := range next.OutlineBatches {
		next.OutlineBatches[i].PlannedFinal = false
	}
	for _, old := range h.state.Chapters {
		next.Chapters = append(next.Chapters, story.ChapterState{Num: old.Num, Title: old.Title, Outline: old.Outline, Characters: append([]story.OutlineChapterCharacter(nil), old.Characters...), Summary: old.Summary, Status: story.StatusAccepted})
	}
	for _, old := range h.state.MemoryEntries {
		old.References = nil
		old.Snippet = ""
		old.Inherited = true
		next.MemoryEntries = append(next.MemoryEntries, old)
	}
	next.CurrentChapterIndex = len(next.Chapters)
	if err := story.SaveProgress(filepath.Join(dir, "progress.json"), next); err != nil {
		h.writeErrorReq(w, r, 500, "save_failed", err)
		return
	}
	h.logger.InfoKey("log.project_created", name)
	h.writeJSON(w, 200, map[string]string{"name": name, "language": cfg.Language})
}
