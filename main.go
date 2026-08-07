package main

import (
	"embed"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"

	"showmethestory/internal/config"
	"showmethestory/internal/devlog"
	"showmethestory/internal/httpapi"
	"showmethestory/internal/llm"
	"showmethestory/internal/sse"
)

//go:embed frontend/dist
var staticFiles embed.FS

// version is injected by CI via -ldflags "-X main.version=...".
var version = "dev"

const defaultPort = ":48090"

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

	// Load API config (global, shared across projects; always in progDir)
	apiCfgPath := filepath.Join(progDir, "api.json")
	apiCfg, err := config.LoadAPIConfig(apiCfgPath)
	if err != nil {
		fmt.Printf(" [错误] 加载API配置失败: %v\n", err)
		os.Exit(1)
	}
	llm.EnsureContextBudget(apiCfg)

	if apiCfg.BaseURL == "" || apiCfg.Model == "" {
		fmt.Println(" [系统] 检测到空白API配置，已自动生成 api.json")
		fmt.Println(" [系统] 请通过 Web UI 配置 API 地址和模型后再使用")
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = defaultPort
	} else {
		port = ":" + port
	}

	logger := sse.NewLogBroadcaster()
	defer logger.Close()

	devlog.Init(progDir, version)

	fmt.Printf(" [系统] 版本: %s\n", version)
	fmt.Printf(" [系统] 程序目录: %s\n", progDir)
	fmt.Printf(" [系统] 项目目录: %s\n", storysDir)
	if devlog.Enabled() {
		fmt.Printf(" [系统] 开发日志: %s\n", filepath.Join(progDir, "dev.log"))
	}

	staticFS, err := fs.Sub(staticFiles, "frontend/dist")
	if err != nil {
		log.Fatalf("嵌入静态文件失败: %v", err)
	}

	httpapi.StartWebServer(apiCfg, apiCfgPath, logger, port, progDir, version, staticFS)
}
