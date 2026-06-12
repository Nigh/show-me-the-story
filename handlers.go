package main

import (
	"context"
	"encoding/json"
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
func (h *Handlers) ensureProject(w http.ResponseWriter) bool {
	h.projectMu.RLock()
	defer h.projectMu.RUnlock()
	if h.projectName == "" {
		h.writeError(w, http.StatusBadRequest, "请先选择一个项目")
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
	h.taskCtx, h.taskCancel = context.WithCancel(context.Background())
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
func (h *Handlers) rejectIfTaskRunning(w http.ResponseWriter) bool {
	if h.isTaskRunning() {
		h.writeError(w, http.StatusConflict, "有AI任务正在运行，暂不能修改，请等待任务完成或先停止任务")
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
		h.writeError(w, http.StatusBadRequest, "无效的JSON: "+err.Error())
		return
	}

	h.taskMu.Lock()
	h.autoConfirm = req.Enabled
	h.taskMu.Unlock()

	if req.Enabled {
		h.logger.Info("已开启自动确认模式：每章生成完成后将自动确认并继续生成下一章")
	} else {
		h.logger.Info("已关闭自动确认模式")
	}
	h.writeJSON(w, http.StatusOK, map[string]bool{"enabled": req.Enabled})
}

func (h *Handlers) PostTaskStop(w http.ResponseWriter, r *http.Request) {
	h.taskMu.Lock()
	if !h.taskRunning {
		h.taskMu.Unlock()
		h.writeError(w, http.StatusBadRequest, "没有正在运行的任务")
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
	if h.rejectIfTaskRunning(w) {
		return
	}
	var newCfg APIConfig
	if err := json.NewDecoder(r.Body).Decode(&newCfg); err != nil {
		h.writeError(w, http.StatusBadRequest, "无效的JSON: "+err.Error())
		return
	}

	if newCfg.HTTPTimeoutSeconds <= 0 {
		newCfg.HTTPTimeoutSeconds = 300
	}
	if newCfg.ContextBudgetTokens <= 0 {
		newCfg.ContextBudgetTokens = defaultContextBudgetTokens
	}

	data, err := json.MarshalIndent(newCfg, "", "  ")
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "序列化API配置失败: "+err.Error())
		return
	}
	if err := writeFileAtomic(h.apiCfgPath, data); err != nil {
		h.writeError(w, http.StatusInternalServerError, "保存API配置失败: "+err.Error())
		return
	}

	h.apiCfg = &newCfg
	h.writeJSON(w, http.StatusOK, h.apiCfg)
}

func (h *Handlers) GetConfig(w http.ResponseWriter, r *http.Request) {
	h.writeJSON(w, http.StatusOK, h.cfg)
}

func (h *Handlers) PutConfig(w http.ResponseWriter, r *http.Request) {
	if h.rejectIfTaskRunning(w) {
		return
	}
	var newCfg Config
	if err := json.NewDecoder(r.Body).Decode(&newCfg); err != nil {
		h.writeError(w, http.StatusBadRequest, "无效的JSON: "+err.Error())
		return
	}

	if newCfg.Story.ChapterCount <= 0 {
		newCfg.Story.ChapterCount = 30
	}
	if newCfg.Story.TargetWordsPerChapter <= 0 {
		newCfg.Story.TargetWordsPerChapter = 2500
	}
	newCfg.Prompts.applyDefaults()

	data, err := json.MarshalIndent(newCfg, "", "  ")
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "序列化配置失败: "+err.Error())
		return
	}
	if err := writeFileAtomic(h.cfgPath, data); err != nil {
		h.writeError(w, http.StatusInternalServerError, "保存配置失败: "+err.Error())
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
		h.writeError(w, http.StatusConflict, "有任务正在运行，无法重置进度")
		return
	}

	if err := deleteFile(h.progressPath); err != nil {
		h.writeError(w, http.StatusInternalServerError, "删除进度文件失败: "+err.Error())
		return
	}

	h.state = &Progress{Phase: "outline"}
	h.writeJSON(w, http.StatusOK, h.state)
}

func (h *Handlers) PostOutlineGenerate(w http.ResponseWriter, r *http.Request) {
	if !h.ensureProject(w) {
		return
	}
	// 检查是否有写作中/审核中/已确认的章节，如果有则拒绝
	for _, ch := range h.state.Chapters {
		if ch.Status == StatusWriting || ch.Status == StatusReview {
			h.writeError(w, http.StatusConflict, "有正在写作/审核中的章节，请先处理后再重新生成大纲")
			return
		}
		if ch.Status == StatusAccepted {
			h.writeError(w, http.StatusConflict, "存在已确认章节，无法整体重新生成大纲。如需追加章节请使用「生成后续大纲」")
			return
		}
	}

	if !h.tryStartTask() {
		h.writeError(w, http.StatusConflict, "有任务正在运行，请等待完成")
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
			h.logger.Info("已自动清除旧的大纲（pending 章节）")
		}
		h.logger.TaskStart("outline_generation")
		ctx := h.taskCtx

		h.logger.Info("正在生成小说大纲...")
		err := GenerateOutlineAction(ctx, h.apiCfg, h.cfg, h.state, h.progressPath, h.logger)

		if err != nil {
			if ctx.Err() != nil {
				h.logger.Warn("大纲生成已取消")
				h.logger.TaskEnd("outline_generation", false)
			} else {
				h.logger.Error(fmt.Sprintf("大纲生成失败: %v", err))
				h.logger.TaskEnd("outline_generation", false)
			}
			return
		}

		h.logger.Success("大纲生成完成！")
		h.logger.TaskEnd("outline_generation", true)
		h.broadcastProgress()
	}()

	h.writeJSON(w, http.StatusAccepted, map[string]string{"status": "started"})
}

