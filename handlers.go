package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Handlers struct {
	apiCfg     *APIConfig
	apiCfgPath string
	logger     *LogBroadcaster

	// Project management
	progDir     string
	projectName string
	projectMu   sync.RWMutex

	// Per-project state (updated on switchProject)
	cfg          *Config
	cfgPath      string
	state        *Progress
	progressPath string
	settings     *ProjectSettings
	settingsPath string
	skills       []Skill
	sessionsDir  string
	postprocess     *PostProcessState
	postprocessPath string

	// Task management
	taskMu      sync.Mutex
	taskRunning bool
	activeWork  int
	taskCtx     context.Context
	taskCancel  context.CancelFunc
	taskTokens  *TaskTokenUsage
	autoConfirm bool // 自动确认模式：章节生成完成后自动确认并继续生成下一章

	pendingContinueContent string
	lastChatMessage        string      // 缓存最后发送的聊天消息，用于重试
	lastReconcileBody      StoryConfig // 缓存最后的设定协调请求
}

func NewHandlers(apiCfg *APIConfig, apiCfgPath string, logger *LogBroadcaster, progDir string) *Handlers {
	return &Handlers{
		apiCfg:     apiCfg,
		apiCfgPath: apiCfgPath,
		logger:     logger,
		progDir:    progDir,
		cfg:        DefaultConfig(),
		state:      &Progress{Phase: "outline"},
		settings:   &ProjectSettings{},
		postprocess: &PostProcessState{
			ExecuteOptions: &PostProcessExecuteOptions{RunSmoothTransitionsFirst: true},
		},
	}
}

func (h *Handlers) storysDir() string {
	return filepath.Join(h.progDir, "storys")
}

// projectDir returns the current project's directory (empty if no project selected).
func (h *Handlers) projectDir() string {
	h.projectMu.RLock()
	defer h.projectMu.RUnlock()
	if h.projectName == "" {
		return h.progDir
	}
	return filepath.Join(h.progDir, "storys", h.projectName)
}

// switchProject loads all project-specific data for the given project name.
func (h *Handlers) switchProject(name string) error {
	h.projectMu.Lock()
	defer h.projectMu.Unlock()

	projectDir := filepath.Join(h.progDir, "storys", name)
	if info, err := os.Stat(projectDir); err != nil || !info.IsDir() {
		return fmt.Errorf("项目目录不存在: %s", name)
	}

	configPath := filepath.Join(projectDir, "config.json")
	progressPath := filepath.Join(projectDir, "progress.json")
	settingsPath := filepath.Join(projectDir, "settings.json")
	sessionsDir := filepath.Join(projectDir, "sessions")
	os.MkdirAll(sessionsDir, 0755)

	cfg, err := LoadConfig(configPath)
	if err != nil {
		return fmt.Errorf("加载项目配置失败: %w", err)
	}

	state, err := LoadProgress(progressPath)
	if err != nil {
		return fmt.Errorf("加载项目进度失败: %w", err)
	}
	if state == nil {
		state = &Progress{Phase: "outline"}
	}

	settings, err := LoadProjectSettings(settingsPath)
	if err != nil {
		return fmt.Errorf("加载项目设定失败: %w", err)
	}

	skills := LoadAllSkills(cfg, projectDir)

	postprocessPath := filepath.Join(projectDir, "postprocess.json")
	postprocess, err := LoadPostProcess(postprocessPath)
	if err != nil {
		return fmt.Errorf("加载全书优化状态失败: %w", err)
	}

	h.projectName = name
	h.cfg = cfg
	h.cfgPath = configPath
	h.state = state
	h.progressPath = progressPath
	h.settings = settings
	h.settingsPath = settingsPath
	h.skills = skills
	h.sessionsDir = sessionsDir
	h.postprocessPath = postprocessPath
	h.postprocess = postprocess

	fmt.Printf(" [系统] 已切换到项目: %s (%s)\n", name, projectDir)
	return nil
}

// ensureProject returns true if a project is selected, otherwise writes an error response.
func (h *Handlers) ensureProject(w http.ResponseWriter, r *http.Request) bool {
	h.projectMu.RLock()
	defer h.projectMu.RUnlock()
	if h.projectName == "" {
		h.writeErrorReq(w, r, http.StatusBadRequest, "select_project_first")
		return false
	}
	return true
}

