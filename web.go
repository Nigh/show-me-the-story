package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

//go:embed frontend/dist
var staticFiles embed.FS

func startWebServer(apiCfg *APIConfig, apiCfgPath string, cfg *Config, state *Progress, settings *ProjectSettings, skills []Skill, sessionsDir string, logger *LogBroadcaster, port string, progDir string) {
	h := NewHandlers(apiCfg, apiCfgPath, logger, progDir)

	mux := http.NewServeMux()

	// Project management endpoints
	mux.HandleFunc("GET /api/projects", h.GetProjects)
	mux.HandleFunc("POST /api/projects", h.PostProject)
	mux.HandleFunc("GET /api/projects/current", h.GetProjectCurrent)
	mux.HandleFunc("POST /api/projects/select", h.PostProjectSelect)
	mux.HandleFunc("DELETE /api/projects/{name}", h.DeleteProject)

	// API config (global, always available)
	mux.HandleFunc("GET /api/config/api", h.GetAPIConfig)
	mux.HandleFunc("PUT /api/config/api", h.PutAPIConfig)

	// Project-scoped endpoints (require project selection)
	mux.HandleFunc("GET /api/config", h.GetConfig)
	mux.HandleFunc("PUT /api/config", h.PutConfig)
	mux.HandleFunc("GET /api/progress", h.GetProgress)
	mux.HandleFunc("DELETE /api/progress", h.DeleteProgress)
	mux.HandleFunc("GET /api/status", h.GetStatus)

	mux.HandleFunc("POST /api/outline/generate", h.PostOutlineGenerate)
	mux.HandleFunc("POST /api/outline/confirm", h.PostOutlineConfirm)
	mux.HandleFunc("POST /api/outline/revise", h.PostOutlineRevise)
	mux.HandleFunc("POST /api/outline/generate-continuation", h.PostOutlineGenerateContinuation)
	mux.HandleFunc("PUT /api/outline/{num}", h.PutChapterOutline)

	mux.HandleFunc("POST /api/chapter/generate", h.PostChapterGenerate)
	mux.HandleFunc("POST /api/chapter/confirm", h.PostChapterConfirm)
	mux.HandleFunc("POST /api/chapter/revise", h.PostChapterRevise)
	mux.HandleFunc("POST /api/chapter/revise/{num}", h.PostChapterReviseSpecific)
	mux.HandleFunc("POST /api/chapter/polish", h.PostChapterPolish)
	mux.HandleFunc("POST /api/chapters/smooth-transitions", h.PostChaptersSmoothTransitions)

	mux.HandleFunc("GET /api/postprocess", h.GetPostProcess)
	mux.HandleFunc("DELETE /api/postprocess", h.DeletePostProcess)
	mux.HandleFunc("PUT /api/postprocess/roadmap", h.PutPostProcessRoadmap)
	mux.HandleFunc("POST /api/postprocess/diagnose", h.PostPostProcessDiagnose)
	mux.HandleFunc("POST /api/postprocess/consistency", h.PostPostProcessConsistency)
	mux.HandleFunc("POST /api/postprocess/roadmap", h.PostPostProcessRoadmap)
	mux.HandleFunc("POST /api/postprocess/execute", h.PostPostProcessExecute)
	mux.HandleFunc("DELETE /api/chapter", h.DeleteChapter)
	mux.HandleFunc("DELETE /api/chapters/from/{num}", h.DeleteChaptersFrom)
	mux.HandleFunc("DELETE /api/outline", h.DeleteOutline)

	mux.HandleFunc("POST /api/task/stop", h.PostTaskStop)

	mux.HandleFunc("GET /api/autoconfirm", h.GetAutoConfirm)
	mux.HandleFunc("PUT /api/autoconfirm", h.PutAutoConfirm)

	mux.HandleFunc("POST /api/settings/reconcile", h.PostSettingsReconcile)
	mux.HandleFunc("GET /api/settings", h.GetSettings)
	mux.HandleFunc("POST /api/settings/ai-generate", h.PostSettingsAIGenerate)
	mux.HandleFunc("POST /api/settings/polish", h.PostSettingsPolish)

	mux.HandleFunc("POST /api/characters", h.PostCharacter)
	mux.HandleFunc("PUT /api/characters/{id}", h.PutCharacter)
	mux.HandleFunc("DELETE /api/characters/{id}", h.DeleteCharacter)

	mux.HandleFunc("POST /api/worldview", h.PostWorldview)
	mux.HandleFunc("PUT /api/worldview/{id}", h.PutWorldview)
	mux.HandleFunc("DELETE /api/worldview/{id}", h.DeleteWorldview)

	mux.HandleFunc("POST /api/organizations", h.PostOrganization)
	mux.HandleFunc("PUT /api/organizations/{id}", h.PutOrganization)
	mux.HandleFunc("DELETE /api/organizations/{id}", h.DeleteOrganization)

	mux.HandleFunc("POST /api/relations", h.PostRelation)
	mux.HandleFunc("PUT /api/relations/{id}", h.PutRelation)
	mux.HandleFunc("DELETE /api/relations/{id}", h.DeleteRelation)

	mux.HandleFunc("GET /api/foreshadows", h.GetForeshadows)
	mux.HandleFunc("POST /api/foreshadows/suggest", h.PostForeshadowsSuggest)
	mux.HandleFunc("POST /api/foreshadows/confirm", h.PostForeshadowsConfirm)
	mux.HandleFunc("POST /api/foreshadows", h.PostForeshadow)
	mux.HandleFunc("PUT /api/foreshadows/{id}", h.PutForeshadow)
	mux.HandleFunc("DELETE /api/foreshadows/{id}", h.DeleteForeshadow)

	mux.HandleFunc("POST /api/continue/import", h.PostContinueImport)
	mux.HandleFunc("POST /api/continue/confirm", h.PostContinueConfirm)

	mux.HandleFunc("GET /api/skills", h.GetSkills)
	mux.HandleFunc("PUT /api/skills/{id}/toggle", h.PutSkillToggle)

	mux.HandleFunc("GET /api/chat/sessions", h.GetChatSessions)
	mux.HandleFunc("POST /api/chat/sessions", h.PostChatSession)
	mux.HandleFunc("GET /api/chat/sessions/{id}", h.GetChatSession)
	mux.HandleFunc("DELETE /api/chat/sessions/{id}", h.DeleteChatSession)
	mux.HandleFunc("POST /api/chat/sessions/{id}/messages", h.PostChatMessage)

	mux.HandleFunc("GET /api/events", h.SSEHandler)

	staticFS, err := fs.Sub(staticFiles, "frontend/dist")
	if err != nil {
		log.Fatalf("嵌入静态文件失败: %v", err)
	}

	fileServer := http.FileServer(http.FS(staticFS))

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || r.URL.Path == "/index.html" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			data, err := staticFiles.ReadFile("frontend/dist/index.html")
			if err != nil {
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}
			w.Write(data)
			return
		}
		fileServer.ServeHTTP(w, r)
	})

	handler := corsMiddleware(loggingMiddleware(mux))

	srv := &http.Server{
		Addr:         port,
		Handler:      handler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 0,
		IdleTimeout:  120 * time.Second,
	}

	fmt.Printf(" [系统] AI 小说生成器 Web UI 启动中...\n")
	fmt.Printf(" [系统] 访问地址: http://localhost%s\n", port)
	fmt.Printf(" [系统] 程序目录: %s\n", progDir)
	fmt.Printf(" [系统] 项目目录: %s\n", filepath.Join(progDir, "storys"))

	go openBrowser(fmt.Sprintf("http://localhost%s", port))

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fmt.Fprintf(os.Stderr, " [错误] 服务器启动失败: %v\n", err)
		os.Exit(1)
	}
}

