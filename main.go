package main

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	defaultPort = ":8090"
)

func main() {
	projectDir := "."
	if len(os.Args) > 1 {
		projectDir = os.Args[1]
	}

	absDir, err := filepath.Abs(projectDir)
	if err != nil {
		fmt.Printf(" [错误] 无法解析项目目录: %v\n", err)
		os.Exit(1)
	}
	projectDir = absDir

	if info, err := os.Stat(projectDir); err != nil || !info.IsDir() {
		fmt.Printf(" [错误] 项目目录不存在或不是目录: %s\n", projectDir)
		os.Exit(1)
	}

	progressPath := filepath.Join(projectDir, "progress.json")
	configPath := filepath.Join(projectDir, "config.json")
	settingsPath := filepath.Join(projectDir, "settings.json")
	sessionsDir := filepath.Join(projectDir, "sessions")

	apiCfgPath := filepath.Join(projectDir, "api.json")
	if _, err := os.Stat(apiCfgPath); os.IsNotExist(err) {
		exePath, err := os.Executable()
		if err == nil {
			globalPath := filepath.Join(filepath.Dir(exePath), "api.json")
			if _, err := os.Stat(globalPath); err == nil {
				apiCfgPath = globalPath
			}
		}
	}

	os.MkdirAll(sessionsDir, 0755)

	fmt.Printf(" [系统] 项目目录: %s\n", projectDir)

	apiCfg, err := LoadAPIConfig(apiCfgPath)
	if err != nil {
		fmt.Printf(" [错误] 加载API配置失败: %v\n", err)
		os.Exit(1)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		fmt.Printf(" [错误] 加载配置失败: %v\n", err)
		os.Exit(1)
	}

	if apiCfg.BaseURL == "" || apiCfg.Model == "" {
		fmt.Println(" [系统] 检测到空白API配置，已自动生成 api.json")
		fmt.Println(" [系统] 请通过 Web UI 配置 API 地址和模型后再使用")
	}

	state, err := LoadProgress(progressPath)
	if err != nil {
		fmt.Printf(" [错误] 加载进度文件失败: %v\n", err)
		os.Exit(1)
	}

	if state == nil {
		fmt.Println(" [系统] 未检测到历史进度，开始全新的创作流程...")
		state = &Progress{
			Phase: "outline",
		}
	} else {
		fmt.Printf(" [系统] 检测到历史进度，当前阶段: %s\n", state.Phase)
	}

	settings, err := LoadProjectSettings(settingsPath)
	if err != nil {
		fmt.Printf(" [错误] 加载设定失败: %v\n", err)
		os.Exit(1)
	}

	skills := LoadAllSkills(cfg, projectDir)

	port := os.Getenv("PORT")
	if port == "" {
		port = defaultPort
	} else {
		port = ":" + port
	}

	logger := NewLogBroadcaster()
	defer logger.Close()

	startWebServer(apiCfg, apiCfgPath, cfg, configPath, state, progressPath, settings, settingsPath, skills, sessionsDir, logger, port, projectDir)
}
