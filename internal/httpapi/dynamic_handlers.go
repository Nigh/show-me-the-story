package httpapi

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"showmethestory/internal/i18n"
	"showmethestory/internal/llm"
	"showmethestory/internal/story"
)

// PostStoryComplete explicitly closes a dynamically planned book. Outstanding
// drafts must be resolved first; active foreshadows are returned for an
// explicit second confirmation instead of silently being discarded.
func (h *Handlers) PostStoryComplete(w http.ResponseWriter, r *http.Request) {
	if h.rejectIfTaskRunning(w, r) {
		return
	}
	for _, ch := range h.state.Chapters {
		if ch.Status != story.StatusAccepted || ch.Content == "" {
			h.writeErrorReq(w, r, http.StatusConflict, "book_not_complete")
			return
		}
	}
	confirmForeshadows := r.URL.Query().Get("confirm_foreshadows") == "true"
	active := 0
	for _, fs := range h.state.Foreshadows {
		if fs.Status != story.ForeshadowResolved && fs.Status != story.ForeshadowAbandoned {
			active++
		}
	}
	if active > 0 && !confirmForeshadows {
		h.writeJSON(w, http.StatusConflict, map[string]any{
			"error":              "active_foreshadows",
			"active_foreshadows": active,
		})
		return
	}
	h.state.BookStatus = story.BookStatusCompleted
	if err := story.SaveProgress(h.progressPath, h.state); err != nil {
		h.writeErrorReq(w, r, http.StatusInternalServerError, "save_failed", err)
		return
	}
	h.writeJSON(w, http.StatusOK, story.ProgressView(h.state))
}

func (h *Handlers) PostPlanningReview(w http.ResponseWriter, r *http.Request) {
	if !h.tryStartTask() {
		h.writeErrorReq(w, r, http.StatusConflict, "task_running_wait")
		return
	}
	go func() {
		defer h.endTask()
		h.logger.TaskStart("planning_review")
		var history strings.Builder
		through := 0
		for _, ch := range h.state.Chapters {
			if ch.Status == story.StatusAccepted {
				fmt.Fprintf(&history, "[%d %s] %s\n", ch.Num, ch.Title, ch.Summary)
				through = ch.Num
			}
		}
		prompt := "Review the accepted novel chapters below. Summarize plot and character state, identify consistency or pacing risks and active foreshadows, then propose several optional directions for the next planning batch. Do not decide for the author.\n\n" + history.String()
		if i18n.NormalizeLanguage(h.cfg.Language) == i18n.LangZH {
			prompt = "复盘以下已确认章节：总结剧情与人物状态，指出一致性、节奏风险和活跃伏笔，并提出数个下一批剧情方向供作者选择，不要替作者作决定。\n\n" + history.String()
		}
		content := llm.CallAPIWithRetryLog(h.taskCtx, h.apiCfg, i18n.SystemPromptFor(h.cfg.Language, "author_default"), prompt, h.logger)
		if strings.TrimSpace(content) == "" {
			h.logger.TaskEnd("planning_review", false)
			return
		}
		h.state.LatestPlanningReview = &story.PlanningReview{ThroughChapter: through, Content: strings.TrimSpace(content), CreatedAt: time.Now().Format(time.RFC3339)}
		if err := story.SaveProgress(h.progressPath, h.state); err != nil {
			h.logger.TaskEnd("planning_review", false)
			return
		}
		h.logger.TaskEnd("planning_review", true)
		h.broadcastProgress()
	}()
	h.writeJSON(w, http.StatusAccepted, map[string]string{"status": "started"})
}

func (h *Handlers) PostStoryResume(w http.ResponseWriter, r *http.Request) {
	if h.rejectIfTaskRunning(w, r) {
		return
	}
	h.state.BookStatus = story.BookStatusActive
	if err := story.SaveProgress(h.progressPath, h.state); err != nil {
		h.writeErrorReq(w, r, http.StatusInternalServerError, "save_failed", err)
		return
	}
	h.writeJSON(w, http.StatusOK, story.ProgressView(h.state))
}