func (h *Handlers) writeJSON(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func (h *Handlers) writeError(w http.ResponseWriter, code int, msg string) {
	h.writeJSON(w, code, map[string]string{"error": msg})
}

func (h *Handlers) tryStartTask() bool {
	h.taskMu.Lock()
	defer h.taskMu.Unlock()
	if h.taskRunning || h.activeWork > 0 {
		return false
	}
	h.taskRunning = true
	h.activeWork = 1
	ctx, cancel := context.WithCancel(context.Background())
	ctx, h.taskTokens = withTaskTokens(ctx, h.logger)
	h.taskCtx = ctx
	h.taskCancel = cancel
	return true
}

func (h *Handlers) endTask() {
	h.taskMu.Lock()
	h.activeWork--
	if h.activeWork <= 0 {
		h.activeWork = 0
		h.taskRunning = false
		if h.taskCancel != nil {
			h.taskCancel()
			h.taskCancel = nil
		}
	}
	h.taskMu.Unlock()
}

// startChildWork 增加活跃工作计数（用于 Agent 子任务），不创建新 context
func (h *Handlers) startChildWork() bool {
	h.taskMu.Lock()
	defer h.taskMu.Unlock()
	if !h.taskRunning {
		return false
	}
	h.activeWork++
	return true
}

func (h *Handlers) isTaskRunning() bool {
	h.taskMu.Lock()
	defer h.taskMu.Unlock()
	return h.taskRunning || h.activeWork > 0
}

// rejectIfTaskRunning 在 AI 任务运行期间拒绝编辑类请求，防止意外提交修改。
// 返回 true 表示已写入 409 响应，调用方应直接 return。
func (h *Handlers) rejectIfTaskRunning(w http.ResponseWriter, r *http.Request) bool {
	if h.isTaskRunning() {
		h.writeErrorReq(w, r, http.StatusConflict, "task_running_locked")
		return true
	}
	return false
}

func (h *Handlers) isAutoConfirmOn() bool {
	h.taskMu.Lock()
	defer h.taskMu.Unlock()
	return h.autoConfirm
}

func (h *Handlers) GetAutoConfirm(w http.ResponseWriter, r *http.Request) {
	h.writeJSON(w, http.StatusOK, map[string]bool{"enabled": h.isAutoConfirmOn()})
}

// PutAutoConfirm 切换自动确认模式，任务运行期间也可随时开关。
func (h *Handlers) PutAutoConfirm(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeErrorReq(w, r, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}

	h.taskMu.Lock()
	h.autoConfirm = req.Enabled
	h.taskMu.Unlock()

	if req.Enabled {
		h.logger.InfoKey("log.autoconfirm_on")
	} else {
		h.logger.InfoKey("log.autoconfirm_off")
	}
	h.writeJSON(w, http.StatusOK, map[string]bool{"enabled": req.Enabled})
}

func (h *Handlers) PostTaskStop(w http.ResponseWriter, r *http.Request) {
	h.taskMu.Lock()
	if !h.taskRunning {
		h.taskMu.Unlock()
		h.writeErrorReq(w, r, http.StatusBadRequest, "no_task_running")
		return
	}
	if h.taskCancel != nil {
		h.taskCancel()
	}
	h.taskMu.Unlock()
	h.writeJSON(w, http.StatusOK, map[string]string{"status": "stopping"})
}

func (h *Handlers) GetAPIConfig(w http.ResponseWriter, r *http.Request) {
	h.writeJSON(w, http.StatusOK, h.apiCfg)
}

func (h *Handlers) PutAPIConfig(w http.ResponseWriter, r *http.Request) {
	if h.rejectIfTaskRunning(w, r) {
		return
	}
	var newCfg APIConfig
	if err := json.NewDecoder(r.Body).Decode(&newCfg); err != nil {
		h.writeErrorReq(w, r, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}

	if newCfg.HTTPTimeoutSeconds <= 0 {
		newCfg.HTTPTimeoutSeconds = 300
	}
	if newCfg.ContextBudgetTokens <= 0 {
		if window := FetchModelContextWindow(&newCfg); window > 0 {
			newCfg.ContextBudgetTokens = window
		} else {
			newCfg.ContextBudgetTokens = defaultContextBudgetTokens
		}
	}

	data, err := json.MarshalIndent(newCfg, "", "  ")
	if err != nil {
		h.writeErrorReq(w, r, http.StatusInternalServerError, "serialize_api_config_failed", err.Error())
		return
	}
	if err := writeFileAtomic(h.apiCfgPath, data); err != nil {
		h.writeErrorReq(w, r, http.StatusInternalServerError, "save_api_config_failed", err.Error())
		return
	}

	h.apiCfg = &newCfg
	h.writeJSON(w, http.StatusOK, h.apiCfg)
}

func (h *Handlers) PostAPITest(w http.ResponseWriter, r *http.Request) {
	if h.rejectIfTaskRunning(w, r) {
		return
	}
	var testCfg APIConfig
	if err := json.NewDecoder(r.Body).Decode(&testCfg); err != nil {
		h.writeErrorReq(w, r, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	if err := validateAPIConfig(&testCfg); err != nil {
		h.writeErrorReq(w, r, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	resp, err := CallAPIMessages(ctx, &testCfg, []Message{
		{Role: "user", Content: "Hi"},
	})
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			h.writeErrorReq(w, r, http.StatusGatewayTimeout, "api_test_timeout")
			return
		}
		h.writeErrorReq(w, r, http.StatusBadGateway, "api_test_failed", err.Error())
		return
	}

	result := map[string]interface{}{
		"success": true,
		"message": "连接成功",
		"model":   testCfg.Model,
	}
	if len(resp) > 100 {
		result["sample"] = resp[:100] + "..."
	} else {
		result["sample"] = resp
	}
	h.writeJSON(w, http.StatusOK, result)
}

func (h *Handlers) GetConfig(w http.ResponseWriter, r *http.Request) {
	h.writeJSON(w, http.StatusOK, h.cfg)
}

func (h *Handlers) PutConfig(w http.ResponseWriter, r *http.Request) {
	if h.rejectIfTaskRunning(w, r) {
		return
	}
	var newCfg Config
	if err := json.NewDecoder(r.Body).Decode(&newCfg); err != nil {
		h.writeErrorReq(w, r, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}

	if newCfg.Story.ChapterCount <= 0 {
		newCfg.Story.ChapterCount = 30
	}
	if newCfg.Story.TargetWordsPerChapter <= 0 {
		newCfg.Story.TargetWordsPerChapter = 2500
	}
	newCfg.Language = NormalizeLanguage(newCfg.Language)
	if newCfg.Language == "" {
		newCfg.Language = h.cfg.Language
	}
	newCfg.Prompts.applyDefaults(newCfg.Language)

	data, err := json.MarshalIndent(newCfg, "", "  ")
	if err != nil {
		h.writeErrorReq(w, r, http.StatusInternalServerError, "serialize_config_failed", err.Error())
		return
	}
	if err := writeFileAtomic(h.cfgPath, data); err != nil {
		h.writeErrorReq(w, r, http.StatusInternalServerError, "save_config_failed", err.Error())
		return
	}

	h.cfg = &newCfg
	h.writeJSON(w, http.StatusOK, h.cfg)
}

func (h *Handlers) GetProgress(w http.ResponseWriter, r *http.Request) {
	h.writeJSON(w, http.StatusOK, h.state)
}

func (h *Handlers) DeleteProgress(w http.ResponseWriter, r *http.Request) {
	if h.isTaskRunning() {
		h.writeErrorReq(w, r, http.StatusConflict, "reset_progress_locked")
		return
	}

	if err := deleteFile(h.progressPath); err != nil {
		h.writeErrorReq(w, r, http.StatusInternalServerError, "delete_progress_failed", err.Error())
		return
	}

	h.state = &Progress{Phase: "outline"}
	h.writeJSON(w, http.StatusOK, h.state)
}

func (h *Handlers) PostOutlineGenerate(w http.ResponseWriter, r *http.Request) {
	if !h.ensureProject(w, r) {
		return
	}
	// 检查是否有写作中/审核中/已确认的章节，如果有则拒绝
	for _, ch := range h.state.Chapters {
		if ch.Status == StatusWriting || ch.Status == StatusReview {
			h.writeErrorReq(w, r, http.StatusConflict, "writing_chapter_present")
			return
		}
		if ch.Status == StatusAccepted {
			h.writeErrorReq(w, r, http.StatusConflict, "accepted_chapter_present")
			return
		}
	}

	if !h.tryStartTask() {
		h.writeErrorReq(w, r, http.StatusConflict, "task_running_wait")
		return
	}

	go func() {
		defer h.endTask()

		// 自动清除旧的大纲（仅 pending 章节，保留 accepted 的通过正常流程处理）
		hasPending := false
		for _, ch := range h.state.Chapters {
			if ch.Status == StatusPending {
				hasPending = true
				break
			}
		}
		if hasPending {
			var kept []ChapterState
			for _, ch := range h.state.Chapters {
				if ch.Status != StatusPending {
					kept = append(kept, ch)
				}
			}
			h.state.Chapters = kept
			if len(h.state.Chapters) == 0 {
				h.state.Title = ""
				h.state.CorePrompt = ""
				h.state.StorySynopsis = ""
				h.state.StoryConfigSnapshot = nil
				h.state.CurrentChapterIndex = 0
			}
			h.logger.InfoKey("log.outline_cleared_pending")
		}
		h.logger.TaskStart("outline_generation")
		ctx := h.taskCtx

		h.logger.InfoKey("log.outline_generating")
		err := GenerateOutlineAction(ctx, h.apiCfg, h.cfg, h.state, h.progressPath, h.logger)

		if err != nil {
			if ctx.Err() != nil {
				h.logger.WarnKey("log.outline_generate_cancelled")
				h.logger.TaskEnd("outline_generation", false)
			} else {
				h.logger.ErrorKey("log.outline_generate_failed", err)
				h.logger.TaskEnd("outline_generation", false)
			}
			return
		}

		h.logger.SuccessKey("log.outline_generate_done")
		h.logger.TaskEnd("outline_generation", true)
		h.broadcastProgress()
	}()

	h.writeJSON(w, http.StatusAccepted, map[string]string{"status": "started"})
}

func (h *Handlers) PostOutlineConfirm(w http.ResponseWriter, r *http.Request) {
	if h.isTaskRunning() {
		h.writeErrorReq(w, r, http.StatusConflict, "task_running_wait")
		return
	}

	if h.state.Phase != "outline" {
		h.writeErrorReq(w, r, http.StatusBadRequest, "phase_not_outline")
		return
	}

	if len(h.state.Chapters) == 0 {
		h.writeErrorReq(w, r, http.StatusBadRequest, "outline_empty")
		return
	}

	if err := ConfirmOutlineAction(h.state, h.progressPath); err != nil {
		h.writeErrorReq(w, r, http.StatusInternalServerError, "outline_confirm_failed", err.Error())
		return
	}

	h.logger.SuccessKey("log.outline_confirmed")
	h.writeJSON(w, http.StatusOK, h.state)
}

func (h *Handlers) PostOutlineRevise(w http.ResponseWriter, r *http.Request) {
	if !h.tryStartTask() {
		h.writeErrorReq(w, r, http.StatusConflict, "task_running_wait")
		return
	}

	var body struct {
		Feedback string `json:"feedback"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Feedback == "" {
		h.endTask()
		h.writeErrorReq(w, r, http.StatusBadRequest, "missing_feedback")
		return
	}

	go func() {
		defer h.endTask()
		h.logger.TaskStart("outline_revision")
		ctx := h.taskCtx

		h.logger.InfoKey("log.outline_revising")
		err := ReviseOutlineAction(ctx, h.apiCfg, h.cfg, h.state, h.progressPath, body.Feedback, h.logger)

		if err != nil {
			if ctx.Err() != nil {
				h.logger.WarnKey("log.outline_revise_cancelled")
				h.logger.TaskEnd("outline_revision", false)
			} else {
				h.logger.ErrorKey("log.outline_revise_failed", err)
				h.logger.TaskEnd("outline_revision", false)
			}
			return
		}

		h.logger.SuccessKey("log.outline_revised")
		h.logger.TaskEnd("outline_revision", true)
		h.broadcastProgress()
	}()

	h.writeJSON(w, http.StatusAccepted, map[string]string{"status": "started"})
}

func (h *Handlers) PostChapterGenerate(w http.ResponseWriter, r *http.Request) {
	if !h.tryStartTask() {
		h.writeErrorReq(w, r, http.StatusConflict, "task_running_wait")
		return
	}

	go func() {
		defer h.endTask()
		h.logger.TaskStart("chapter_generation")
		ctx := h.taskCtx

		for {
			chIdx := h.state.CurrentChapterIndex
			chTitle := ""
			if chIdx < len(h.state.Chapters) {
				chTitle = h.state.Chapters[chIdx].Title
			}

			h.logger.InfoKey("log.chapter_writing", chIdx+1)
			err := GenerateChapterAction(ctx, h.apiCfg, h.cfg, h.state, h.progressPath, h.settings, h.logger)

			if err != nil {
				if ctx.Err() != nil {
					h.logger.WarnKey("log.chapter_write_cancelled")
				} else {
					var wcErr *WritingConflictError
					if errors.As(err, &wcErr) {
						h.logger.WarnKey("log.chapter_write_conflict_pause")
					} else {
						h.logger.ErrorKey("log.chapter_write_failed", err)
					}
				}
				h.logger.TaskEnd("chapter_generation", false)
				h.broadcastProgress()
				return
			}

			h.logger.SuccessKey("log.chapter_write_done", chIdx+1, chTitle)
			h.broadcastProgress()

			// 自动确认模式：自动确认本章并继续生成下一章；关闭开关后在本章结束时停止
			if !h.isAutoConfirmOn() {
				break
			}
			if err := ConfirmChapterAction(h.state, h.progressPath); err != nil {
				h.logger.WarnKey("log.chapter_autoconfirm_failed", err)
				break
			}
			h.logger.SuccessKey("log.chapter_autoconfirmed", chIdx+1, chTitle)
			h.broadcastProgress()

			if h.state.CurrentChapterIndex >= len(h.state.Chapters) {
				h.logger.SuccessKey("log.all_chapters_done")
				break
			}
			if ctx.Err() != nil {
				h.logger.WarnKey("log.autowrite_cancelled")
				break
			}
		}

		h.logger.TaskEnd("chapter_generation", true)
		h.broadcastProgress()
	}()

	h.writeJSON(w, http.StatusAccepted, map[string]string{"status": "started"})
}

func (h *Handlers) GetChapterConflict(w http.ResponseWriter, r *http.Request) {
	h.writeJSON(w, http.StatusOK, map[string]interface{}{
		"conflict": h.state.PendingWritingConflict,
	})
}

func (h *Handlers) PostChapterConflictResolve(w http.ResponseWriter, r *http.Request) {
	if h.isTaskRunning() {
		h.writeErrorReq(w, r, http.StatusConflict, "task_running_wait")
		return
	}
	if h.state.PendingWritingConflict == nil {
		h.writeErrorReq(w, r, http.StatusBadRequest, "writing_conflict_none")
		return
	}

	var body struct {
		Action string `json:"action"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Action == "" {
		h.writeErrorReq(w, r, http.StatusBadRequest, "missing_action")
		return
	}

	conflict := h.state.PendingWritingConflict
	idx := conflict.ChapterIndex
	if idx < 0 || idx >= len(h.state.Chapters) {
		h.writeErrorReq(w, r, http.StatusBadRequest, "invalid_conflict_chapter_idx")
		return
	}
	ch := &h.state.Chapters[idx]

	switch body.Action {
	case "force_review":
		ch.Status = StatusReview
		h.state.PendingWritingConflict = nil
		if err := SaveProgress(h.progressPath, h.state); err != nil {
			h.writeErrorReq(w, r, http.StatusInternalServerError, "save_progress_failed", err.Error())
			return
		}
		h.logger.SuccessKey("log.chapter_kept_review", ch.Num)
		h.broadcastProgress()
		h.writeJSON(w, http.StatusOK, h.state)
	case "dismiss":
		h.state.PendingWritingConflict = nil
		if err := SaveProgress(h.progressPath, h.state); err != nil {
			h.writeErrorReq(w, r, http.StatusInternalServerError, "save_progress_failed", err.Error())
			return
		}
		h.broadcastProgress()
		h.writeJSON(w, http.StatusOK, h.state)
	case "retry":
		h.state.PendingWritingConflict = nil
		if err := SaveProgress(h.progressPath, h.state); err != nil {
			h.writeErrorReq(w, r, http.StatusInternalServerError, "save_progress_failed", err.Error())
			return
		}
		h.broadcastProgress()
		h.writeJSON(w, http.StatusAccepted, map[string]string{"status": "retry"})
	default:
		h.writeErrorReq(w, r, http.StatusBadRequest, "unsupported_action", body.Action)
	}
}

func (h *Handlers) PostForeshadowOutlineCheck(w http.ResponseWriter, r *http.Request) {
	if !h.tryStartTask() {
		h.writeErrorReq(w, r, http.StatusConflict, "task_running_wait")
		return
	}
	if len(h.state.Foreshadows) == 0 {
		h.endTask()
		h.writeErrorReq(w, r, http.StatusBadRequest, "no_foreshadows_to_check")
		return
	}

	go func() {
		defer h.endTask()
		h.logger.TaskStart("foreshadow_outline_check")
		ctx := h.taskCtx
		RunForeshadowOutlineCheckAndSave(ctx, h.apiCfg, h.cfg, h.state, h.progressPath, h.logger)
		h.logger.TaskEnd("foreshadow_outline_check", true)
		h.broadcastProgress()
	}()

	h.writeJSON(w, http.StatusAccepted, map[string]string{"status": "started"})
}

func (h *Handlers) PostChapterConfirm(w http.ResponseWriter, r *http.Request) {
	if h.isTaskRunning() {
		h.writeErrorReq(w, r, http.StatusConflict, "task_running_wait")
		return
	}

	if h.state.Phase != "writing" {
		h.writeErrorReq(w, r, http.StatusBadRequest, "phase_not_writing")
		return
	}

	if err := ConfirmChapterAction(h.state, h.progressPath); err != nil {
		h.writeErrorReq(w, r, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}

	ch := h.state.Chapters[h.state.CurrentChapterIndex-1]
	h.logger.SuccessKey("log.chapter_confirmed", ch.Num)
	h.writeJSON(w, http.StatusOK, h.state)
}

func (h *Handlers) PostChapterEdit(w http.ResponseWriter, r *http.Request) {
	if h.rejectIfTaskRunning(w, r) {
		return
	}

	var req EditChapterContentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeErrorReq(w, r, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	if req.Operation == "" {
		h.writeErrorReq(w, r, http.StatusBadRequest, "chapter_edit_op_required")
		return
	}
	if req.NewText == "" && req.Operation != EditOpReplaceText {
		h.writeErrorReq(w, r, http.StatusBadRequest, "chapter_edit_text_required")
		return
	}

	totalLines, err := EditChapterContent(h.state, req)
	if err != nil {
		h.writeErrorReq(w, r, http.StatusBadRequest, "chapter_edit_failed", err.Error())
		return
	}

	if err := SaveProgress(h.progressPath, h.state); err != nil {
		h.writeErrorReq(w, r, http.StatusInternalServerError, "save_progress_failed", err.Error())
		return
	}

	SaveChapterMarkdown(h.projectDir(), h.getChapterByNum(req.ChapterNum), "")
	h.broadcastProgress()
	h.writeJSON(w, http.StatusOK, map[string]interface{}{
		"success":     true,
		"total_lines": totalLines,
		"chapter":     h.getChapterByNum(req.ChapterNum),
	})
}

func (h *Handlers) getChapterByNum(num int) ChapterState {
	for _, ch := range h.state.Chapters {
		if ch.Num == num {
			return ch
		}
	}
	return ChapterState{}
}

func (h *Handlers) PostChapterRevise(w http.ResponseWriter, r *http.Request) {
	if !h.tryStartTask() {
		h.writeErrorReq(w, r, http.StatusConflict, "task_running_wait")
		return
	}

	var body struct {
		Feedback string `json:"feedback"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Feedback == "" {
		h.endTask()
		h.writeErrorReq(w, r, http.StatusBadRequest, "missing_feedback")
		return
	}

	go func() {
		defer h.endTask()
		h.logger.TaskStart("chapter_revision")
		ctx := h.taskCtx

		h.logger.InfoKey("log.chapter_revising")
		err := ReviseChapterAction(ctx, h.apiCfg, h.cfg, h.state, h.progressPath, body.Feedback, h.settings, h.logger)

		if err != nil {
			if ctx.Err() != nil {
				h.logger.WarnKey("log.chapter_revise_cancelled")
				h.logger.TaskEnd("chapter_revision", false)
			} else {
				h.logger.ErrorKey("log.chapter_revise_failed", err)
				h.logger.TaskEnd("chapter_revision", false)
			}
			return
		}

		h.logger.SuccessKey("log.chapter_revised")
		h.logger.TaskEnd("chapter_revision", true)
		h.broadcastProgress()
	}()

	h.writeJSON(w, http.StatusAccepted, map[string]string{"status": "started"})
}

// PostChapterReviseSpecific 对指定编号章节做定向最小化修订（含已确认章节），
// 仅修改该章正文与摘要，不影响其他章节和大纲。
func (h *Handlers) PostChapterReviseSpecific(w http.ResponseWriter, r *http.Request) {
	numStr := r.PathValue("num")
	var num int
	if _, err := fmt.Sscanf(numStr, "%d", &num); err != nil {
		h.writeErrorReq(w, r, http.StatusBadRequest, "invalid_chapter_num")
		return
	}

	if !h.tryStartTask() {
		h.writeErrorReq(w, r, http.StatusConflict, "task_running_wait")
		return
	}

	var body struct {
		Feedback string `json:"feedback"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Feedback == "" {
		h.endTask()
		h.writeErrorReq(w, r, http.StatusBadRequest, "missing_feedback")
		return
	}

	go func() {
		defer h.endTask()
		h.logger.TaskStart("chapter_revision")
		ctx := h.taskCtx

		h.logger.InfoKey("log.chapter_specific_revising", num)
		err := ReviseSpecificChapterAction(ctx, h.apiCfg, h.cfg, h.state, h.progressPath, num, body.Feedback, h.settings, h.logger)

		if err != nil {
			if ctx.Err() != nil {
				h.logger.WarnKey("log.chapter_revise_cancelled")
			} else {
				h.logger.ErrorKey("log.chapter_revise_failed", err)
			}
			h.logger.TaskEnd("chapter_revision", false)
			return
		}

		h.logger.TaskEnd("chapter_revision", true)
		h.broadcastProgress()
	}()

	h.writeJSON(w, http.StatusAccepted, map[string]string{"status": "started"})
}

// PostChaptersSmoothTransitions 批量优化已确认章节之间的衔接（修补旧项目用）。
// 逐章检查上一章结尾与本章开头的衔接，仅在生硬时最小化重写本章开头片段。
func (h *Handlers) PostChaptersSmoothTransitions(w http.ResponseWriter, r *http.Request) {
	if !h.ensureProject(w, r) {
		return
	}

	pairs := 0
	for i := 1; i < len(h.state.Chapters); i++ {
		if h.state.Chapters[i].Status == StatusAccepted && h.state.Chapters[i].Content != "" &&
			h.state.Chapters[i-1].Status == StatusAccepted && h.state.Chapters[i-1].Content != "" {
			pairs++
		}
	}
	if pairs == 0 {
		h.writeErrorReq(w, r, http.StatusBadRequest, "no_transitions_to_optimize")
		return
	}

	if !h.tryStartTask() {
		h.writeErrorReq(w, r, http.StatusConflict, "task_running_wait")
		return
	}

	go func() {
		defer h.endTask()
		h.logger.TaskStart("smooth_transitions")
		ctx := h.taskCtx

		err := SmoothTransitionsAction(ctx, h.apiCfg, h.cfg, h.state, h.progressPath, h.logger)
		if err != nil {
			if ctx.Err() != nil {
				h.logger.WarnKey("log.smooth_transitions_cancelled")
			} else {
				h.logger.ErrorKey("log.smooth_transitions_failed", err)
			}
			h.logger.TaskEnd("smooth_transitions", false)
			h.broadcastProgress()
			return
		}

		h.logger.TaskEnd("smooth_transitions", true)
		h.broadcastProgress()
	}()

	h.writeJSON(w, http.StatusAccepted, map[string]string{"status": "started"})
}

func (h *Handlers) DeleteChapter(w http.ResponseWriter, r *http.Request) {
	if h.isTaskRunning() {
		h.writeErrorReq(w, r, http.StatusConflict, "delete_chapter_locked")
		return
	}

	if len(h.state.Chapters) == 0 {
		h.writeErrorReq(w, r, http.StatusBadRequest, "no_chapters_to_delete")
		return
	}

	lastIdx := len(h.state.Chapters) - 1
	ch := &h.state.Chapters[lastIdx]

	if ch.Status == StatusWriting {
		h.writeErrorReq(w, r, http.StatusConflict, "writing_chapter_cannot_delete")
		return
	}

	deleteFile(ChapterMarkdownPath(h.projectDir(), ch.Num))
	ch.Content = ""
	ch.Summary = ""
	ch.Status = StatusPending

	if h.state.CurrentChapterIndex > lastIdx {
		h.state.CurrentChapterIndex = lastIdx
	}

	if err := SaveProgress(h.progressPath, h.state); err != nil {
		h.writeErrorReq(w, r, http.StatusInternalServerError, "save_progress_failed", err.Error())
		return
	}

	h.logger.SuccessKey("log.chapter_deleted", ch.Num)
	h.writeJSON(w, http.StatusOK, h.state)
}

func (h *Handlers) DeleteOutline(w http.ResponseWriter, r *http.Request) {
	if h.isTaskRunning() {
		h.writeErrorReq(w, r, http.StatusConflict, "delete_outline_locked")
		return
	}

	for _, ch := range h.state.Chapters {
		if ch.Status == StatusWriting || ch.Status == StatusReview {
			h.writeErrorReq(w, r, http.StatusConflict, "writing_chapter_present_delete")
			return
		}
	}

	h.state.Title = ""
	h.state.CorePrompt = ""
	h.state.StorySynopsis = ""
	h.state.Chapters = nil
	h.state.StoryConfigSnapshot = nil
	h.state.CurrentChapterIndex = 0

	if err := SaveProgress(h.progressPath, h.state); err != nil {
		h.writeErrorReq(w, r, http.StatusInternalServerError, "save_progress_failed", err.Error())
		return
	}

	h.logger.SuccessKey("log.outline_deleted")
	h.writeJSON(w, http.StatusOK, h.state)
}

func (h *Handlers) PutChapterOutline(w http.ResponseWriter, r *http.Request) {
	if h.isTaskRunning() {
		h.writeErrorReq(w, r, http.StatusConflict, "task_running_wait")
		return
	}

	numStr := r.PathValue("num")
	var num int
	if _, err := fmt.Sscanf(numStr, "%d", &num); err != nil {
		h.writeErrorReq(w, r, http.StatusBadRequest, "invalid_chapter_num")
		return
	}

	var body struct {
		Title   string `json:"title"`
		Outline string `json:"outline"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.writeErrorReq(w, r, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}

	if err := EditChapterOutline(h.state, num, body.Title, body.Outline); err != nil {
		h.writeErrorReq(w, r, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}

	if err := SaveProgress(h.progressPath, h.state); err != nil {
		h.writeErrorReq(w, r, http.StatusInternalServerError, "save_progress_failed", err.Error())
		return
	}

	go RunForeshadowOutlineCheckAndSave(context.Background(), h.apiCfg, h.cfg, h.state, h.progressPath, h.logger)

	h.logger.SuccessKey("log.chapter_outline_updated", num)
	h.writeJSON(w, http.StatusOK, h.state)
}

func (h *Handlers) PostSettingsReconcile(w http.ResponseWriter, r *http.Request) {
	if !h.tryStartTask() {
		h.writeErrorReq(w, r, http.StatusConflict, "task_running_wait")
		return
	}

	var body StoryConfig
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.endTask()
		h.writeErrorReq(w, r, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}

	go func() {
		defer h.endTask()
		h.logger.TaskStart("settings_reconciliation")
		ctx := h.taskCtx

		h.logger.InfoKey("log.settings_reconciling")
		err := ReconcileSettingsAction(ctx, h.apiCfg, h.cfg, h.state, body, h.progressPath, h.cfgPath, h.logger)

		if err != nil {
			if ctx.Err() != nil {
				h.logger.WarnKey("log.settings_reconcile_cancelled")
				h.logger.TaskEnd("settings_reconciliation", false)
			} else {
				h.logger.ErrorKey("log.settings_reconcile_failed", err)
				h.logger.TaskEnd("settings_reconciliation", false)
			}
			return
		}

		h.logger.SuccessKey("log.settings_reconcile_done")
		h.logger.TaskEnd("settings_reconciliation", true)
		h.broadcastProgress()
	}()

	h.writeJSON(w, http.StatusAccepted, map[string]string{"status": "started"})
}

func (h *Handlers) DeleteChaptersFrom(w http.ResponseWriter, r *http.Request) {
	if h.isTaskRunning() {
		h.writeErrorReq(w, r, http.StatusConflict, "delete_chapter_locked")
		return
	}

	numStr := r.PathValue("num")
	var num int
	if _, err := fmt.Sscanf(numStr, "%d", &num); err != nil {
		h.writeErrorReq(w, r, http.StatusBadRequest, "invalid_chapter_num")
		return
	}

	startIdx := -1
	for i, ch := range h.state.Chapters {
		if ch.Num == num {
			startIdx = i
			break
		}
	}
	if startIdx == -1 {
		h.writeErrorReq(w, r, http.StatusNotFound, "chapter_n_not_found", num)
		return
	}

	for i := startIdx; i < len(h.state.Chapters); i++ {
		if h.state.Chapters[i].Status == StatusWriting {
			h.writeErrorReq(w, r, http.StatusConflict, "writing_range_has_writing")
			return
		}
	}

	deletedCount := len(h.state.Chapters) - startIdx

	for i := startIdx; i < len(h.state.Chapters); i++ {
		ch := &h.state.Chapters[i]
		deleteFile(ChapterMarkdownPath(h.projectDir(), ch.Num))
		ch.Content = ""
		ch.Summary = ""
		ch.Status = StatusPending
	}

	if h.state.CurrentChapterIndex >= startIdx {
		h.state.CurrentChapterIndex = startIdx
	}

	if err := SaveProgress(h.progressPath, h.state); err != nil {
		h.writeErrorReq(w, r, http.StatusInternalServerError, "save_progress_failed", err.Error())
		return
	}

	h.logger.SuccessKey("log.chapters_deleted_from", num, deletedCount)
	h.writeJSON(w, http.StatusOK, h.state)
}

func (h *Handlers) broadcastProgress() {
	accepted := 0
	for _, ch := range h.state.Chapters {
		if ch.Status == StatusAccepted {
			accepted++
		}
	}
	total := len(h.state.Chapters)
	var pct float64
	if total > 0 {
		pct = float64(accepted) / float64(total) * 100
	}
	h.logger.ProgressUpdate(map[string]interface{}{
		"phase":             h.state.Phase,
		"title":             h.state.Title,
		"current_chapter":   h.state.CurrentChapterIndex,
		"total_chapters":    total,
		"accepted_chapters": accepted,
		"percent":           pct,
		"is_task_running":   h.isTaskRunning(),
	})
}

func (h *Handlers) GetStatus(w http.ResponseWriter, r *http.Request) {
	lang := LangZH
	if h.cfg != nil {
		lang = NormalizeLanguage(h.cfg.Language)
	}
	resp := map[string]interface{}{
		"phase":            h.state.Phase,
		"title":            h.state.Title,
		"total_chapters":   len(h.state.Chapters),
		"is_task_running":  h.isTaskRunning(),
		"auto_confirm":     h.isAutoConfirmOn(),
		"project_language": lang,
	}
	if h.isTaskRunning() && h.taskTokens != nil {
		prompt, completion := h.taskTokens.Snapshot()
		resp["token_usage"] = map[string]int{
			"prompt_tokens":     prompt,
			"completion_tokens": completion,
		}
	}
	h.writeJSON(w, http.StatusOK, resp)
}

func (h *Handlers) GetForeshadows(w http.ResponseWriter, r *http.Request) {
	if h.state.Foreshadows == nil {
		h.writeJSON(w, http.StatusOK, []Foreshadow{})
		return
	}
	h.writeJSON(w, http.StatusOK, h.state.Foreshadows)
}

func (h *Handlers) GetForeshadowsRoadmap(w http.ResponseWriter, r *http.Request) {
	if !h.ensureProject(w, r) {
		return
	}
	markdown := BuildForeshadowRoadmapMarkdown(h.state)
	h.writeJSON(w, http.StatusOK, map[string]string{
		"markdown": markdown,
		"path":     ForeshadowRoadmapPath(h.projectDir()),
	})
}

func (h *Handlers) persistForeshadowRoadmap() {
	if err := SaveForeshadowRoadmap(h.projectDir(), h.state); err != nil {
		h.logger.WarnKey("log.foreshadow_roadmap_save_failed", err)
	}
}

func (h *Handlers) PostForeshadowsSuggest(w http.ResponseWriter, r *http.Request) {
	if !h.tryStartTask() {
		h.writeErrorReq(w, r, http.StatusConflict, "task_running_wait")
		return
	}

	if len(h.state.Chapters) == 0 {
		h.endTask()
		h.writeErrorReq(w, r, http.StatusBadRequest, "need_generate_outline_first")
		return
	}

	go func() {
		defer h.endTask()
		h.logger.TaskStart("foreshadow_suggest")
		ctx := h.taskCtx

		h.logger.InfoKey("log.foreshadow_suggesting")
		suggestions, err := SuggestForeshadows(ctx, h.apiCfg, h.cfg, h.state, h.logger)

		if err != nil {
			if ctx.Err() != nil {
				h.logger.WarnKey("log.foreshadow_suggest_cancelled")
				h.logger.TaskEnd("foreshadow_suggest", false)
			} else {
				h.logger.ErrorKey("log.foreshadow_suggest_failed", err)
				h.logger.TaskEnd("foreshadow_suggest", false)
			}
			return
		}

		h.logger.SuccessKey("log.foreshadow_suggest_done", len(suggestions))
		h.logger.TaskEnd("foreshadow_suggest", true)
		h.logger.ForeshadowSuggestions(suggestions)
	}()

	h.writeJSON(w, http.StatusAccepted, map[string]string{"status": "started"})
}

func (h *Handlers) PostForeshadow(w http.ResponseWriter, r *http.Request) {
	if h.rejectIfTaskRunning(w, r) {
		return
	}
	var req struct {
		Name          string `json:"name"`
		Description   string `json:"description"`
		PlantChapter  int    `json:"plant_chapter"`
		TargetChapter int    `json:"target_chapter"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeErrorReq(w, r, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	if req.Name == "" {
		h.writeErrorReq(w, r, http.StatusBadRequest, "foreshadow_name_required")
		return
	}
	if req.Description == "" {
		h.writeErrorReq(w, r, http.StatusBadRequest, "foreshadow_desc_required")
		return
	}

	fs := Foreshadow{
		ID:            NextForeshadowID(h.state.Foreshadows),
		Name:          req.Name,
		Description:   req.Description,
		PlantChapter:  req.PlantChapter,
		TargetChapter: req.TargetChapter,
		Status:        ForeshadowPlanted,
		Events:        []ForeshadowEvent{},
	}

	h.state.Foreshadows = append(h.state.Foreshadows, fs)

	if err := SaveProgress(h.progressPath, h.state); err != nil {
		h.writeErrorReq(w, r, http.StatusInternalServerError, "save_failed", err.Error())
		return
	}

	h.persistForeshadowRoadmap()
	h.writeJSON(w, http.StatusOK, fs)
}

func (h *Handlers) PutForeshadow(w http.ResponseWriter, r *http.Request) {
	if h.rejectIfTaskRunning(w, r) {
		return
	}
	idStr := r.PathValue("id")
	var id int
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		h.writeErrorReq(w, r, http.StatusBadRequest, "invalid_foreshadow_id")
		return
	}

	var req struct {
		Name          string           `json:"name"`
		Description   string           `json:"description"`
		PlantChapter  int              `json:"plant_chapter"`
		TargetChapter int              `json:"target_chapter"`
		Status        ForeshadowStatus `json:"status"`
		Resolution    string           `json:"resolution"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeErrorReq(w, r, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}

	idx := -1
	for i, fs := range h.state.Foreshadows {
		if fs.ID == id {
			idx = i
			break
		}
	}
	if idx == -1 {
		h.writeErrorReq(w, r, http.StatusNotFound, "foreshadow_not_found")
		return
	}

	fs := &h.state.Foreshadows[idx]
	if req.Name != "" {
		fs.Name = req.Name
	}
	if req.Description != "" {
		fs.Description = req.Description
	}
	if req.PlantChapter > 0 {
		fs.PlantChapter = req.PlantChapter
	}
	if req.TargetChapter > 0 {
		fs.TargetChapter = req.TargetChapter
	}
	if req.Status != "" {
		fs.Status = req.Status
	}
	if req.Resolution != "" {
		fs.Resolution = req.Resolution
	}

	if err := SaveProgress(h.progressPath, h.state); err != nil {
		h.writeErrorReq(w, r, http.StatusInternalServerError, "save_failed", err.Error())
		return
	}

	h.persistForeshadowRoadmap()
	h.writeJSON(w, http.StatusOK, fs)
}

func (h *Handlers) DeleteForeshadow(w http.ResponseWriter, r *http.Request) {
	if h.rejectIfTaskRunning(w, r) {
		return
	}
	idStr := r.PathValue("id")
	var id int
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		h.writeErrorReq(w, r, http.StatusBadRequest, "invalid_foreshadow_id")
		return
	}

	idx := -1
	for i, fs := range h.state.Foreshadows {
		if fs.ID == id {
			idx = i
			break
		}
	}
	if idx == -1 {
		h.writeErrorReq(w, r, http.StatusNotFound, "foreshadow_not_found")
		return
	}

	h.state.Foreshadows = append(h.state.Foreshadows[:idx], h.state.Foreshadows[idx+1:]...)

	if err := SaveProgress(h.progressPath, h.state); err != nil {
		h.writeErrorReq(w, r, http.StatusInternalServerError, "save_failed", err.Error())
		return
	}

	h.persistForeshadowRoadmap()
	h.writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *Handlers) PostForeshadowsConfirm(w http.ResponseWriter, r *http.Request) {
	if h.rejectIfTaskRunning(w, r) {
		return
	}
	var req struct {
		Foreshadows []Foreshadow `json:"foreshadows"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeErrorReq(w, r, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}

	for i := range req.Foreshadows {
		req.Foreshadows[i].ID = NextForeshadowID(h.state.Foreshadows) + i
		req.Foreshadows[i].Status = ForeshadowPlanted
		if req.Foreshadows[i].Events == nil {
			req.Foreshadows[i].Events = []ForeshadowEvent{}
		}
	}

	h.state.Foreshadows = append(h.state.Foreshadows, req.Foreshadows...)

	if err := SaveProgress(h.progressPath, h.state); err != nil {
		h.writeErrorReq(w, r, http.StatusInternalServerError, "save_failed", err.Error())
		return
	}

	h.persistForeshadowRoadmap()
	h.broadcastProgress()
	go RunForeshadowOutlineCheckAndSave(context.Background(), h.apiCfg, h.cfg, h.state, h.progressPath, h.logger)
	h.writeJSON(w, http.StatusOK, h.state.Foreshadows)
}

func (h *Handlers) PostContinueImport(w http.ResponseWriter, r *http.Request) {
	if !h.tryStartTask() {
		h.writeErrorReq(w, r, http.StatusConflict, "task_running_wait")
		return
	}

	var body struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Content == "" {
		h.endTask()
		h.writeErrorReq(w, r, http.StatusBadRequest, "missing_content")
		return
	}

	go func() {
		defer h.endTask()
		h.logger.TaskStart("continue_analysis")
		ctx := h.taskCtx

		h.logger.InfoKey("log.continue_analyzing")
		analysis, err := AnalyzeExistingContent(ctx, h.apiCfg, h.cfg, body.Content)

		if err != nil {
			if ctx.Err() != nil {
				h.logger.WarnKey("log.continue_analyze_cancelled")
				h.logger.TaskEnd("continue_analysis", false)
			} else {
				h.logger.ErrorKey("log.continue_analyze_failed", err)
				h.logger.TaskEnd("continue_analysis", false)
			}
			return
		}

		h.pendingContinueContent = body.Content

		h.logger.SuccessKey("log.continue_analyze_done", len(analysis.Chapters))
		h.logger.TaskEnd("continue_analysis", true)
		h.logger.ContinueAnalysisResult(analysis)
	}()

	h.writeJSON(w, http.StatusAccepted, map[string]string{"status": "started"})
}

func (h *Handlers) PostContinueConfirm(w http.ResponseWriter, r *http.Request) {
	if h.isTaskRunning() {
		h.writeErrorReq(w, r, http.StatusConflict, "task_running_wait")
		return
	}

	if h.state.Phase != "outline" {
		h.writeErrorReq(w, r, http.StatusBadRequest, "continue_reset_first")
		return
	}

	if h.pendingContinueContent == "" {
		h.writeErrorReq(w, r, http.StatusBadRequest, "continue_analyze_first")
		return
	}

	var analysis ContinueAnalysis
	if err := json.NewDecoder(r.Body).Decode(&analysis); err != nil {
		h.writeErrorReq(w, r, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}

	if len(analysis.Chapters) == 0 {
		h.writeErrorReq(w, r, http.StatusBadRequest, "analysis_no_chapters")
		return
	}

	content := h.pendingContinueContent
	h.pendingContinueContent = ""

	if err := ImportContinueAction(h.cfg, h.state, &analysis, content, h.progressPath, h.cfgPath); err != nil {
		h.writeErrorReq(w, r, http.StatusInternalServerError, "continue_import_failed", err.Error())
		return
	}

	h.logger.SuccessKey("log.continue_import_done")
	h.writeJSON(w, http.StatusOK, h.state)
}

func (h *Handlers) PostOutlineGenerateContinuation(w http.ResponseWriter, r *http.Request) {
	if !h.tryStartTask() {
		h.writeErrorReq(w, r, http.StatusConflict, "task_running_wait")
		return
	}

	if h.state.Phase != "outline" {
		h.endTask()
		h.writeErrorReq(w, r, http.StatusBadRequest, "phase_not_outline")
		return
	}

	var body struct {
		ChapterCount int `json:"chapter_count"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ChapterCount <= 0 {
		body.ChapterCount = 5
	}

	go func() {
		defer h.endTask()
		h.logger.TaskStart("continuation_outline")
		ctx := h.taskCtx

		h.logger.InfoKey("log.continuation_outline_generating")
		err := GenerateContinuationOutline(ctx, h.apiCfg, h.cfg, h.state, body.ChapterCount, h.progressPath, h.logger)

		if err != nil {
			if ctx.Err() != nil {
				h.logger.WarnKey("log.continuation_outline_cancelled")
				h.logger.TaskEnd("continuation_outline", false)
			} else {
				h.logger.ErrorKey("log.continuation_outline_failed", err)
				h.logger.TaskEnd("continuation_outline", false)
			}
			return
		}

		h.logger.SuccessKey("log.continuation_outline_done")
		h.logger.TaskEnd("continuation_outline", true)
		h.broadcastProgress()
	}()

	h.writeJSON(w, http.StatusAccepted, map[string]string{"status": "started"})
}

func (h *Handlers) SSEHandler(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	ch := h.logger.Subscribe()
	defer h.logger.Unsubscribe(ch)

	ctx := r.Context()

	for {
		select {
		case msg, ok := <-ch:
			if !ok {
				return
			}
			_, err := w.Write(formatSSE(msg))
			if err != nil {
				return
			}
			flusher.Flush()
		case <-ctx.Done():
			return
		}
	}
}

func (h *Handlers) GetSettings(w http.ResponseWriter, r *http.Request) {
	h.writeJSON(w, http.StatusOK, h.settings)
}

func (h *Handlers) PostCharacter(w http.ResponseWriter, r *http.Request) {
	if h.rejectIfTaskRunning(w, r) {
		return
	}
	var c Character
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		h.writeErrorReq(w, r, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	if c.Name == "" {
		h.writeErrorReq(w, r, http.StatusBadRequest, "character_name_empty")
		return
	}

	c.ID = h.settings.nextCharacterID()
	h.settings.Characters = append(h.settings.Characters, c)

	if err := SaveProjectSettings(h.settingsPath, h.settings); err != nil {
		h.writeErrorReq(w, r, http.StatusInternalServerError, "save_failed", err.Error())
		return
	}

	h.writeJSON(w, http.StatusOK, c)
}

func (h *Handlers) PutCharacter(w http.ResponseWriter, r *http.Request) {
	if h.rejectIfTaskRunning(w, r) {
		return
	}
	id := r.PathValue("id")

	var req Character
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeErrorReq(w, r, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}

	for i, c := range h.settings.Characters {
		if c.ID == id {
			if req.Name != "" {
				h.settings.Characters[i].Name = req.Name
			}
			if req.Age != "" {
				h.settings.Characters[i].Age = req.Age
			}
			if req.Appearance != "" {
				h.settings.Characters[i].Appearance = req.Appearance
			}
			if req.Personality != "" {
				h.settings.Characters[i].Personality = req.Personality
			}
			if req.Background != "" {
				h.settings.Characters[i].Background = req.Background
			}
			if req.Motivation != "" {
				h.settings.Characters[i].Motivation = req.Motivation
			}
			if req.Abilities != "" {
				h.settings.Characters[i].Abilities = req.Abilities
			}
			if req.Notes != "" {
				h.settings.Characters[i].Notes = req.Notes
			}

			if err := SaveProjectSettings(h.settingsPath, h.settings); err != nil {
				h.writeErrorReq(w, r, http.StatusInternalServerError, "save_failed", err.Error())
				return
			}

			h.writeJSON(w, http.StatusOK, h.settings.Characters[i])
			return
		}
	}

	h.writeErrorReq(w, r, http.StatusNotFound, "character_not_found")
}

func (h *Handlers) DeleteCharacter(w http.ResponseWriter, r *http.Request) {
	if h.rejectIfTaskRunning(w, r) {
		return
	}
	id := r.PathValue("id")

	for i, c := range h.settings.Characters {
		if c.ID == id {
			h.settings.Characters = append(h.settings.Characters[:i], h.settings.Characters[i+1:]...)
			if err := SaveProjectSettings(h.settingsPath, h.settings); err != nil {
				h.writeErrorReq(w, r, http.StatusInternalServerError, "save_failed", err.Error())
				return
			}
			h.writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
			return
		}
	}

	h.writeErrorReq(w, r, http.StatusNotFound, "character_not_found")
}

func (h *Handlers) PostWorldview(w http.ResponseWriter, r *http.Request) {
	if h.rejectIfTaskRunning(w, r) {
		return
	}
	var wv WorldviewEntry
	if err := json.NewDecoder(r.Body).Decode(&wv); err != nil {
		h.writeErrorReq(w, r, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	if wv.Name == "" || wv.Description == "" {
		h.writeErrorReq(w, r, http.StatusBadRequest, "worldview_field_empty")
		return
	}

	wv.ID = h.settings.nextWorldviewID()
	h.settings.Worldview = append(h.settings.Worldview, wv)

	if err := SaveProjectSettings(h.settingsPath, h.settings); err != nil {
		h.writeErrorReq(w, r, http.StatusInternalServerError, "save_failed", err.Error())
		return
	}

	h.writeJSON(w, http.StatusOK, wv)
}

func (h *Handlers) PutWorldview(w http.ResponseWriter, r *http.Request) {
	if h.rejectIfTaskRunning(w, r) {
		return
	}
	id := r.PathValue("id")

	var req WorldviewEntry
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeErrorReq(w, r, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}

	for i, wv := range h.settings.Worldview {
		if wv.ID == id {
			if req.Name != "" {
				h.settings.Worldview[i].Name = req.Name
			}
			if req.Category != "" {
				h.settings.Worldview[i].Category = req.Category
			}
			if req.Description != "" {
				h.settings.Worldview[i].Description = req.Description
			}
			if req.Tags != "" {
				h.settings.Worldview[i].Tags = req.Tags
			}

			if err := SaveProjectSettings(h.settingsPath, h.settings); err != nil {
				h.writeErrorReq(w, r, http.StatusInternalServerError, "save_failed", err.Error())
				return
			}

			h.writeJSON(w, http.StatusOK, h.settings.Worldview[i])
			return
		}
	}

	h.writeErrorReq(w, r, http.StatusNotFound, "worldview_not_found")
}

func (h *Handlers) DeleteWorldview(w http.ResponseWriter, r *http.Request) {
	if h.rejectIfTaskRunning(w, r) {
		return
	}
	id := r.PathValue("id")

	for i, wv := range h.settings.Worldview {
		if wv.ID == id {
			h.settings.Worldview = append(h.settings.Worldview[:i], h.settings.Worldview[i+1:]...)
			if err := SaveProjectSettings(h.settingsPath, h.settings); err != nil {
				h.writeErrorReq(w, r, http.StatusInternalServerError, "save_failed", err.Error())
				return
			}
			h.writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
			return
		}
	}

	h.writeErrorReq(w, r, http.StatusNotFound, "worldview_not_found")
}

func (h *Handlers) PostOrganization(w http.ResponseWriter, r *http.Request) {
	if h.rejectIfTaskRunning(w, r) {
		return
	}
	var o Organization
	if err := json.NewDecoder(r.Body).Decode(&o); err != nil {
		h.writeErrorReq(w, r, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	if o.Name == "" {
		h.writeErrorReq(w, r, http.StatusBadRequest, "organization_name_empty")
		return
	}

	o.ID = h.settings.nextOrganizationID()
	h.settings.Organizations = append(h.settings.Organizations, o)

	if err := SaveProjectSettings(h.settingsPath, h.settings); err != nil {
		h.writeErrorReq(w, r, http.StatusInternalServerError, "save_failed", err.Error())
		return
	}

	h.writeJSON(w, http.StatusOK, o)
}

func (h *Handlers) PutOrganization(w http.ResponseWriter, r *http.Request) {
	if h.rejectIfTaskRunning(w, r) {
		return
	}
	id := r.PathValue("id")

	var req Organization
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeErrorReq(w, r, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}

	for i, o := range h.settings.Organizations {
		if o.ID == id {
			if req.Name != "" {
				h.settings.Organizations[i].Name = req.Name
			}
			if req.Type != "" {
				h.settings.Organizations[i].Type = req.Type
			}
			if req.Description != "" {
				h.settings.Organizations[i].Description = req.Description
			}
			if req.Members != nil {
				h.settings.Organizations[i].Members = req.Members
			}

			if err := SaveProjectSettings(h.settingsPath, h.settings); err != nil {
				h.writeErrorReq(w, r, http.StatusInternalServerError, "save_failed", err.Error())
				return
			}

			h.writeJSON(w, http.StatusOK, h.settings.Organizations[i])
			return
		}
	}

	h.writeErrorReq(w, r, http.StatusNotFound, "organization_not_found")
}

func (h *Handlers) DeleteOrganization(w http.ResponseWriter, r *http.Request) {
	if h.rejectIfTaskRunning(w, r) {
		return
	}
	id := r.PathValue("id")

	for i, o := range h.settings.Organizations {
		if o.ID == id {
			h.settings.Organizations = append(h.settings.Organizations[:i], h.settings.Organizations[i+1:]...)
			if err := SaveProjectSettings(h.settingsPath, h.settings); err != nil {
				h.writeErrorReq(w, r, http.StatusInternalServerError, "save_failed", err.Error())
				return
			}
			h.writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
			return
		}
	}

	h.writeErrorReq(w, r, http.StatusNotFound, "organization_not_found")
}

func (h *Handlers) PostRelation(w http.ResponseWriter, r *http.Request) {
	if h.rejectIfTaskRunning(w, r) {
		return
	}
	var rel Relation
	if err := json.NewDecoder(r.Body).Decode(&rel); err != nil {
		h.writeErrorReq(w, r, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	if rel.SourceID == "" || rel.TargetID == "" {
		h.writeErrorReq(w, r, http.StatusBadRequest, "relation_endpoints_empty")
		return
	}

	rel.ID = h.settings.nextRelationID()
	h.settings.Relations = append(h.settings.Relations, rel)

	if err := SaveProjectSettings(h.settingsPath, h.settings); err != nil {
		h.writeErrorReq(w, r, http.StatusInternalServerError, "save_failed", err.Error())
		return
	}

	h.writeJSON(w, http.StatusOK, rel)
}

func (h *Handlers) PutRelation(w http.ResponseWriter, r *http.Request) {
	if h.rejectIfTaskRunning(w, r) {
		return
	}
	id := r.PathValue("id")

	var req Relation
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeErrorReq(w, r, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}

	for i, rel := range h.settings.Relations {
		if rel.ID == id {
			if req.SourceID != "" {
				h.settings.Relations[i].SourceID = req.SourceID
			}
			if req.SourceType != "" {
				h.settings.Relations[i].SourceType = req.SourceType
			}
			if req.TargetID != "" {
				h.settings.Relations[i].TargetID = req.TargetID
			}
			if req.TargetType != "" {
				h.settings.Relations[i].TargetType = req.TargetType
			}
			if req.Label != "" {
				h.settings.Relations[i].Label = req.Label
			}

			if err := SaveProjectSettings(h.settingsPath, h.settings); err != nil {
				h.writeErrorReq(w, r, http.StatusInternalServerError, "save_failed", err.Error())
				return
			}

			h.writeJSON(w, http.StatusOK, h.settings.Relations[i])
			return
		}
	}

	h.writeErrorReq(w, r, http.StatusNotFound, "relation_not_found")
}

func (h *Handlers) DeleteRelation(w http.ResponseWriter, r *http.Request) {
	if h.rejectIfTaskRunning(w, r) {
		return
	}
	id := r.PathValue("id")

	for i, rel := range h.settings.Relations {
		if rel.ID == id {
			h.settings.Relations = append(h.settings.Relations[:i], h.settings.Relations[i+1:]...)
			if err := SaveProjectSettings(h.settingsPath, h.settings); err != nil {
				h.writeErrorReq(w, r, http.StatusInternalServerError, "save_failed", err.Error())
				return
			}
			h.writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
			return
		}
	}

	h.writeErrorReq(w, r, http.StatusNotFound, "relation_not_found")
}

func (h *Handlers) PostSettingsAIGenerate(w http.ResponseWriter, r *http.Request) {
	h.writeErrorReq(w, r, http.StatusGone, "settings_ai_generate_moved")
}

func (h *Handlers) PostSettingsPolish(w http.ResponseWriter, r *http.Request) {
	h.writeErrorReq(w, r, http.StatusGone, "settings_polish_moved")
}

func (h *Handlers) PostChapterPolish(w http.ResponseWriter, r *http.Request) {
	if !h.ensureProject(w, r) {
		return
	}

	polishSkills := GetEnabledSkillsByCategory(h.skills, h.cfg.SkillConfig, "polish")
	if len(polishSkills) == 0 {
		h.writeErrorReq(w, r, http.StatusBadRequest, "need_polish_skill")
		return
	}

	var body struct {
		Num int `json:"num"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	chapterIdx := -1
	if body.Num > 0 {
		for i, ch := range h.state.Chapters {
			if ch.Num == body.Num {
				chapterIdx = i
				break
			}
		}
		if chapterIdx == -1 {
			h.writeErrorReq(w, r, http.StatusBadRequest, "chapter_not_found")
			return
		}
	} else {
		chapterIdx = h.state.CurrentChapterIndex
		if chapterIdx < 0 || chapterIdx >= len(h.state.Chapters) {
			h.writeErrorReq(w, r, http.StatusBadRequest, "chapter_num_required")
			return
		}
	}

	ch := h.state.Chapters[chapterIdx]
	if ch.Content == "" {
		h.writeErrorReq(w, r, http.StatusBadRequest, "chapter_content_empty")
		return
	}
	if ch.Status == StatusWriting {
		h.writeErrorReq(w, r, http.StatusBadRequest, "chapter_in_writing")
		return
	}

	if !h.tryStartTask() {
		h.writeErrorReq(w, r, http.StatusConflict, "task_running_wait")
		return
	}

	prevStatus := ch.Status
	idx := chapterIdx

	go func() {
		defer h.endTask()
		h.logger.TaskStart("chapter_polish")
		ctx := h.taskCtx

		err := PolishChapterAction(ctx, h.apiCfg, h.cfg, h.state, idx, polishSkills, h.progressPath, h.logger)
		if err != nil {
			if ctx.Err() != nil {
				h.logger.WarnKey("log.chapter_polish_cancelled")
			} else {
				h.logger.ErrorKey("log.chapter_polish_failed", err)
			}
			h.logger.TaskEnd("chapter_polish", false)
			return
		}

		if prevStatus == StatusAccepted {
			h.state.Chapters[idx].Status = StatusAccepted
			_ = SaveProgress(h.progressPath, h.state)
		}

		h.logger.TaskEnd("chapter_polish", true)
		h.broadcastProgress()
	}()

	h.writeJSON(w, http.StatusAccepted, map[string]string{"status": "started"})
}

// GetPostProcess 获取全书优化状态。
func (h *Handlers) GetPostProcess(w http.ResponseWriter, r *http.Request) {
	if !h.ensureProject(w, r) {
		return
	}
	h.writeJSON(w, http.StatusOK, h.postProcessResponse())
}

func (h *Handlers) postProcessResponse() map[string]interface{} {
	return map[string]interface{}{
		"book_complete": isBookFullyAccepted(h.state),
		"state":         h.postprocess,
	}
}

// PutPostProcessRoadmap 更新优化工单（勾选、编辑意见等）。
func (h *Handlers) PutPostProcessRoadmap(w http.ResponseWriter, r *http.Request) {
	if !h.ensureProject(w, r) {
		return
	}
	if h.rejectIfTaskRunning(w, r) {
		return
	}

	var req struct {
		Roadmap        []RoadmapItem              `json:"roadmap"`
		ExecuteOptions *PostProcessExecuteOptions `json:"execute_options"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeErrorReq(w, r, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	if req.Roadmap != nil {
		h.postprocess.Roadmap = req.Roadmap
	}
	if req.ExecuteOptions != nil {
		h.postprocess.ExecuteOptions = req.ExecuteOptions
	}
	if err := SavePostProcess(h.postprocessPath, h.postprocess); err != nil {
		h.writeErrorReq(w, r, http.StatusInternalServerError, "save_failed", err.Error())
		return
	}
	h.logger.PostProcessUpdate(h.postprocess)
	h.writeJSON(w, http.StatusOK, h.postProcessResponse())
}

// DeletePostProcess 清空全书优化报告与工单。
func (h *Handlers) DeletePostProcess(w http.ResponseWriter, r *http.Request) {
	if !h.ensureProject(w, r) {
		return
	}
	if h.rejectIfTaskRunning(w, r) {
		return
	}

	h.postprocess = &PostProcessState{
		ExecuteOptions: &PostProcessExecuteOptions{RunSmoothTransitionsFirst: true},
	}
	if err := SavePostProcess(h.postprocessPath, h.postprocess); err != nil {
		h.writeErrorReq(w, r, http.StatusInternalServerError, "clear_postprocess_failed", err.Error())
		return
	}
	h.logger.PostProcessUpdate(h.postprocess)
	h.writeJSON(w, http.StatusOK, h.postProcessResponse())
}

// PostPostProcessDiagnose 异步：全书诊断 + 一致性核查 + 生成路线图。
func (h *Handlers) PostPostProcessDiagnose(w http.ResponseWriter, r *http.Request) {
	if !h.ensureProject(w, r) {
		return
	}
	if !isBookFullyAccepted(h.state) {
		h.writeErrorReq(w, r, http.StatusBadRequest, "book_not_complete")
		return
	}
	if !h.tryStartTask() {
		h.writeErrorReq(w, r, http.StatusConflict, "task_running_wait")
		return
	}

	go func() {
		defer h.endTask()
		h.logger.TaskStart("postprocess_diagnose")
		ctx := h.taskCtx

		err := FullPostProcessAnalyzeAction(ctx, h.apiCfg, h.cfg, h.settings, h.state, h.postprocess, h.postprocessPath, h.logger)
		if err != nil {
			if ctx.Err() != nil {
				h.logger.WarnKey("log.postprocess_diagnose_cancelled")
			} else {
				h.logger.ErrorKey("log.postprocess_diagnose_failed", err)
			}
			h.logger.TaskEnd("postprocess_diagnose", false)
			return
		}

		h.logger.PostProcessUpdate(h.postprocess)
		h.logger.TaskEnd("postprocess_diagnose", true)
	}()

	h.writeJSON(w, http.StatusAccepted, map[string]string{"status": "started"})
}

// PostPostProcessConsistency 异步：仅重新运行全书一致性核查。
func (h *Handlers) PostPostProcessConsistency(w http.ResponseWriter, r *http.Request) {
	if !h.ensureProject(w, r) {
		return
	}
	if !isBookFullyAccepted(h.state) {
		h.writeErrorReq(w, r, http.StatusBadRequest, "book_not_complete")
		return
	}
	if !h.tryStartTask() {
		h.writeErrorReq(w, r, http.StatusConflict, "task_running_wait")
		return
	}

	go func() {
		defer h.endTask()
		h.logger.TaskStart("postprocess_consistency")
		ctx := h.taskCtx

		report, err := ConsistencyCheckBookAction(ctx, h.apiCfg, h.cfg, h.settings, h.state, h.logger)
		if err != nil {
			if ctx.Err() != nil {
				h.logger.WarnKey("log.postprocess_consistency_cancelled")
			} else {
				h.logger.ErrorKey("log.postprocess_consistency_failed", err)
			}
			h.logger.TaskEnd("postprocess_consistency", false)
			return
		}

		h.postprocess.ConsistencyReport = report
		h.postprocess.ConsistencyAt = time.Now().Format(time.RFC3339)
		_ = SavePostProcess(h.postprocessPath, h.postprocess)
		h.logger.PostProcessReport("consistency", report)
		h.logger.PostProcessUpdate(h.postprocess)
		h.logger.TaskEnd("postprocess_consistency", true)
	}()

	h.writeJSON(w, http.StatusAccepted, map[string]string{"status": "started"})
}

// PostPostProcessRoadmap 异步：根据已有报告重新生成路线图。
func (h *Handlers) PostPostProcessRoadmap(w http.ResponseWriter, r *http.Request) {
	if !h.ensureProject(w, r) {
		return
	}
	if strings.TrimSpace(h.postprocess.DiagnosisReport) == "" && strings.TrimSpace(h.postprocess.ConsistencyReport) == "" {
		h.writeErrorReq(w, r, http.StatusBadRequest, "missing_diagnosis_or_consistency")
		return
	}
	if !h.tryStartTask() {
		h.writeErrorReq(w, r, http.StatusConflict, "task_running_wait")
		return
	}

	go func() {
		defer h.endTask()
		h.logger.TaskStart("postprocess_roadmap")
		ctx := h.taskCtx

		roadmap, err := BuildRoadmapAction(ctx, h.apiCfg, h.cfg, h.postprocess.DiagnosisReport, h.postprocess.ConsistencyReport, h.logger)
		if err != nil {
			if ctx.Err() != nil {
				h.logger.WarnKey("log.postprocess_roadmap_cancelled")
			} else {
				h.logger.ErrorKey("log.postprocess_roadmap_failed", err)
			}
			h.logger.TaskEnd("postprocess_roadmap", false)
			return
		}

		h.postprocess.Roadmap = roadmap
		h.postprocess.RoadmapAt = time.Now().Format(time.RFC3339)
		_ = SavePostProcess(h.postprocessPath, h.postprocess)
		h.logger.PostProcessRoadmap(h.postprocess)
		h.logger.PostProcessUpdate(h.postprocess)
		h.logger.TaskEnd("postprocess_roadmap", true)
	}()

	h.writeJSON(w, http.StatusAccepted, map[string]string{"status": "started"})
}

// PostPostProcessExecute 异步：执行已勾选的优化工单。
func (h *Handlers) PostPostProcessExecute(w http.ResponseWriter, r *http.Request) {
	if !h.ensureProject(w, r) {
		return
	}
	if !isBookFullyAccepted(h.state) {
		h.writeErrorReq(w, r, http.StatusBadRequest, "book_not_complete")
		return
	}
	if len(h.postprocess.Roadmap) == 0 {
		h.writeErrorReq(w, r, http.StatusBadRequest, "no_roadmap_items")
		return
	}

	var body struct {
		ExecuteOptions *PostProcessExecuteOptions `json:"execute_options"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if body.ExecuteOptions != nil {
		h.postprocess.ExecuteOptions = body.ExecuteOptions
	}

	selected := 0
	for i := range h.postprocess.Roadmap {
		if h.postprocess.Roadmap[i].Selected && h.postprocess.Roadmap[i].Status == RoadmapStatusPending {
			selected++
		}
	}
	if selected == 0 {
		h.writeErrorReq(w, r, http.StatusBadRequest, "select_at_least_one_item")
		return
	}

	if !h.tryStartTask() {
		h.writeErrorReq(w, r, http.StatusConflict, "task_running_wait")
		return
	}

	go func() {
		defer h.endTask()
		h.logger.TaskStart("postprocess_execute")
		ctx := h.taskCtx

		err := ExecuteRoadmapAction(ctx, h.apiCfg, h.cfg, h.settings, h.state, h.postprocess, h.progressPath, h.postprocessPath, h.skills, h.logger)
		if err != nil {
			if ctx.Err() != nil {
				h.logger.WarnKey("log.postprocess_execute_cancelled")
			} else {
				h.logger.ErrorKey("log.postprocess_execute_failed", err)
			}
			h.logger.TaskEnd("postprocess_execute", false)
			h.broadcastProgress()
			h.logger.PostProcessUpdate(h.postprocess)
			return
		}

		h.logger.TaskEnd("postprocess_execute", true)
		h.broadcastProgress()
		h.logger.PostProcessUpdate(h.postprocess)
	}()

	h.writeJSON(w, http.StatusAccepted, map[string]string{"status": "started"})
}

func (h *Handlers) GetSkills(w http.ResponseWriter, r *http.Request) {
	type SkillView struct {
		Skill   Skill `json:"skill"`
		Enabled bool  `json:"enabled"`
	}

	var views []SkillView
	for _, s := range h.skills {
		enabled := false
		if h.cfg.SkillConfig != nil && h.cfg.SkillConfig.EnabledSkills != nil {
			enabled = h.cfg.SkillConfig.EnabledSkills[s.ID]
		}
		views = append(views, SkillView{Skill: s, Enabled: enabled})
	}

	h.writeJSON(w, http.StatusOK, views)
}

func (h *Handlers) PutSkillToggle(w http.ResponseWriter, r *http.Request) {
	if h.rejectIfTaskRunning(w, r) {
		return
	}
	id := r.PathValue("id")

	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeErrorReq(w, r, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}

	found := false
	for _, s := range h.skills {
		if s.ID == id {
			found = true
			break
		}
	}
	if !found {
		h.writeErrorReq(w, r, http.StatusNotFound, "skill_not_found")
		return
	}

	if h.cfg.SkillConfig == nil {
		h.cfg.SkillConfig = &SkillConfig{EnabledSkills: make(map[string]bool)}
	}
	if h.cfg.SkillConfig.EnabledSkills == nil {
		h.cfg.SkillConfig.EnabledSkills = make(map[string]bool)
	}

	h.cfg.SkillConfig.EnabledSkills[id] = req.Enabled

	if err := saveConfig(h.cfgPath, h.cfg); err != nil {
		h.writeErrorReq(w, r, http.StatusInternalServerError, "save_config_failed", err.Error())
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]interface{}{"id": id, "enabled": req.Enabled})
}

func (h *Handlers) GetChatSessions(w http.ResponseWriter, r *http.Request) {
	idx, err := LoadChatSessions(h.sessionsDir)
	if err != nil {
		h.writeErrorReq(w, r, http.StatusInternalServerError, "load_session_list_failed", err.Error())
		return
	}
	if idx == nil {
		idx = &ChatSessionIndex{}
	}

	// 清理空会话：删除 msg_count == 0 且不在当前会话中的条目
	var cleaned []ChatSessionMeta
	for _, m := range idx.Sessions {
		if m.MsgCount == 0 {
			path := filepath.Join(chatSessionsDir(h.sessionsDir), m.ID+".json")
			deleteFile(path)
			continue
		}
		cleaned = append(cleaned, m)
	}
	idx.Sessions = cleaned

	h.writeJSON(w, http.StatusOK, idx)
}

func (h *Handlers) PostChatSession(w http.ResponseWriter, r *http.Request) {
	now := time.Now().Format(time.RFC3339)
	session := &ChatSession{
		ID:        generateSessionID(),
		Title:     "新会话",
		Messages:  []ChatMessage{},
		CreatedAt: now,
		UpdatedAt: now,
	}

	// 保存会话文件但不加入索引，等首次发消息时 SaveChatSession 才入索引，
	// 避免产生 0 条记录的空会话。
	dir := chatSessionsDir(h.sessionsDir)
	os.MkdirAll(dir, 0755)
	path := filepath.Join(dir, session.ID+".json")
	data, _ := json.MarshalIndent(session, "", "  ")
	writeFileAtomic(path, data)

	h.writeJSON(w, http.StatusOK, session)
}

func (h *Handlers) GetChatSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	session, err := LoadChatSession(h.sessionsDir, id)
	if err != nil {
		h.writeErrorReq(w, r, http.StatusNotFound, "chat_session_not_found")
		return
	}

	h.writeJSON(w, http.StatusOK, session)
}

func (h *Handlers) DeleteChatSession(w http.ResponseWriter, r *http.Request) {
	if h.rejectIfTaskRunning(w, r) {
		return
	}
	id := r.PathValue("id")

	if err := DeleteChatSession(h.sessionsDir, id); err != nil {
		h.writeErrorReq(w, r, http.StatusInternalServerError, "delete_session_failed", err.Error())
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *Handlers) PostChatMessage(w http.ResponseWriter, r *http.Request) {
	if !h.tryStartTask() {
		h.writeErrorReq(w, r, http.StatusConflict, "task_running_wait")
		return
	}

	sessionID := r.PathValue("id")

	var req struct {
		Content     string `json:"content"`
		ContextPage string `json:"context_page"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Content == "" {
		h.endTask()
		h.writeErrorReq(w, r, http.StatusBadRequest, "missing_content")
		return
	}

	session, err := LoadChatSession(h.sessionsDir, sessionID)
	if err != nil {
		h.endTask()
		h.writeErrorReq(w, r, http.StatusNotFound, "chat_session_not_found")
		return
	}

	now := time.Now().Format(time.RFC3339)
	session.Messages = append(session.Messages, ChatMessage{
		Role:      "user",
		Content:   req.Content,
		Timestamp: now,
	})

	if len(session.Messages) == 1 {
		session.Title = generateChatTitle(req.Content)
	}

	if err := SaveChatSession(h.sessionsDir, session); err != nil {
		h.endTask()
		h.writeErrorReq(w, r, http.StatusInternalServerError, "save_session_failed", err.Error())
		return
	}

	// 缓存消息用于重试
	h.lastChatMessage = req.Content

	go func() {
		// defer 确保任何错误路径都会释放任务锁，否则后续所有任务将永久 409
		defer h.endTask()
		h.logger.TaskStart("chat_message")
		ctx := h.taskCtx

		var history []AgentStep
		for _, m := range session.Messages {
			if m.Role == "user" {
				history = append(history, AgentStep{Role: "user", Content: m.Content})
			} else if m.Role == "assistant" {
				step := AgentStep{Role: "assistant", Content: m.Content}
				if len(m.ToolCalls) > 0 {
					step.ToolCall = &m.ToolCalls[0]
				}
				history = append(history, step)
			} else if m.Role == "tool" {
				history = append(history, AgentStep{
					Role:           "tool",
					ToolResult:     m.ToolResult,
					ToolResultKey:  m.ToolResultKey,
					ToolResultArgs: m.ToolResultArgs,
				})
			}
		}

		agentCtx := &AgentContext{
			APICfg:       h.apiCfg,
			Settings:     h.settings,
			SettingsPath: h.settingsPath,
			State:        h.state,
			Config:       h.cfg,
			Skills:       h.skills,
			Logger:       h.logger,
			ContextPage:  req.ContextPage,
			ProgressPath: h.progressPath,
			CfgPath:      h.cfgPath,
			SessionsDir:  h.sessionsDir,
			ProjectDir:   filepath.Join(h.progDir, "storys", h.projectName),
			StartAsync: func(taskName string, fn func(goCtx context.Context) error) {
				// 子任务必须计入 activeWork，否则 Agent 主循环结束后锁被释放，
				// 子任务仍在运行时新任务可并发进入，造成数据竞争。
				if !h.startChildWork() {
					h.logger.WarnKey("log.child_task_start_failed", taskName)
					return
				}
				childCtx := h.taskCtx
				go func() {
					defer h.endTask()
					h.logger.TaskStart(taskName)
					err := fn(childCtx)
					h.broadcastProgress()
					h.logger.TaskEnd(taskName, err == nil)
				}()
			},
		}

		reply, newHistory, err := RunAgentLoop(ctx, agentCtx, req.Content, history, 30)
		if err != nil {
			// 即使失败也保存已产生的对话步骤，避免上下文丢失
			saveAgentSteps(session, newHistory[len(history):])
			session.UpdatedAt = time.Now().Format(time.RFC3339)
			if saveErr := SaveChatSession(h.sessionsDir, session); saveErr != nil {
				h.logger.WarnKey("log.save_session_failed", saveErr)
			}
			if ctx.Err() != nil {
				h.logger.WarnKey("log.chat_cancelled")
			} else {
				h.logger.ErrorKey("log.chat_failed", err)
			}
			h.logger.TaskEnd("chat_message", false)
			return
		}

		saveAgentSteps(session, newHistory[len(history):])

		if reply != "" {
			found := false
			for i := len(session.Messages) - 1; i >= 0; i-- {
				if session.Messages[i].Role == "assistant" && session.Messages[i].Content == reply {
					found = true
					break
				}
			}
			if !found {
				session.Messages = append(session.Messages, ChatMessage{
					Role:      "assistant",
					Content:   reply,
					Timestamp: time.Now().Format(time.RFC3339),
				})
			}
		}

		session.UpdatedAt = time.Now().Format(time.RFC3339)

		if err := SaveChatSession(h.sessionsDir, session); err != nil {
			h.logger.WarnKey("log.save_session_failed", err)
		}

		h.logger.ChatChunk(sessionID, reply)

		h.logger.SuccessKey("log.chat_done")
		h.logger.TaskEnd("chat_message", true)
	}()

	h.writeJSON(w, http.StatusAccepted, map[string]string{"status": "started"})
}

// saveAgentSteps 将 Agent 步骤追加为会话消息。
func saveAgentSteps(session *ChatSession, steps []AgentStep) {
	for _, step := range steps {
		if step.Role == "assistant" {
			msg := ChatMessage{
				Role:      "assistant",
				Content:   step.Content,
				Timestamp: time.Now().Format(time.RFC3339),
			}
			if step.ToolCall != nil {
				msg.ToolCalls = []ToolCall{*step.ToolCall}
			}
			session.Messages = append(session.Messages, msg)
		} else if step.Role == "tool" {
			session.Messages = append(session.Messages, ChatMessage{
				Role:           "tool",
				ToolResult:     step.ToolResult,
				ToolResultKey:  step.ToolResultKey,
				ToolResultArgs: step.ToolResultArgs,
				Timestamp:      time.Now().Format(time.RFC3339),
			})
		}
	}
}

func writeFileAtomic(path string, data []byte) error {
	tmpPath := path + ".tmp"
	if err := writeFile(tmpPath, data); err != nil {
		return err
	}
	if err := renameFile(tmpPath, path); err != nil {
		deleteFile(tmpPath)
		return err
	}
	return nil
}

func writeFile(path string, data []byte) error {
	return writeFileImpl(path, data)
}

func deleteFile(path string) error {
	return deleteFileImpl(path)
}

func renameFile(old, new string) error {
	return renameFileImpl(old, new)
}
