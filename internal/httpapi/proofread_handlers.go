package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"showmethestory/internal/story"
)

func (h *Handlers) proofreadReady(w http.ResponseWriter, r *http.Request) bool {
	if !h.ensureProject(w, r) {
		return false
	}
	if !story.IsBookFullyAccepted(h.state) {
		h.writeErrorReq(w, r, http.StatusConflict, "book_not_complete")
		return false
	}
	return true
}

func (h *Handlers) GetProofread(w http.ResponseWriter, r *http.Request) {
	if !h.ensureProject(w, r) {
		return
	}
	h.writeJSON(w, http.StatusOK, h.postprocess)
}

func (h *Handlers) DeleteProofread(w http.ResponseWriter, r *http.Request) {
	if !h.ensureProject(w, r) || h.rejectIfTaskRunning(w, r) {
		return
	}
	keep := h.postprocess
	h.postprocess = story.NewProofreadState()
	h.postprocess.BackupAcknowledged = keep.BackupAcknowledged
	h.postprocess.ContentModified = keep.ContentModified
	h.postprocess.Revisions = keep.Revisions
	if err := story.SavePostProcess(h.postprocessPath, h.postprocess); err != nil {
		h.writeErrorReq(w, r, 500, "save_failed", err)
		return
	}
	h.writeJSON(w, 200, h.postprocess)
}

func (h *Handlers) PostProofreadBackup(w http.ResponseWriter, r *http.Request) {
	if !h.proofreadReady(w, r) || h.rejectIfTaskRunning(w, r) {
		return
	}
	h.postprocess.BackupAcknowledged = true
	if err := story.SavePostProcess(h.postprocessPath, h.postprocess); err != nil {
		h.writeErrorReq(w, r, 500, "save_failed", err)
		return
	}
	h.writeJSON(w, 200, h.postprocess)
}

func (h *Handlers) beginProofreadTask(w http.ResponseWriter, r *http.Request) bool {
	if !h.proofreadReady(w, r) {
		return false
	}
	if !h.postprocess.BackupAcknowledged {
		h.writeErrorReq(w, r, http.StatusConflict, "proofread_backup_required")
		return false
	}
	if !h.tryStartTask() {
		h.writeErrorReq(w, r, http.StatusConflict, "task_running_wait")
		return false
	}
	h.taskMu.Lock()
	h.skipKnowledgeSync = true
	h.taskMu.Unlock()
	return true
}

func (h *Handlers) PostProofreadAnalyze(w http.ResponseWriter, r *http.Request) {
	if !h.beginProofreadTask(w, r) {
		return
	}
	go func() {
		defer h.endTask()
		h.logger.TaskStart("proofread_analyze")
		err := story.AnalyzeProofread(h.taskCtx, h.apiCfg, h.cfg, h.settings, h.state, h.postprocess, h.logger)
		if err == nil {
			err = story.SavePostProcess(h.postprocessPath, h.postprocess)
		}
		if err != nil {
			h.logger.ErrorKey("log.postprocess_diagnose_failed", err)
		}
		h.logger.TaskEnd("proofread_analyze", err == nil)
		h.broadcastProgress()
	}()
	h.writeJSON(w, http.StatusAccepted, map[string]string{"status": "started"})
}

