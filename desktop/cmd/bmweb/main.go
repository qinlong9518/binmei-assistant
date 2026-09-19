//go:build windows

// 彬煤助手 v1.13.0 —— Wails 版（HTML/CSS 界面 + WebView2 渲染）
// 业务逻辑与 walk 版（cmd/bmwin）完全同源：
//   - 配置/账号管理：config.go
//   - 自动答题引擎：engine.go
//   - 在线更新：updater.go
//   - 安装/卸载：installer.go
package main

import (
	"context"
	"embed"
	"fmt"
	"log"
	"os"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	// 卸载模式（无窗口）
	if len(os.Args) > 1 && os.Args[1] == "/uninstall" {
		runUninstaller()
		return
	}

	// 崩溃日志（启动期）
	defer func() {
		if r := recover(); r != nil {
			writeCrashLog(fmt.Sprintf("PANIC: %v", r))
		}
	}()

	cleanupOld()
	// 安装引导：未安装 → 询问安装（系统级弹窗）→ 从安装目录重启
	ensureInstalledFlow()
	// WebView2 检测：缺失时中文提示并自动静默安装（替代 Wails 英文弹窗）
	if !ensureWebview2() {
		return
	}

	bridge := NewBridge()

	err := wails.Run(&options.App{
		Title:     "彬煤助手 v" + AppVersion,
		Width:     400,
		Height:    650,
		MinWidth:  360,
		MinHeight: 520,
		AssetServer: &assetserver.Options{Assets: assets},
		OnStartup: func(ctx context.Context) {
			bridge.attach(ctx)
			go updateWatchdog()
		},
		Bind:      []interface{}{bridge},
		Windows: &windows.Options{
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
			DisableWindowIcon:    false,
		},
	})
	if err != nil {
		log.Fatal(err)
	}
}