// Project management handlers

func (h *Handlers) GetProjects(w http.ResponseWriter, r *http.Request) {
	storysDir := h.storysDir()
	entries, err := os.ReadDir(storysDir)
	if err != nil {
		h.writeJSON(w, http.StatusOK, []map[string]string{})
		return
	}

	projects := make([]map[string]string, 0)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		projectDir := filepath.Join(storysDir, name)

		// Get progress info if available
		phase := ""
		title := ""
		progressPath := filepath.Join(projectDir, "progress.json")
		if data, err := os.ReadFile(progressPath); err == nil {
			var p Progress
			if json.Unmarshal(data, &p) == nil {
				phase = p.Phase
				title = p.Title
			}
		}

		info := map[string]string{
			"name":  name,
			"phase": phase,
			"title": title,
		}

		// Get mod time for sorting
		if stat, err := os.Stat(projectDir); err == nil {
			info["updated_at"] = stat.ModTime().Format(time.RFC3339)
		}

		projects = append(projects, info)
	}

	// Sort by updated_at descending
	sort.Slice(projects, func(i, j int) bool {
		return projects[i]["updated_at"] > projects[j]["updated_at"]
	})

	h.writeJSON(w, http.StatusOK, projects)
}

func (h *Handlers) PostProject(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Name) == "" {
		h.writeError(w, http.StatusBadRequest, "缺少项目名称")
		return
	}

	name := strings.TrimSpace(req.Name)

	// Validate project name
	for _, c := range name {
		if c == '/' || c == '\\' || c == ':' || c == '*' || c == '?' || c == '"' || c == '<' || c == '>' || c == '|' {
			h.writeError(w, http.StatusBadRequest, "项目名称包含非法字符")
			return
		}
	}

	projectDir := filepath.Join(h.storysDir(), name)
	if _, err := os.Stat(projectDir); err == nil {
		h.writeError(w, http.StatusConflict, "项目已存在")
		return
	}

	// Create project directory and initialize
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		h.writeError(w, http.StatusInternalServerError, "创建项目目录失败: "+err.Error())
		return
	}

	sessionsDir := filepath.Join(projectDir, "sessions")
	os.MkdirAll(sessionsDir, 0755)

	// Initialize default config
	cfg := DefaultConfig()
	if err := saveConfig(filepath.Join(projectDir, "config.json"), cfg); err != nil {
		h.writeError(w, http.StatusInternalServerError, "初始化项目配置失败: "+err.Error())
		return
	}

	h.logger.Info(fmt.Sprintf("项目「%s」创建成功", name))
	h.writeJSON(w, http.StatusOK, map[string]string{"name": name})
}

