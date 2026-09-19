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

// pendingUpdate 待提醒的更新信息（供前端主动拉取，防事件早于监听丢失）
var pendingUpdate = ""

// notifiedVersion 本会话已弹过提醒的版本（避免重复打扰）
var notifiedVersion = ""

// silentCheckUpdate 静默检查更新；发现新版本 → 状态栏 + 自动弹窗提醒
func silentCheckUpdate() {
	meta, err := fetchUpdateMeta()
	if err != nil || meta == nil {
		return
	}
	if meta.VersionCode > AppVersionCode {
		setStatus("发现新版本 v" + meta.VersionName)
		if notifiedVersion == meta.VersionName {
			return
		}
		notifiedVersion = meta.VersionName
		msg := meta.VersionName + "\n\n" + meta.Changelog
		pendingUpdate = msg
		if bridgeRef != nil {
			bridgeRef.emit("updateAvailable", msg)
		}
	}
}

// updateWatchdog 启动 5 秒后首查，之后每 4 小时复查一次
func updateWatchdog() {
	time.Sleep(5 * time.Second)
	silentCheckUpdate()
	ticker := time.NewTicker(4 * time.Hour)
	for range ticker.C {
		silentCheckUpdate()
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