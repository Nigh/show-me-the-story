package main

import (
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"time"
)

//go:embed static
var staticFiles embed.FS

func startWebServer(apiCfg *APIConfig, apiCfgPath string, cfg *Config, cfgPath string, state *Progress, progressPath string, settings *ProjectSettings, settingsPath string, skills []Skill, sessionsDir string, logger *LogBroadcaster, port string, projectDir string) {
	h := NewHandlers(apiCfg, apiCfgPath, cfg, cfgPath, state, progressPath, settings, settingsPath, skills, sessionsDir, logger, projectDir)

	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/config/api", h.GetAPIConfig)
	mux.HandleFunc("PUT /api/config/api", h.PutAPIConfig)
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
	mux.HandleFunc("POST /api/chapter/polish", h.PostChapterPolish)
	mux.HandleFunc("DELETE /api/chapter", h.DeleteChapter)
	mux.HandleFunc("DELETE /api/chapters/from/{num}", h.DeleteChaptersFrom)
	mux.HandleFunc("DELETE /api/outline", h.DeleteOutline)

	mux.HandleFunc("POST /api/settings/reconcile", h.PostSettingsReconcile)
	mux.HandleFunc("GET /api/settings", h.GetSettings)
	mux.HandleFunc("POST /api/settings/ai-generate", h.PostSettingsAIGenerate)

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

	staticFS, err := fs.Sub(staticFiles, "static")
	if err != nil {
		log.Fatalf("嵌入静态文件失败: %v", err)
	}

	fileServer := http.FileServer(http.FS(staticFS))

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || r.URL.Path == "/index.html" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			data, err := staticFiles.ReadFile("static/index.html")
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
	fmt.Printf(" [系统] 项目目录: %s\n", projectDir)
	fmt.Printf(" [系统] 当前阶段: %s\n", state.Phase)
	if state.Title != "" {
		fmt.Printf(" [系统] 小说标题: 《%s》\n", state.Title)
	}

	go openBrowser(fmt.Sprintf("http://localhost%s", port))

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fmt.Fprintf(os.Stderr, " [错误] 服务器启动失败: %v\n", err)
		os.Exit(1)
	}
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
