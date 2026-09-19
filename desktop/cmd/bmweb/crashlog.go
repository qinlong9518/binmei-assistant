//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// writeCrashLog 崩溃/调试日志
func writeCrashLog(msg string) {
	dir := installDir()
	os.MkdirAll(dir, 0755)
	f, err := os.OpenFile(filepath.Join(dir, "bm_crash.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "[%s] %s\n", time.Now().Format("2006-01-02 15:04:05"), msg)
}

// silentCheckUpdate 启动后静默检查（仅状态栏提示）
func silentCheckUpdate() {
	meta, err := fetchUpdateMeta()
	if err != nil || meta == nil {
		return
	}
	if meta.VersionCode > AppVersionCode {
		setStatus("发现新版本 v" + meta.VersionName + "，可点「检查更新」安装")
	}
}

// startBackground 启动后台任务（startup 后调用）
func startBackground(bridge *UIBridge) {
	go func() {
		time.Sleep(2 * time.Second)
		silentCheckUpdate()
	}()
	_ = bridge
}