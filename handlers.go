package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

type Handlers struct {
	apiCfg       *APIConfig
	apiCfgPath   string
	cfg          *Config
	cfgPath      string
	state        *Progress
	progressPath string
	settings     *ProjectSettings
	settingsPath string
	skills       []Skill
	sessionsDir  string
	logger       *LogBroadcaster
	taskMu       sync.Mutex
	taskRunning  bool
	projectDir   string

	pendingContinueContent string
}

func NewHandlers(apiCfg *APIConfig, apiCfgPath string, cfg *Config, cfgPath string, state *Progress, progressPath string, settings *ProjectSettings, settingsPath string, skills []Skill, sessionsDir string, logger *LogBroadcaster, projectDir string) *Handlers {
	return &Handlers{
		apiCfg:       apiCfg,
		apiCfgPath:   apiCfgPath,
		cfg:          cfg,
		cfgPath:      cfgPath,
		state:        state,
		progressPath: progressPath,
		settings:     settings,
		settingsPath: settingsPath,
		skills:       skills,
		sessionsDir:  sessionsDir,
		logger:       logger,
		projectDir:   projectDir,
	}
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
	if h.taskRunning {
		return false
	}
	h.taskRunning = true
	return true
}

func (h *Handlers) endTask() {
	h.taskMu.Lock()
	h.taskRunning = false
	h.taskMu.Unlock()
}

func (h *Handlers) isTaskRunning() bool {
	h.taskMu.Lock()
	defer h.taskMu.Unlock()
	return h.taskRunning
}

func (h *Handlers) GetAPIConfig(w http.ResponseWriter, r *http.Request) {
	h.writeJSON(w, http.StatusOK, h.apiCfg)
}

func (h *Handlers) PutAPIConfig(w http.ResponseWriter, r *http.Request) {
	var newCfg APIConfig
	if err := json.NewDecoder(r.Body).Decode(&newCfg); err != nil {
		h.writeError(w, http.StatusBadRequest, "无效的JSON: "+err.Error())
		return
	}

	if newCfg.HTTPTimeoutSeconds <= 0 {
		newCfg.HTTPTimeoutSeconds = 300
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
	if !h.tryStartTask() {
		h.writeError(w, http.StatusConflict, "有任务正在运行，请等待完成")
		return
	}

	go func() {
		h.logger.TaskStart("outline_generation")

		h.logger.Info("正在生成小说大纲...")
		err := GenerateOutlineAction(h.apiCfg, h.cfg, h.state, h.progressPath, h.logger)

		if err != nil {
			h.endTask()
			h.logger.Error(fmt.Sprintf("大纲生成失败: %v", err))
			h.logger.TaskEnd("outline_generation", false)
			return
		}

		h.endTask()
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
		h.logger.TaskStart("outline_revision")

		h.logger.Info("正在根据意见修订大纲...")
		err := ReviseOutlineAction(h.apiCfg, h.cfg, h.state, h.progressPath, body.Feedback, h.logger)

		if err != nil {
			h.endTask()
			h.logger.Error(fmt.Sprintf("大纲修订失败: %v", err))
			h.logger.TaskEnd("outline_revision", false)
			return
		}

		h.endTask()
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
		h.logger.TaskStart("chapter_generation")

		chIdx := h.state.CurrentChapterIndex
		chTitle := ""
		if chIdx < len(h.state.Chapters) {
			chTitle = h.state.Chapters[chIdx].Title
		}

		h.logger.Info(fmt.Sprintf("正在创作第 %d 章...", chIdx+1))
		err := GenerateChapterAction(h.apiCfg, h.cfg, h.state, h.progressPath, h.settings, h.logger)

		if err != nil {
			h.endTask()
			h.logger.Error(fmt.Sprintf("章节创作失败: %v", err))
			h.logger.TaskEnd("chapter_generation", false)
			return
		}

		h.endTask()
		h.logger.Success(fmt.Sprintf("第 %d 章《%s》创作完成！", chIdx+1, chTitle))
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
		h.logger.TaskStart("chapter_revision")

		h.logger.Info("正在根据意见修改当前章节...")
		err := ReviseChapterAction(h.apiCfg, h.cfg, h.state, h.progressPath, body.Feedback, h.logger)

		if err != nil {
			h.endTask()
			h.logger.Error(fmt.Sprintf("章节修订失败: %v", err))
			h.logger.TaskEnd("chapter_revision", false)
			return
		}

		h.endTask()
		h.logger.Success("章节已修订。")
		h.logger.TaskEnd("chapter_revision", true)
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

	mdFile := fmt.Sprintf("Chapter_%02d.md", ch.Num)
	deleteFile(mdFile)

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
	h.state.CoreRequirements = ""
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
		h.logger.TaskStart("settings_reconciliation")

		h.logger.Info("正在协调新设定与已有内容...")
		err := ReconcileSettingsAction(h.apiCfg, h.cfg, h.state, body, h.progressPath, h.cfgPath, h.logger)

		if err != nil {
			h.endTask()
			h.logger.Error(fmt.Sprintf("设定协调失败: %v", err))
			h.logger.TaskEnd("settings_reconciliation", false)
			return
		}

		h.endTask()
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
		mdFile := fmt.Sprintf("Chapter_%02d.md", h.state.Chapters[i].Num)
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
		"phase":            h.state.Phase,
		"title":            h.state.Title,
		"current_chapter":  h.state.CurrentChapterIndex,
		"total_chapters":   total,
		"accepted_chapters": accepted,
		"percent":          pct,
		"is_task_running":  h.isTaskRunning(),
	})
}