func (h *Handlers) PostOutlineConfirm(w http.ResponseWriter, r *http.Request) {
	if h.isTaskRunning() {
		h.writeError(w, http.StatusConflict, "有任务正在运行，请等待完成")
		return
	}

	if h.state.Phase != "outline" {
		h.writeError(w, http.StatusBadRequest, "当前不在大纲阶段")
		return
	}

	if len(h.state.Chapters) == 0 {
		h.writeError(w, http.StatusBadRequest, "大纲为空，请先生成大纲")
		return
	}

	if err := ConfirmOutlineAction(h.state, h.progressPath); err != nil {
		h.writeError(w, http.StatusInternalServerError, "确认大纲失败: "+err.Error())
		return
	}

	h.logger.Success("大纲已确认，进入写作阶段。")
	h.writeJSON(w, http.StatusOK, h.state)
}

func (h *Handlers) PostOutlineRevise(w http.ResponseWriter, r *http.Request) {
	if !h.tryStartTask() {
		h.writeError(w, http.StatusConflict, "有任务正在运行，请等待完成")
		return
	}

	var body struct {
		Feedback string `json:"feedback"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Feedback == "" {
		h.endTask()
		h.writeError(w, http.StatusBadRequest, "缺少 feedback 字段")
		return
	}

	go func() {
		defer h.endTask()
		h.logger.TaskStart("outline_revision")
		ctx := h.taskCtx

		h.logger.Info("正在根据意见修订大纲...")
		err := ReviseOutlineAction(ctx, h.apiCfg, h.cfg, h.state, h.progressPath, body.Feedback, h.logger)

		if err != nil {
			if ctx.Err() != nil {
				h.logger.Warn("大纲修订已取消")
				h.logger.TaskEnd("outline_revision", false)
			} else {
				h.logger.Error(fmt.Sprintf("大纲修订失败: %v", err))
				h.logger.TaskEnd("outline_revision", false)
			}
			return
		}

		h.logger.Success("大纲已修订。")
		h.logger.TaskEnd("outline_revision", true)
		h.broadcastProgress()
	}()

	h.writeJSON(w, http.StatusAccepted, map[string]string{"status": "started"})
}

func (h *Handlers) PostChapterGenerate(w http.ResponseWriter, r *http.Request) {
	if !h.tryStartTask() {
		h.writeError(w, http.StatusConflict, "有任务正在运行，请等待完成")
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

			h.logger.Info(fmt.Sprintf("正在创作第 %d 章...", chIdx+1))
			err := GenerateChapterAction(ctx, h.apiCfg, h.cfg, h.state, h.progressPath, h.settings, h.logger)

			if err != nil {
				if ctx.Err() != nil {
					h.logger.Warn("章节创作已取消")
				} else {
					h.logger.Error(fmt.Sprintf("章节创作失败: %v", err))
				}
				h.logger.TaskEnd("chapter_generation", false)
				return
			}

			h.logger.Success(fmt.Sprintf("第 %d 章《%s》创作完成！", chIdx+1, chTitle))
			h.broadcastProgress()

			// 自动确认模式：自动确认本章并继续生成下一章；关闭开关后在本章结束时停止
			if !h.isAutoConfirmOn() {
				break
			}
			if err := ConfirmChapterAction(h.state, h.progressPath); err != nil {
				h.logger.Warn(fmt.Sprintf("自动确认失败: %v", err))
				break
			}
			h.logger.Success(fmt.Sprintf("第 %d 章《%s》已自动确认。", chIdx+1, chTitle))
			h.broadcastProgress()

			if h.state.CurrentChapterIndex >= len(h.state.Chapters) {
				h.logger.Success("全部章节已创作完成！")
				break
			}
			if ctx.Err() != nil {
				h.logger.Warn("任务已取消，停止自动续写")
				break
			}
		}

		h.logger.TaskEnd("chapter_generation", true)
		h.broadcastProgress()
	}()

	h.writeJSON(w, http.StatusAccepted, map[string]string{"status": "started"})
}

func (h *Handlers) PostChapterConfirm(w http.ResponseWriter, r *http.Request) {
	if h.isTaskRunning() {
		h.writeError(w, http.StatusConflict, "有任务正在运行，请等待完成")
		return
	}

	if h.state.Phase != "writing" {
		h.writeError(w, http.StatusBadRequest, "当前不在写作阶段")
		return
	}

	if err := ConfirmChapterAction(h.state, h.progressPath); err != nil {
		h.writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	ch := h.state.Chapters[h.state.CurrentChapterIndex-1]
	h.logger.Success(fmt.Sprintf("第 %d 章已确认。", ch.Num))
	h.writeJSON(w, http.StatusOK, h.state)
}

func (h *Handlers) PostChapterRevise(w http.ResponseWriter, r *http.Request) {
	if !h.tryStartTask() {
		h.writeError(w, http.StatusConflict, "有任务正在运行，请等待完成")
		return
	}

	var body struct {
		Feedback string `json:"feedback"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Feedback == "" {
		h.endTask()
		h.writeError(w, http.StatusBadRequest, "缺少 feedback 字段")
		return
	}

	go func() {
		defer h.endTask()
		h.logger.TaskStart("chapter_revision")
		ctx := h.taskCtx

		h.logger.Info("正在根据意见修改当前章节...")
		err := ReviseChapterAction(ctx, h.apiCfg, h.cfg, h.state, h.progressPath, body.Feedback, h.settings, h.logger)

		if err != nil {
			if ctx.Err() != nil {
				h.logger.Warn("章节修订已取消")
				h.logger.TaskEnd("chapter_revision", false)
			} else {
				h.logger.Error(fmt.Sprintf("章节修订失败: %v", err))
				h.logger.TaskEnd("chapter_revision", false)
			}
			return
		}

		h.logger.Success("章节已修订。")
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
		h.writeError(w, http.StatusBadRequest, "无效的章节编号")
		return
	}

	if !h.tryStartTask() {
		h.writeError(w, http.StatusConflict, "有任务正在运行，请等待完成")
		return
	}

	var body struct {
		Feedback string `json:"feedback"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Feedback == "" {
		h.endTask()
		h.writeError(w, http.StatusBadRequest, "缺少 feedback 字段")
		return
	}

	go func() {
		defer h.endTask()
		h.logger.TaskStart("chapter_revision")
		ctx := h.taskCtx

		h.logger.Info(fmt.Sprintf("正在定向修订第 %d 章...", num))
		err := ReviseSpecificChapterAction(ctx, h.apiCfg, h.cfg, h.state, h.progressPath, num, body.Feedback, h.settings, h.logger)

		if err != nil {
			if ctx.Err() != nil {
				h.logger.Warn("章节修订已取消")
			} else {
				h.logger.Error(fmt.Sprintf("章节修订失败: %v", err))
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
	if !h.ensureProject(w) {
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
		h.writeError(w, http.StatusBadRequest, "没有可优化的章节（需要至少两个相邻的已确认章节）")
		return
	}

	if !h.tryStartTask() {
		h.writeError(w, http.StatusConflict, "有任务正在运行，请等待完成")
		return
	}

	go func() {
		defer h.endTask()
		h.logger.TaskStart("smooth_transitions")
		ctx := h.taskCtx

		err := SmoothTransitionsAction(ctx, h.apiCfg, h.cfg, h.state, h.progressPath, h.logger)
		if err != nil {
			if ctx.Err() != nil {
				h.logger.Warn("章节衔接优化已取消（已完成部分不会丢失）")
			} else {
				h.logger.Error(fmt.Sprintf("章节衔接优化失败: %v", err))
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
		h.writeError(w, http.StatusConflict, "有任务正在运行，无法删除章节")
		return
	}

	if len(h.state.Chapters) == 0 {
		h.writeError(w, http.StatusBadRequest, "没有可删除的章节")
		return
	}

	lastIdx := len(h.state.Chapters) - 1
	ch := h.state.Chapters[lastIdx]

	if ch.Status == StatusWriting {
		h.writeError(w, http.StatusConflict, "正在写作中的章节无法删除")
		return
	}

	deleteFile(ChapterMarkdownPath(h.projectDir(), ch.Num))

	h.state.Chapters = h.state.Chapters[:lastIdx]

	if h.state.CurrentChapterIndex > len(h.state.Chapters) {
		h.state.CurrentChapterIndex = len(h.state.Chapters)
	}

	if len(h.state.Chapters) == 0 {
		h.state.Phase = "outline"
		h.state.CurrentChapterIndex = 0
		h.state.StoryConfigSnapshot = nil
	}

	if err := SaveProgress(h.progressPath, h.state); err != nil {
		h.writeError(w, http.StatusInternalServerError, "保存进度失败: "+err.Error())
		return
	}

	h.logger.Success(fmt.Sprintf("已删除第 %d 章。", ch.Num))
	h.writeJSON(w, http.StatusOK, h.state)
}

func (h *Handlers) DeleteOutline(w http.ResponseWriter, r *http.Request) {
	if h.isTaskRunning() {
		h.writeError(w, http.StatusConflict, "有任务正在运行，无法删除大纲")
		return
	}

	for _, ch := range h.state.Chapters {
		if ch.Status == StatusWriting || ch.Status == StatusReview {
			h.writeError(w, http.StatusConflict, "有正在写作/审核中的章节，请先处理后再删除大纲")
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
		h.writeError(w, http.StatusInternalServerError, "保存进度失败: "+err.Error())
		return
	}

	h.logger.Success("大纲已删除。")
	h.writeJSON(w, http.StatusOK, h.state)
}

func (h *Handlers) PutChapterOutline(w http.ResponseWriter, r *http.Request) {
	if h.isTaskRunning() {
		h.writeError(w, http.StatusConflict, "有任务正在运行，请等待完成")
		return
	}

	numStr := r.PathValue("num")
	var num int
	if _, err := fmt.Sscanf(numStr, "%d", &num); err != nil {
		h.writeError(w, http.StatusBadRequest, "无效的章节编号")
		return
	}

	var body struct {
		Title   string `json:"title"`
		Outline string `json:"outline"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.writeError(w, http.StatusBadRequest, "无效的JSON: "+err.Error())
		return
	}

	if err := EditChapterOutline(h.state, num, body.Title, body.Outline); err != nil {
		h.writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := SaveProgress(h.progressPath, h.state); err != nil {
		h.writeError(w, http.StatusInternalServerError, "保存进度失败: "+err.Error())
		return
	}

	h.logger.Success(fmt.Sprintf("第 %d 章大纲已更新。", num))
	h.writeJSON(w, http.StatusOK, h.state)
}

func (h *Handlers) PostSettingsReconcile(w http.ResponseWriter, r *http.Request) {
	if !h.tryStartTask() {
		h.writeError(w, http.StatusConflict, "有任务正在运行，请等待完成")
		return
	}

	var body StoryConfig
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.endTask()
		h.writeError(w, http.StatusBadRequest, "无效的JSON: "+err.Error())
		return
	}

	go func() {
		defer h.endTask()
		h.logger.TaskStart("settings_reconciliation")
		ctx := h.taskCtx

		h.logger.Info("正在协调新设定与已有内容...")
		err := ReconcileSettingsAction(ctx, h.apiCfg, h.cfg, h.state, body, h.progressPath, h.cfgPath, h.logger)

		if err != nil {
			if ctx.Err() != nil {
				h.logger.Warn("设定协调已取消")
				h.logger.TaskEnd("settings_reconciliation", false)
			} else {
				h.logger.Error(fmt.Sprintf("设定协调失败: %v", err))
				h.logger.TaskEnd("settings_reconciliation", false)
			}
			return
		}

		h.logger.Success("设定协调完成！")
		h.logger.TaskEnd("settings_reconciliation", true)
		h.broadcastProgress()
	}()

	h.writeJSON(w, http.StatusAccepted, map[string]string{"status": "started"})
}

func (h *Handlers) DeleteChaptersFrom(w http.ResponseWriter, r *http.Request) {
	if h.isTaskRunning() {
		h.writeError(w, http.StatusConflict, "有任务正在运行，无法删除章节")
		return
	}

	numStr := r.PathValue("num")
	var num int
	if _, err := fmt.Sscanf(numStr, "%d", &num); err != nil {
		h.writeError(w, http.StatusBadRequest, "无效的章节编号")
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
		h.writeError(w, http.StatusNotFound, fmt.Sprintf("章节 %d 不存在", num))
		return
	}

	for i := startIdx; i < len(h.state.Chapters); i++ {
		if h.state.Chapters[i].Status == StatusWriting {
			h.writeError(w, http.StatusConflict, "删除范围内有正在写作中的章节，无法删除")
			return
		}
	}

	deletedCount := len(h.state.Chapters) - startIdx

	for i := startIdx; i < len(h.state.Chapters); i++ {
		mdFile := ChapterMarkdownPath(h.projectDir(), h.state.Chapters[i].Num)
		if err := deleteFile(mdFile); err != nil {
			h.logger.Warn(fmt.Sprintf("删除文件 %s 失败: %v", mdFile, err))
		}
	}

	h.state.Chapters = h.state.Chapters[:startIdx]

	if h.state.CurrentChapterIndex > len(h.state.Chapters) {
		h.state.CurrentChapterIndex = len(h.state.Chapters)
	}

	if len(h.state.Chapters) == 0 {
		h.state.Phase = "outline"
		h.state.CurrentChapterIndex = 0
		h.state.StoryConfigSnapshot = nil
	}

	if err := SaveProgress(h.progressPath, h.state); err != nil {
		h.writeError(w, http.StatusInternalServerError, "保存进度失败: "+err.Error())
		return
	}

	h.logger.Success(fmt.Sprintf("已从第 %d 章删除到末尾，共删除 %d 章。", num, deletedCount))
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
	h.writeJSON(w, http.StatusOK, map[string]interface{}{
		"phase":           h.state.Phase,
		"title":           h.state.Title,
		"total_chapters":  len(h.state.Chapters),
		"is_task_running": h.isTaskRunning(),
		"auto_confirm":    h.isAutoConfirmOn(),
	})
}

func (h *Handlers) GetForeshadows(w http.ResponseWriter, r *http.Request) {
	if h.state.Foreshadows == nil {
		h.writeJSON(w, http.StatusOK, []Foreshadow{})
		return
	}
	h.writeJSON(w, http.StatusOK, h.state.Foreshadows)
}

func (h *Handlers) PostForeshadowsSuggest(w http.ResponseWriter, r *http.Request) {
	if !h.tryStartTask() {
		h.writeError(w, http.StatusConflict, "有任务正在运行，请等待完成")
		return
	}

	if len(h.state.Chapters) == 0 {
		h.endTask()
		h.writeError(w, http.StatusBadRequest, "请先生成大纲")
		return
	}

	go func() {
		defer h.endTask()
		h.logger.TaskStart("foreshadow_suggest")
		ctx := h.taskCtx

		h.logger.Info("正在分析大纲，设计伏笔方案...")
		suggestions, err := SuggestForeshadows(ctx, h.apiCfg, h.cfg, h.state, h.logger)

		if err != nil {
			if ctx.Err() != nil {
				h.logger.Warn("伏笔建议已取消")
				h.logger.TaskEnd("foreshadow_suggest", false)
			} else {
				h.logger.Error(fmt.Sprintf("伏笔建议生成失败: %v", err))
				h.logger.TaskEnd("foreshadow_suggest", false)
			}
			return
		}

		h.logger.Success(fmt.Sprintf("伏笔建议生成完成，共 %d 条", len(suggestions)))
		h.logger.TaskEnd("foreshadow_suggest", true)
		h.logger.ForeshadowSuggestions(suggestions)
	}()

	h.writeJSON(w, http.StatusAccepted, map[string]string{"status": "started"})
}

func (h *Handlers) PostForeshadow(w http.ResponseWriter, r *http.Request) {
	if h.rejectIfTaskRunning(w) {
		return
	}
	var req struct {
		Name          string `json:"name"`
		Description   string `json:"description"`
		PlantChapter  int    `json:"plant_chapter"`
		TargetChapter int    `json:"target_chapter"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, "无效的JSON: "+err.Error())
		return
	}
	if req.Name == "" {
		h.writeError(w, http.StatusBadRequest, "缺少 name")
		return
	}
	if req.Description == "" {
		h.writeError(w, http.StatusBadRequest, "缺少 description")
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
		h.writeError(w, http.StatusInternalServerError, "保存失败: "+err.Error())
		return
	}

	h.writeJSON(w, http.StatusOK, fs)
}

func (h *Handlers) PutForeshadow(w http.ResponseWriter, r *http.Request) {
	if h.rejectIfTaskRunning(w) {
		return
	}
	idStr := r.PathValue("id")
	var id int
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		h.writeError(w, http.StatusBadRequest, "无效的伏笔ID")
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
		h.writeError(w, http.StatusBadRequest, "无效的JSON: "+err.Error())
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
		h.writeError(w, http.StatusNotFound, "伏笔不存在")
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
		h.writeError(w, http.StatusInternalServerError, "保存失败: "+err.Error())
		return
	}

	h.writeJSON(w, http.StatusOK, fs)
}

func (h *Handlers) DeleteForeshadow(w http.ResponseWriter, r *http.Request) {
	if h.rejectIfTaskRunning(w) {
		return
	}
	idStr := r.PathValue("id")
	var id int
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		h.writeError(w, http.StatusBadRequest, "无效的伏笔ID")
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
		h.writeError(w, http.StatusNotFound, "伏笔不存在")
		return
	}

	h.state.Foreshadows = append(h.state.Foreshadows[:idx], h.state.Foreshadows[idx+1:]...)

	if err := SaveProgress(h.progressPath, h.state); err != nil {
		h.writeError(w, http.StatusInternalServerError, "保存失败: "+err.Error())
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *Handlers) PostForeshadowsConfirm(w http.ResponseWriter, r *http.Request) {
	if h.rejectIfTaskRunning(w) {
		return
	}
	var req struct {
		Foreshadows []Foreshadow `json:"foreshadows"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, "无效的JSON: "+err.Error())
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
		h.writeError(w, http.StatusInternalServerError, "保存失败: "+err.Error())
		return
	}

	h.writeJSON(w, http.StatusOK, h.state.Foreshadows)
}