func (h *Handlers) PostProofreadApply(w http.ResponseWriter, r *http.Request) {
	if !h.beginProofreadTask(w, r) {
		return
	}
	var body struct {
		Preferences string `json:"preferences"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	h.postprocess.AuthorRequirements = strings.TrimSpace(body.Preferences)
	go func() {
		defer h.endTask()
		h.logger.TaskStart("proofread_apply")
		err := story.ApplyProofread(h.taskCtx, h.apiCfg, h.cfg, h.state, h.settings, h.postprocess, h.progressPath, h.settingsPath, h.postprocessPath, h.skills, h.logger)
		if saveErr := story.SavePostProcess(h.postprocessPath, h.postprocess); err == nil {
			err = saveErr
		}
		if err != nil {
			h.logger.ErrorKey("log.postprocess_execute_failed", err)
		}
		h.logger.TaskEnd("proofread_apply", err == nil)
		h.broadcastProgress()
	}()
	h.writeJSON(w, http.StatusAccepted, map[string]string{"status": "started"})
}

func (h *Handlers) PutProofreadIssue(w http.ResponseWriter, r *http.Request) {
	if !h.ensureProject(w, r) {
		return
	}
	if h.rejectIfTaskRunning(w, r) {
		return
	}
	var body struct {
		Status string `json:"status"`
	}
	if json.NewDecoder(r.Body).Decode(&body) != nil || (body.Status != "pending" && body.Status != "resolved" && body.Status != "ignored") {
		h.writeErrorReq(w, r, 400, "invalid_json")
		return
	}
	found := false
	for i := range h.postprocess.Issues {
		if h.postprocess.Issues[i].ID == r.PathValue("id") {
			h.postprocess.Issues[i].Status = body.Status
			found = true
			break
		}
	}
	if !found {
		h.writeErrorReq(w, r, 404, "proofread_issue_not_found")
		return
	}
	if err := story.SavePostProcess(h.postprocessPath, h.postprocess); err != nil {
		h.writeErrorReq(w, r, 500, "save_failed", err)
		return
	}
	h.writeJSON(w, 200, h.postprocess)
}

func (h *Handlers) PostProofreadUndo(w http.ResponseWriter, r *http.Request) {
	if !h.proofreadReady(w, r) {
		return
	}
	if h.rejectIfTaskRunning(w, r) {
		return
	}
	num, err := strconv.Atoi(r.PathValue("num"))
	if err != nil {
		h.writeErrorReq(w, r, 400, "invalid_chapter_num")
		return
	}
	stateSnapshot, _ := json.Marshal(h.state)
	settingsSnapshot, _ := json.Marshal(h.settings)
	postprocessSnapshot, _ := json.Marshal(h.postprocess)
	if err := story.UndoProofreadChapter(h.state, h.settings, h.postprocess, num); err != nil {
		h.writeErrorReq(w, r, http.StatusConflict, "proofread_undo_conflict", err)
		return
	}
	if err := story.SaveProjectSettings(h.settingsPath, h.settings); err != nil {
		_ = json.Unmarshal(stateSnapshot, h.state)
		_ = json.Unmarshal(settingsSnapshot, h.settings)
		_ = json.Unmarshal(postprocessSnapshot, h.postprocess)
		h.writeErrorReq(w, r, 500, "save_failed", err)
		return
	}
	if err := story.SaveProgress(h.progressPath, h.state); err != nil {
		_ = json.Unmarshal(stateSnapshot, h.state)
		_ = json.Unmarshal(settingsSnapshot, h.settings)
		_ = json.Unmarshal(postprocessSnapshot, h.postprocess)
		_ = story.SaveProjectSettings(h.settingsPath, h.settings)
		_ = story.SaveProgress(h.progressPath, h.state)
		h.writeErrorReq(w, r, 500, "save_failed", err)
		return
	}
	if err := story.SavePostProcess(h.postprocessPath, h.postprocess); err != nil {
		_ = json.Unmarshal(stateSnapshot, h.state)
		_ = json.Unmarshal(settingsSnapshot, h.settings)
		_ = json.Unmarshal(postprocessSnapshot, h.postprocess)
		_ = story.SaveProjectSettings(h.settingsPath, h.settings)
		_ = story.SaveProgress(h.progressPath, h.state)
		h.writeErrorReq(w, r, 500, "save_failed", err)
		return
	}
	story.SaveChapterMarkdown(h.projectDir(), h.state.Chapters[story.FindChapterIdx(h.state, num)], h.state.Title)
	h.broadcastProgress()
	h.writeJSON(w, 200, h.postprocess)
}

func (h *Handlers) proofreadBlockEdit(w http.ResponseWriter, r *http.Request, edit func(*story.ChapterState) error) {
	if !h.proofreadReady(w, r) || h.rejectIfTaskRunning(w, r) {
		return
	}
	if !h.postprocess.BackupAcknowledged {
		h.writeErrorReq(w, r, http.StatusConflict, "proofread_backup_required")
		return
	}
	num, err := strconv.Atoi(r.PathValue("num"))
	if err != nil {
		h.writeErrorReq(w, r, 400, "invalid_chapter_num")
		return
	}
	i := story.FindChapterIdx(h.state, num)
	if i < 0 {
		h.writeErrorReq(w, r, 404, "chapter_not_found")
		return
	}
	stateSnapshot, _ := json.Marshal(h.state)
	settingsSnapshot, _ := json.Marshal(h.settings)
	postprocessSnapshot, _ := json.Marshal(h.postprocess)
	if err := edit(&h.state.Chapters[i]); err != nil {
		h.writeErrorReq(w, r, 400, "chapter_edit_failed", err)
		return
	}
	story.RefreshProofreadAnchors(h.state, h.settings, &h.state.Chapters[i])
	h.postprocess.ContentModified = true
	if err := story.SaveProjectSettings(h.settingsPath, h.settings); err != nil {
		_ = json.Unmarshal(stateSnapshot, h.state)
		_ = json.Unmarshal(settingsSnapshot, h.settings)
		_ = json.Unmarshal(postprocessSnapshot, h.postprocess)
		h.writeErrorReq(w, r, 500, "save_failed", err)
		return
	}
	if err := story.SaveProgress(h.progressPath, h.state); err != nil {
		_ = json.Unmarshal(stateSnapshot, h.state)
		_ = json.Unmarshal(settingsSnapshot, h.settings)
		_ = story.SaveProjectSettings(h.settingsPath, h.settings)
		_ = story.SaveProgress(h.progressPath, h.state)
		_ = json.Unmarshal(postprocessSnapshot, h.postprocess)
		h.writeErrorReq(w, r, 500, "save_failed", err)
		return
	}
	if err := story.SavePostProcess(h.postprocessPath, h.postprocess); err != nil {
		_ = json.Unmarshal(stateSnapshot, h.state)
		_ = json.Unmarshal(settingsSnapshot, h.settings)
		_ = json.Unmarshal(postprocessSnapshot, h.postprocess)
		_ = story.SaveProjectSettings(h.settingsPath, h.settings)
		_ = story.SaveProgress(h.progressPath, h.state)
		h.writeErrorReq(w, r, 500, "save_failed", err)
		return
	}
	story.SaveChapterMarkdown(h.projectDir(), h.state.Chapters[i], h.state.Title)
	h.broadcastProgress()
	h.writeJSON(w, 200, h.state.Chapters[i])
}

func (h *Handlers) PutProofreadBlock(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Text string `json:"text"`
	}
	if json.NewDecoder(r.Body).Decode(&body) != nil || strings.TrimSpace(body.Text) == "" {
		h.writeErrorReq(w, r, 400, "block_text_required")
		return
	}
	id, _ := strconv.Atoi(r.PathValue("id"))
	h.proofreadBlockEdit(w, r, func(ch *story.ChapterState) error { return story.UpdateBlock(ch, id, body.Text) })
}
func (h *Handlers) DeleteProofreadBlock(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(r.PathValue("id"))
	h.proofreadBlockEdit(w, r, func(ch *story.ChapterState) error { return story.DeleteBlock(ch, id) })
}
func (h *Handlers) PostProofreadBlock(w http.ResponseWriter, r *http.Request) {
	var body struct {
		AfterID int    `json:"after_id"`
		Text    string `json:"text"`
	}
	if json.NewDecoder(r.Body).Decode(&body) != nil || strings.TrimSpace(body.Text) == "" {
		h.writeErrorReq(w, r, 400, "block_text_required")
		return
	}
	h.proofreadBlockEdit(w, r, func(ch *story.ChapterState) error {
		_, err := story.InsertBlockAfter(ch, body.AfterID, body.Text)
		return err
	})
}

func (h *Handlers) GetProofreadExport(w http.ResponseWriter, r *http.Request) {
	if !h.ensureProject(w, r) {
		return
	}
	title := h.state.Title
	if h.cfg.Story.Title != "" {
		title = h.cfg.Story.Title
	}
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="proofread-report-%d.md"`, time.Now().Unix()))
	english := h.cfg.Language == "en"
	if english {
		fmt.Fprintf(w, "# %s — Final Proofreading Report\n\n", title)
	} else {
		fmt.Fprintf(w, "# %s — 完稿校订报告\n\n", title)
	}
	for _, issue := range h.postprocess.Issues {
		if english {
			fmt.Fprintf(w, "## [%s/%s/%s] %s\n\n%s\n\nSuggestion: %s\n\n", issue.Status, issue.Severity, issue.Category, issue.Title, issue.Detail, issue.Suggestion)
		} else {
			fmt.Fprintf(w, "## [%s/%s/%s] %s\n\n%s\n\n建议：%s\n\n", issue.Status, issue.Severity, issue.Category, issue.Title, issue.Detail, issue.Suggestion)
		}
		for _, a := range issue.Anchors {
			if english {
				fmt.Fprintf(w, "- Chapter %d · block %d: %s\n", a.ChapterNum, a.BlockID, a.Excerpt)
			} else {
				fmt.Fprintf(w, "- 第 %d 章 · 段落 %d：%s\n", a.ChapterNum, a.BlockID, a.Excerpt)
			}
		}
		fmt.Fprintln(w)
	}
}