func (h *Handlers) GetProjectCurrent(w http.ResponseWriter, r *http.Request) {
	h.projectMu.RLock()
	defer h.projectMu.RUnlock()
	h.writeJSON(w, http.StatusOK, map[string]string{"name": h.projectName})
}

func (h *Handlers) PostProjectSelect(w http.ResponseWriter, r *http.Request) {
	if h.isTaskRunning() {
		h.writeError(w, http.StatusConflict, "有任务正在运行，请等待完成")
		return
	}

	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		h.writeError(w, http.StatusBadRequest, "缺少项目名称")
		return
	}

	if err := h.switchProject(req.Name); err != nil {
		h.writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]string{"name": h.projectName})
}

func (h *Handlers) DeleteProject(w http.ResponseWriter, r *http.Request) {
	if h.isTaskRunning() {
		h.writeError(w, http.StatusConflict, "有任务正在运行，无法删除项目")
		return
	}

	name := r.PathValue("name")
	if name == "" {
		h.writeError(w, http.StatusBadRequest, "缺少项目名称")
		return
	}

	h.projectMu.RLock()
	currentProject := h.projectName
	h.projectMu.RUnlock()

	if name == currentProject {
		h.writeError(w, http.StatusConflict, "不能删除当前正在使用的项目")
		return
	}

	projectDir := filepath.Join(h.storysDir(), name)
	if _, err := os.Stat(projectDir); os.IsNotExist(err) {
		h.writeError(w, http.StatusNotFound, "项目不存在")
		return
	}

	if err := os.RemoveAll(projectDir); err != nil {
		h.writeError(w, http.StatusInternalServerError, "删除项目失败: "+err.Error())
		return
	}

	h.logger.Info(fmt.Sprintf("项目「%s」已删除", name))
	h.writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/events" {
			log.Printf(" %s %s", r.Method, r.URL.Path)
		}
		next.ServeHTTP(w, r)
	})
}

func openBrowser(url string) {
	time.Sleep(500 * time.Millisecond)
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	cmd.Start()
}