func (h *Handlers) PostContinueImport(w http.ResponseWriter, r *http.Request) {
	if !h.tryStartTask() {
		h.writeError(w, http.StatusConflict, "有任务正在运行，请等待完成")
		return
	}

	var body struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Content == "" {
		h.endTask()
		h.writeError(w, http.StatusBadRequest, "缺少 content 字段")
		return
	}

	go func() {
		defer h.endTask()
		h.logger.TaskStart("continue_analysis")
		ctx := h.taskCtx

		h.logger.Info("正在分析已有内容...")
		analysis, err := AnalyzeExistingContent(ctx, h.apiCfg, h.cfg, body.Content)

		if err != nil {
			if ctx.Err() != nil {
				h.logger.Warn("内容分析已取消")
				h.logger.TaskEnd("continue_analysis", false)
			} else {
				h.logger.Error(fmt.Sprintf("内容分析失败: %v", err))
				h.logger.TaskEnd("continue_analysis", false)
			}
			return
		}

		h.pendingContinueContent = body.Content

		h.logger.Success(fmt.Sprintf("内容分析完成，发现 %d 章", len(analysis.Chapters)))
		h.logger.TaskEnd("continue_analysis", true)
		h.logger.ContinueAnalysisResult(analysis)
	}()

	h.writeJSON(w, http.StatusAccepted, map[string]string{"status": "started"})
}