func (h *Handlers) GetStatus(w http.ResponseWriter, r *http.Request) {
	h.writeJSON(w, http.StatusOK, map[string]interface{}{
		"phase":           h.state.Phase,
		"title":           h.state.Title,
		"total_chapters":  len(h.state.Chapters),
		"is_task_running": h.isTaskRunning(),
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
		h.logger.TaskStart("foreshadow_suggest")

		h.logger.Info("正在分析大纲，设计伏笔方案...")
		suggestions, err := SuggestForeshadows(h.apiCfg, h.cfg, h.state, h.logger)

		if err != nil {
			h.endTask()
			h.logger.Error(fmt.Sprintf("伏笔建议生成失败: %v", err))
			h.logger.TaskEnd("foreshadow_suggest", false)
			return
		}

		h.endTask()
		h.logger.Success(fmt.Sprintf("伏笔建议生成完成，共 %d 条", len(suggestions)))
		h.logger.TaskEnd("foreshadow_suggest", true)
		h.logger.ForeshadowSuggestions(suggestions)
	}()

	h.writeJSON(w, http.StatusAccepted, map[string]string{"status": "started"})
}

func (h *Handlers) PostForeshadow(w http.ResponseWriter, r *http.Request) {
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
	idStr := r.PathValue("id")
	var id int
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		h.writeError(w, http.StatusBadRequest, "无效的伏笔ID")
		return
	}

	var req struct {
		Name          string          `json:"name"`
		Description   string          `json:"description"`
		PlantChapter  int             `json:"plant_chapter"`
		TargetChapter int             `json:"target_chapter"`
		Status        ForeshadowStatus `json:"status"`
		Resolution    string          `json:"resolution"`
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
		h.logger.TaskStart("continue_analysis")

		h.logger.Info("正在分析已有内容...")
		analysis, err := AnalyzeExistingContent(h.apiCfg, h.cfg, body.Content)

		if err != nil {
			h.endTask()
			h.logger.Error(fmt.Sprintf("内容分析失败: %v", err))
			h.logger.TaskEnd("continue_analysis", false)
			return
		}

		h.pendingContinueContent = body.Content

		h.endTask()
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
		h.logger.TaskStart("continuation_outline")

		h.logger.Info("正在生成续写大纲...")
		err := GenerateContinuationOutline(h.apiCfg, h.cfg, h.state, body.ChapterCount, h.progressPath, h.logger)

		if err != nil {
			h.endTask()
			h.logger.Error(fmt.Sprintf("续写大纲生成失败: %v", err))
			h.logger.TaskEnd("continuation_outline", false)
			return
		}

		h.endTask()
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
	if !h.tryStartTask() {
		h.writeError(w, http.StatusConflict, "有任务正在运行，请等待完成")
		return
	}

	go func() {
		h.logger.TaskStart("ai_settings_generate")

		h.logger.Info("正在 AI 自动生成设定...")
		err := h.aiGenerateSettings()

		if err != nil {
			h.endTask()
			h.logger.Error(fmt.Sprintf("AI 设定生成失败: %v", err))
			h.logger.TaskEnd("ai_settings_generate", false)
			return
		}

		h.endTask()
		h.logger.Success("AI 设定生成完成！")
		h.logger.TaskEnd("ai_settings_generate", true)
	}()

	h.writeJSON(w, http.StatusAccepted, map[string]string{"status": "started"})
}

func (h *Handlers) aiGenerateSettings() error {
	outline := ""
	for _, ch := range h.state.Chapters {
		outline += fmt.Sprintf("第%d章《%s》: %s\n", ch.Num, ch.Title, ch.Outline)
	}

	if outline == "" {
		outline = "暂无大纲"
	}

	snapshot := h.state.StoryConfigSnapshot
	if snapshot == nil {
		snapshot = &h.cfg.Story
	}

	userPrompt := fmt.Sprintf(`请根据以下小说信息，生成详细的角色设定和世界观设定。

小说标题: 《%s》
故事类型: %s
写作风格: %s
角色设定（原文）: %s
世界观设定（原文）: %s

大纲:
%s

请以JSON格式返回：
{
  "characters": [
    {
      "name": "角色名",
      "age": "年龄",
      "appearance": "外貌描述",
      "personality": "性格特点",
      "background": "背景故事",
      "motivation": "核心动机",
      "abilities": "能力/技能",
      "notes": "其他备注"
    }
  ],
  "worldview": [
    {
      "category": "geography/faction/rule/history/other",
      "name": "名称",
      "description": "详细描述",
      "tags": "相关标签"
    }
  ],
  "organizations": [
    {
      "name": "组织名",
      "type": "family/sect/nation/guild/other",
      "description": "组织描述",
      "members": []
    }
  ]
}

注意：
1. 角色应覆盖大纲中的主要人物
2. 世界观应涵盖故事所需的重要设定
3. 组织应反映故事中的势力结构
4. 请严格以JSON格式输出`,
		h.state.Title, snapshot.Type, snapshot.WritingStyle,
		snapshot.CharacterSetting, snapshot.WorldSetting, outline)

	systemPrompt := "你是一位专业的小说设定生成师。请严格按照要求的JSON格式输出，不要添加任何额外文字或markdown代码块标记。"

	rawResp := CallAPIWithRetryLog(h.apiCfg, systemPrompt, userPrompt, h.logger)
	rawResp = cleanJSONResponse(rawResp)

	var resp struct {
		Characters    []Character      `json:"characters"`
		Worldview     []WorldviewEntry `json:"worldview"`
		Organizations []Organization   `json:"organizations"`
	}
	if err := json.Unmarshal([]byte(rawResp), &resp); err != nil {
		return fmt.Errorf("解析AI生成结果失败: %w", err)
	}

	for i := range resp.Characters {
		resp.Characters[i].ID = h.settings.nextCharacterID()
		h.settings.Characters = append(h.settings.Characters, resp.Characters[i])
	}

	for i := range resp.Worldview {
		resp.Worldview[i].ID = h.settings.nextWorldviewID()
		h.settings.Worldview = append(h.settings.Worldview, resp.Worldview[i])
	}

	for i := range resp.Organizations {
		resp.Organizations[i].ID = h.settings.nextOrganizationID()
		h.settings.Organizations = append(h.settings.Organizations, resp.Organizations[i])
	}

	if err := SaveProjectSettings(h.settingsPath, h.settings); err != nil {
		return fmt.Errorf("保存设定失败: %w", err)
	}

	h.logger.Info(fmt.Sprintf("已生成 %d 个角色、%d 条世界观、%d 个组织",
		len(resp.Characters), len(resp.Worldview), len(resp.Organizations)))

	return nil
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

func (h *Handlers) PostChapterPolish(w http.ResponseWriter, r *http.Request) {
	if !h.tryStartTask() {
		h.writeError(w, http.StatusConflict, "有任务正在运行，请等待完成")
		return
	}

	polishSkills := GetEnabledSkillsByCategory(h.skills, h.cfg.SkillConfig, "polish")
	if len(polishSkills) == 0 {
		h.endTask()
		h.writeError(w, http.StatusBadRequest, "没有启用的润色技能，请先在技能管理页启用 polish 类技能")
		return
	}

	go func() {
		h.logger.TaskStart("chapter_polish")

		chIdx := h.state.CurrentChapterIndex
		if chIdx >= len(h.state.Chapters) {
			chIdx = len(h.state.Chapters) - 1
		}
		if chIdx < 0 || chIdx >= len(h.state.Chapters) {
			h.endTask()
			h.logger.Error("没有可润色的章节")
			h.logger.TaskEnd("chapter_polish", false)
			return
		}

		if h.state.Chapters[chIdx].Status != StatusReview && chIdx > 0 {
			chIdx = chIdx - 1
		}
		if h.state.Chapters[chIdx].Status != StatusReview {
			h.endTask()
			h.logger.Error("当前没有处于审核状态的章节可润色")
			h.logger.TaskEnd("chapter_polish", false)
			return
		}

		h.logger.Info(fmt.Sprintf("正在对第 %d 章进行去AI味处理...", h.state.Chapters[chIdx].Num))
		err := PolishChapterAction(h.apiCfg, h.cfg, h.state, chIdx, polishSkills, h.progressPath, h.logger)

		if err != nil {
			h.endTask()
			h.logger.Error(fmt.Sprintf("去AI味失败: %v", err))
			h.logger.TaskEnd("chapter_polish", false)
			return
		}

		h.endTask()
		h.logger.Success(fmt.Sprintf("第 %d 章去AI味完成！", h.state.Chapters[chIdx].Num))
		h.logger.TaskEnd("chapter_polish", true)
		h.broadcastProgress()
	}()

	h.writeJSON(w, http.StatusAccepted, map[string]string{"status": "started"})
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
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Content == "" {
		h.endTask()
		h.writeError(w, http.StatusBadRequest, "缺少 content 字段")
		return
	}

	go func() {
		h.logger.TaskStart("chat_message")

		session, err := LoadChatSession(h.sessionsDir, sessionID)
		if err != nil {
			h.endTask()
			h.logger.Error(fmt.Sprintf("加载会话失败: %v", err))
			h.logger.TaskEnd("chat_message", false)
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

		ctx := &AgentContext{
			APICfg:       h.apiCfg,
			Settings:     h.settings,
			SettingsPath: h.settingsPath,
			State:        h.state,
			Config:       h.cfg,
			Skills:       h.skills,
			Logger:       h.logger,
		}

		reply, newHistory, err := RunAgentLoop(ctx, req.Content, history, 10)
		if err != nil {
			h.endTask()
			h.logger.Error(fmt.Sprintf("助理回复失败: %v", err))
			h.logger.TaskEnd("chat_message", false)
			return
		}

		for _, step := range newHistory[len(history):] {
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

		h.endTask()
		h.logger.Success("助理回复完成")
		h.logger.TaskEnd("chat_message", true)
	}()

	h.writeJSON(w, http.StatusAccepted, map[string]string{"status": "started"})
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
