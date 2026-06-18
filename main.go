package main

import (
	"fmt"
	"os"
	"path/filepath"
)

var (
	version = "dev"
)

const (
	defaultPort = ":48090"
)

func main() {
	// Determine program directory (progDir)
	// Priority: os.Args[1] if it's a valid existing directory, otherwise use cwd
	progDir := ""

	if len(os.Args) > 1 {
		absDir, err := filepath.Abs(os.Args[1])
		if err == nil {
			if info, err := os.Stat(absDir); err == nil && info.IsDir() {
				progDir = absDir
			}
		}
	}

	if progDir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			fmt.Printf(" [错误] 无法获取当前目录: %v\n", err)
			os.Exit(1)
		}
		progDir = cwd
	}

	// Create storys directory
	storysDir := filepath.Join(progDir, "storys")
	os.MkdirAll(storysDir, 0755)

	// API config: always in progDir
	apiCfgPath := filepath.Join(progDir, "api.json")

	// Load API config (global, shared across projects)
	apiCfg, err := LoadAPIConfig(apiCfgPath)
	if err != nil {
		fmt.Printf(" [错误] 加载API配置失败: %v\n", err)
		os.Exit(1)
	}

	if apiCfg.BaseURL == "" || apiCfg.Model == "" {
		fmt.Println(" [系统] 检测到空白API配置，已自动生成 api.json")
		fmt.Println(" [系统] 请通过 Web UI 配置 API 地址和模型后再使用")
	}

	// Start with no project selected
	cfg := DefaultConfig()
	state := &Progress{Phase: "outline"}
	settings := &ProjectSettings{}
	skills := LoadAllSkills(cfg, progDir)
	sessionsDir := filepath.Join(progDir, "sessions")
	os.MkdirAll(sessionsDir, 0755)

	port := os.Getenv("PORT")
	if port == "" {
		port = defaultPort
	} else {
		port = ":" + port
	}

	logger := NewLogBroadcaster()
	defer logger.Close()

	fmt.Printf(" [系统] 版本: %s\n", version)
	fmt.Printf(" [系统] 程序目录: %s\n", progDir)
	fmt.Printf(" [系统] 项目目录: %s\n", storysDir)

	startWebServer(apiCfg, apiCfgPath, cfg, state, settings, skills, sessionsDir, logger, port, progDir, version)
}