func (h *Handlers) PostContinueConfirm(w http.ResponseWriter, r *http.Request) {
	if h.isTaskRunning() {
		h.writeError(w, http.StatusConflict, "有任务正在运行，请等待完成")
		return
	}

	if h.state.Phase != "outline" {
		h.writeError(w, http.StatusBadRequest, "续写前请先重置进度")
		return
	}

	if h.pendingContinueContent == "" {
		h.writeError(w, http.StatusBadRequest, "请先分析内容")
		return
	}

	var analysis ContinueAnalysis
	if err := json.NewDecoder(r.Body).Decode(&analysis); err != nil {
		h.writeError(w, http.StatusBadRequest, "无效的JSON: "+err.Error())
		return
	}

	if len(analysis.Chapters) == 0 {
		h.writeError(w, http.StatusBadRequest, "分析结果中没有任何章节")
		return
	}

	content := h.pendingContinueContent
	h.pendingContinueContent = ""

	if err := ImportContinueAction(h.cfg, h.state, &analysis, content, h.progressPath, h.cfgPath); err != nil {
		h.writeError(w, http.StatusInternalServerError, "导入续写失败: "+err.Error())
		return
	}

	h.logger.Success("续写导入完成，已进入大纲阶段。")
	h.writeJSON(w, http.StatusOK, h.state)
}

func (h *Handlers) PostOutlineGenerateContinuation(w http.ResponseWriter, r *http.Request) {
	if !h.tryStartTask() {
		h.writeError(w, http.StatusConflict, "有任务正在运行，请等待完成")
		return
	}

	if h.state.Phase != "outline" {
		h.endTask()
		h.writeError(w, http.StatusBadRequest, "当前不在大纲阶段")
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

		h.logger.Info("正在生成续写大纲...")
		err := GenerateContinuationOutline(ctx, h.apiCfg, h.cfg, h.state, body.ChapterCount, h.progressPath, h.logger)

		if err != nil {
			if ctx.Err() != nil {
				h.logger.Warn("续写大纲生成已取消")
				h.logger.TaskEnd("continuation_outline", false)
			} else {
				h.logger.Error(fmt.Sprintf("续写大纲生成失败: %v", err))
				h.logger.TaskEnd("continuation_outline", false)
			}
			return
		}

		h.logger.Success("续写大纲生成完成！")
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
	if h.rejectIfTaskRunning(w) {
		return
	}
	var c Character
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		h.writeError(w, http.StatusBadRequest, "无效的JSON: "+err.Error())
		return
	}
	if c.Name == "" {
		h.writeError(w, http.StatusBadRequest, "角色名不能为空")
		return
	}

	c.ID = h.settings.nextCharacterID()
	h.settings.Characters = append(h.settings.Characters, c)

	if err := SaveProjectSettings(h.settingsPath, h.settings); err != nil {
		h.writeError(w, http.StatusInternalServerError, "保存失败: "+err.Error())
		return
	}

	h.writeJSON(w, http.StatusOK, c)
}

func (h *Handlers) PutCharacter(w http.ResponseWriter, r *http.Request) {
	if h.rejectIfTaskRunning(w) {
		return
	}
	id := r.PathValue("id")

	var req Character
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, "无效的JSON: "+err.Error())
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
				h.writeError(w, http.StatusInternalServerError, "保存失败: "+err.Error())
				return
			}

			h.writeJSON(w, http.StatusOK, h.settings.Characters[i])
			return
		}
	}

	h.writeError(w, http.StatusNotFound, "角色不存在")
}

func (h *Handlers) DeleteCharacter(w http.ResponseWriter, r *http.Request) {
	if h.rejectIfTaskRunning(w) {
		return
	}
	id := r.PathValue("id")

	for i, c := range h.settings.Characters {
		if c.ID == id {
			h.settings.Characters = append(h.settings.Characters[:i], h.settings.Characters[i+1:]...)
			if err := SaveProjectSettings(h.settingsPath, h.settings); err != nil {
				h.writeError(w, http.StatusInternalServerError, "保存失败: "+err.Error())
				return
			}
			h.writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
			return
		}
	}

	h.writeError(w, http.StatusNotFound, "角色不存在")
}

func (h *Handlers) PostWorldview(w http.ResponseWriter, r *http.Request) {
	if h.rejectIfTaskRunning(w) {
		return
	}
	var wv WorldviewEntry
	if err := json.NewDecoder(r.Body).Decode(&wv); err != nil {
		h.writeError(w, http.StatusBadRequest, "无效的JSON: "+err.Error())
		return
	}
	if wv.Name == "" || wv.Description == "" {
		h.writeError(w, http.StatusBadRequest, "名称和描述不能为空")
		return
	}

	wv.ID = h.settings.nextWorldviewID()
	h.settings.Worldview = append(h.settings.Worldview, wv)

	if err := SaveProjectSettings(h.settingsPath, h.settings); err != nil {
		h.writeError(w, http.StatusInternalServerError, "保存失败: "+err.Error())
		return
	}

	h.writeJSON(w, http.StatusOK, wv)
}

func (h *Handlers) PutWorldview(w http.ResponseWriter, r *http.Request) {
	if h.rejectIfTaskRunning(w) {
		return
	}
	id := r.PathValue("id")

	var req WorldviewEntry
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, "无效的JSON: "+err.Error())
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
				h.writeError(w, http.StatusInternalServerError, "保存失败: "+err.Error())
				return
			}

			h.writeJSON(w, http.StatusOK, h.settings.Worldview[i])
			return
		}
	}

	h.writeError(w, http.StatusNotFound, "世界观条目不存在")
}

func (h *Handlers) DeleteWorldview(w http.ResponseWriter, r *http.Request) {
	if h.rejectIfTaskRunning(w) {
		return
	}
	id := r.PathValue("id")

	for i, wv := range h.settings.Worldview {
		if wv.ID == id {
			h.settings.Worldview = append(h.settings.Worldview[:i], h.settings.Worldview[i+1:]...)
			if err := SaveProjectSettings(h.settingsPath, h.settings); err != nil {
				h.writeError(w, http.StatusInternalServerError, "保存失败: "+err.Error())
				return
			}
			h.writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
			return
		}
	}

	h.writeError(w, http.StatusNotFound, "世界观条目不存在")
}

func (h *Handlers) PostOrganization(w http.ResponseWriter, r *http.Request) {
	if h.rejectIfTaskRunning(w) {
		return
	}
	var o Organization
	if err := json.NewDecoder(r.Body).Decode(&o); err != nil {
		h.writeError(w, http.StatusBadRequest, "无效的JSON: "+err.Error())
		return
	}
	if o.Name == "" {
		h.writeError(w, http.StatusBadRequest, "组织名不能为空")
		return
	}

	o.ID = h.settings.nextOrganizationID()
	h.settings.Organizations = append(h.settings.Organizations, o)

	if err := SaveProjectSettings(h.settingsPath, h.settings); err != nil {
		h.writeError(w, http.StatusInternalServerError, "保存失败: "+err.Error())
		return
	}

	h.writeJSON(w, http.StatusOK, o)
}

func (h *Handlers) PutOrganization(w http.ResponseWriter, r *http.Request) {
	if h.rejectIfTaskRunning(w) {
		return
	}
	id := r.PathValue("id")

	var req Organization
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, "无效的JSON: "+err.Error())
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
				h.writeError(w, http.StatusInternalServerError, "保存失败: "+err.Error())
				return
			}

			h.writeJSON(w, http.StatusOK, h.settings.Organizations[i])
			return
		}
	}

	h.writeError(w, http.StatusNotFound, "组织不存在")
}

func (h *Handlers) DeleteOrganization(w http.ResponseWriter, r *http.Request) {
	if h.rejectIfTaskRunning(w) {
		return
	}
	id := r.PathValue("id")

	for i, o := range h.settings.Organizations {
		if o.ID == id {
			h.settings.Organizations = append(h.settings.Organizations[:i], h.settings.Organizations[i+1:]...)
			if err := SaveProjectSettings(h.settingsPath, h.settings); err != nil {
				h.writeError(w, http.StatusInternalServerError, "保存失败: "+err.Error())
				return
			}
			h.writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
			return
		}
	}

	h.writeError(w, http.StatusNotFound, "组织不存在")
}

func (h *Handlers) PostRelation(w http.ResponseWriter, r *http.Request) {
	if h.rejectIfTaskRunning(w) {
		return
	}
	var rel Relation
	if err := json.NewDecoder(r.Body).Decode(&rel); err != nil {
		h.writeError(w, http.StatusBadRequest, "无效的JSON: "+err.Error())
		return
	}
	if rel.SourceID == "" || rel.TargetID == "" {
		h.writeError(w, http.StatusBadRequest, "源和目标不能为空")
		return
	}

	rel.ID = h.settings.nextRelationID()
	h.settings.Relations = append(h.settings.Relations, rel)

	if err := SaveProjectSettings(h.settingsPath, h.settings); err != nil {
		h.writeError(w, http.StatusInternalServerError, "保存失败: "+err.Error())
		return
	}

	h.writeJSON(w, http.StatusOK, rel)
}

func (h *Handlers) PutRelation(w http.ResponseWriter, r *http.Request) {
	if h.rejectIfTaskRunning(w) {
		return
	}
	id := r.PathValue("id")

	var req Relation
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, "无效的JSON: "+err.Error())
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
				h.writeError(w, http.StatusInternalServerError, "保存失败: "+err.Error())
				return
			}

			h.writeJSON(w, http.StatusOK, h.settings.Relations[i])
			return
		}
	}

	h.writeError(w, http.StatusNotFound, "关系不存在")
}

func (h *Handlers) DeleteRelation(w http.ResponseWriter, r *http.Request) {
	if h.rejectIfTaskRunning(w) {
		return
	}
	id := r.PathValue("id")

	for i, rel := range h.settings.Relations {
		if rel.ID == id {
			h.settings.Relations = append(h.settings.Relations[:i], h.settings.Relations[i+1:]...)
			if err := SaveProjectSettings(h.settingsPath, h.settings); err != nil {
				h.writeError(w, http.StatusInternalServerError, "保存失败: "+err.Error())
				return
			}
			h.writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
			return
		}
	}

	h.writeError(w, http.StatusNotFound, "关系不存在")
}

func (h *Handlers) PostSettingsAIGenerate(w http.ResponseWriter, r *http.Request) {
	h.writeError(w, http.StatusGone, "此功能已移至 LLM 对话中，请通过聊天让 AI 帮你生成设定")
}

func (h *Handlers) PostSettingsPolish(w http.ResponseWriter, r *http.Request) {
	h.writeError(w, http.StatusGone, "此功能已移至 LLM 对话中，请通过聊天让 AI 帮你润色")
}

func (h *Handlers) PostChapterPolish(w http.ResponseWriter, r *http.Request) {
	if !h.ensureProject(w) {
		return
	}

	polishSkills := GetEnabledSkillsByCategory(h.skills, h.cfg.SkillConfig, "polish")
	if len(polishSkills) == 0 {
		h.writeError(w, http.StatusBadRequest, "没有启用的润色技能，请先在技能管理页启用 polish 类技能")
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
			h.writeError(w, http.StatusBadRequest, "章节不存在")
			return
		}
	} else {
		chapterIdx = h.state.CurrentChapterIndex
		if chapterIdx < 0 || chapterIdx >= len(h.state.Chapters) {
			h.writeError(w, http.StatusBadRequest, "请指定章节编号")
			return
		}
	}

	ch := h.state.Chapters[chapterIdx]
	if ch.Content == "" {
		h.writeError(w, http.StatusBadRequest, "章节内容为空，无法润色")
		return
	}
	if ch.Status == StatusWriting {
		h.writeError(w, http.StatusBadRequest, "章节正在写作中，无法润色")
		return
	}

	if !h.tryStartTask() {
		h.writeError(w, http.StatusConflict, "有任务正在运行，请等待完成")
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
				h.logger.Warn("章节润色已取消")
			} else {
				h.logger.Error(fmt.Sprintf("章节润色失败: %v", err))
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
	if !h.ensureProject(w) {
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
	if !h.ensureProject(w) {
		return
	}
	if h.rejectIfTaskRunning(w) {
		return
	}

	var req struct {
		Roadmap        []RoadmapItem              `json:"roadmap"`
		ExecuteOptions *PostProcessExecuteOptions `json:"execute_options"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, "无效的JSON: "+err.Error())
		return
	}
	if req.Roadmap != nil {
		h.postprocess.Roadmap = req.Roadmap
	}
	if req.ExecuteOptions != nil {
		h.postprocess.ExecuteOptions = req.ExecuteOptions
	}
	if err := SavePostProcess(h.postprocessPath, h.postprocess); err != nil {
		h.writeError(w, http.StatusInternalServerError, "保存失败: "+err.Error())
		return
	}
	h.logger.PostProcessUpdate(h.postprocess)
	h.writeJSON(w, http.StatusOK, h.postProcessResponse())
}

// DeletePostProcess 清空全书优化报告与工单。
func (h *Handlers) DeletePostProcess(w http.ResponseWriter, r *http.Request) {
	if !h.ensureProject(w) {
		return
	}
	if h.rejectIfTaskRunning(w) {
		return
	}

	h.postprocess = &PostProcessState{
		ExecuteOptions: &PostProcessExecuteOptions{RunSmoothTransitionsFirst: true},
	}
	if err := SavePostProcess(h.postprocessPath, h.postprocess); err != nil {
		h.writeError(w, http.StatusInternalServerError, "清空失败: "+err.Error())
		return
	}
	h.logger.PostProcessUpdate(h.postprocess)
	h.writeJSON(w, http.StatusOK, h.postProcessResponse())
}

// PostPostProcessDiagnose 异步：全书诊断 + 一致性核查 + 生成路线图。
func (h *Handlers) PostPostProcessDiagnose(w http.ResponseWriter, r *http.Request) {
	if !h.ensureProject(w) {
		return
	}
	if !isBookFullyAccepted(h.state) {
		h.writeError(w, http.StatusBadRequest, "全书尚未完成（需所有章节已确认）")
		return
	}
	if !h.tryStartTask() {
		h.writeError(w, http.StatusConflict, "有任务正在运行，请等待完成")
		return
	}

	go func() {
		defer h.endTask()
		h.logger.TaskStart("postprocess_diagnose")
		ctx := h.taskCtx

		err := FullPostProcessAnalyzeAction(ctx, h.apiCfg, h.cfg, h.settings, h.state, h.postprocess, h.postprocessPath, h.logger)
		if err != nil {
			if ctx.Err() != nil {
				h.logger.Warn("全书优化分析已取消")
			} else {
				h.logger.Error(fmt.Sprintf("全书优化分析失败: %v", err))
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
	if !h.ensureProject(w) {
		return
	}
	if !isBookFullyAccepted(h.state) {
		h.writeError(w, http.StatusBadRequest, "全书尚未完成（需所有章节已确认）")
		return
	}
	if !h.tryStartTask() {
		h.writeError(w, http.StatusConflict, "有任务正在运行，请等待完成")
		return
	}

	go func() {
		defer h.endTask()
		h.logger.TaskStart("postprocess_consistency")
		ctx := h.taskCtx

		report, err := ConsistencyCheckBookAction(ctx, h.apiCfg, h.cfg, h.settings, h.state, h.logger)
		if err != nil {
			if ctx.Err() != nil {
				h.logger.Warn("全书一致性核查已取消")
			} else {
				h.logger.Error(fmt.Sprintf("全书一致性核查失败: %v", err))
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
	if !h.ensureProject(w) {
		return
	}
	if strings.TrimSpace(h.postprocess.DiagnosisReport) == "" && strings.TrimSpace(h.postprocess.ConsistencyReport) == "" {
		h.writeError(w, http.StatusBadRequest, "缺少诊断或核查报告，请先运行全书诊断")
		return
	}
	if !h.tryStartTask() {
		h.writeError(w, http.StatusConflict, "有任务正在运行，请等待完成")
		return
	}

	go func() {
		defer h.endTask()
		h.logger.TaskStart("postprocess_roadmap")
		ctx := h.taskCtx

		roadmap, err := BuildRoadmapAction(ctx, h.apiCfg, h.cfg, h.postprocess.DiagnosisReport, h.postprocess.ConsistencyReport, h.logger)
		if err != nil {
			if ctx.Err() != nil {
				h.logger.Warn("路线图生成已取消")
			} else {
				h.logger.Error(fmt.Sprintf("路线图生成失败: %v", err))
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
	if !h.ensureProject(w) {
		return
	}
	if !isBookFullyAccepted(h.state) {
		h.writeError(w, http.StatusBadRequest, "全书尚未完成（需所有章节已确认）")
		return
	}
	if len(h.postprocess.Roadmap) == 0 {
		h.writeError(w, http.StatusBadRequest, "没有可执行的优化工单")
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
		h.writeError(w, http.StatusBadRequest, "请至少勾选一条待执行的工单")
		return
	}

	if !h.tryStartTask() {
		h.writeError(w, http.StatusConflict, "有任务正在运行，请等待完成")
		return
	}

	go func() {
		defer h.endTask()
		h.logger.TaskStart("postprocess_execute")
		ctx := h.taskCtx

		err := ExecuteRoadmapAction(ctx, h.apiCfg, h.cfg, h.settings, h.state, h.postprocess, h.progressPath, h.postprocessPath, h.skills, h.logger)
		if err != nil {
			if ctx.Err() != nil {
				h.logger.Warn("全书优化执行已取消（已完成项不会丢失）")
			} else {
				h.logger.Error(fmt.Sprintf("全书优化执行失败: %v", err))
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
	if h.rejectIfTaskRunning(w) {
		return
	}
	id := r.PathValue("id")

	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, "无效的JSON: "+err.Error())
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
		h.writeError(w, http.StatusNotFound, "技能不存在")
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
		h.writeError(w, http.StatusInternalServerError, "保存配置失败: "+err.Error())
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]interface{}{"id": id, "enabled": req.Enabled})
}

func (h *Handlers) GetChatSessions(w http.ResponseWriter, r *http.Request) {
	idx, err := LoadChatSessions(h.sessionsDir)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "加载会话列表失败: "+err.Error())
		return
	}
	if idx == nil {
		idx = &ChatSessionIndex{}
	}
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

	if err := SaveChatSession(h.sessionsDir, session); err != nil {
		h.writeError(w, http.StatusInternalServerError, "创建会话失败: "+err.Error())
		return
	}

	h.writeJSON(w, http.StatusOK, session)
}

func (h *Handlers) GetChatSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	session, err := LoadChatSession(h.sessionsDir, id)
	if err != nil {
		h.writeError(w, http.StatusNotFound, "会话不存在")
		return
	}

	h.writeJSON(w, http.StatusOK, session)
}

func (h *Handlers) DeleteChatSession(w http.ResponseWriter, r *http.Request) {
	if h.rejectIfTaskRunning(w) {
		return
	}
	id := r.PathValue("id")

	if err := DeleteChatSession(h.sessionsDir, id); err != nil {
		h.writeError(w, http.StatusInternalServerError, "删除会话失败: "+err.Error())
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *Handlers) PostChatMessage(w http.ResponseWriter, r *http.Request) {
	if !h.tryStartTask() {
		h.writeError(w, http.StatusConflict, "有任务正在运行，请等待完成")
		return
	}

	sessionID := r.PathValue("id")

	var req struct {
		Content     string `json:"content"`
		ContextPage string `json:"context_page"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Content == "" {
		h.endTask()
		h.writeError(w, http.StatusBadRequest, "缺少 content 字段")
		return
	}

	session, err := LoadChatSession(h.sessionsDir, sessionID)
	if err != nil {
		h.endTask()
		h.writeError(w, http.StatusNotFound, "会话不存在")
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
		h.writeError(w, http.StatusInternalServerError, "保存会话失败: "+err.Error())
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
				history = append(history, AgentStep{Role: "tool", ToolResult: m.ToolResult})
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
			StartAsync: func(taskName string, fn func(goCtx context.Context)) {
				// 子任务必须计入 activeWork，否则 Agent 主循环结束后锁被释放，
				// 子任务仍在运行时新任务可并发进入，造成数据竞争。
				if !h.startChildWork() {
					h.logger.Warn(fmt.Sprintf("无法启动子任务 %s：主任务已结束", taskName))
					return
				}
				childCtx := h.taskCtx
				go func() {
					defer h.endTask()
					h.logger.TaskStart(taskName)
					fn(childCtx)
					h.logger.TaskEnd(taskName, true)
					h.broadcastProgress()
				}()
			},
		}

		reply, newHistory, err := RunAgentLoop(ctx, agentCtx, req.Content, history, 30)
		if err != nil {
			// 即使失败也保存已产生的对话步骤，避免上下文丢失
			saveAgentSteps(session, newHistory[len(history):])
			session.UpdatedAt = time.Now().Format(time.RFC3339)
			if saveErr := SaveChatSession(h.sessionsDir, session); saveErr != nil {
				h.logger.Warn(fmt.Sprintf("保存会话失败: %v", saveErr))
			}
			if ctx.Err() != nil {
				h.logger.Warn("助理对话已取消")
			} else {
				h.logger.Error(fmt.Sprintf("助理回复失败: %v", err))
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
			h.logger.Warn(fmt.Sprintf("保存会话失败: %v", err))
		}

		h.logger.ChatChunk(sessionID, reply)

		h.logger.Success("助理回复完成")
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
				Role:       "tool",
				ToolResult: step.ToolResult,
				Timestamp:  time.Now().Format(time.RFC3339),
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
