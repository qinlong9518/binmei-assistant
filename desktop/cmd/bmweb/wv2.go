//go:build windows

package main

import (
	"os"
	"os/exec"
	"time"

	"golang.org/x/sys/windows/registry"
)

// webview2Installed 检测 WebView2 运行时（注册表 HKLM / WOW64 / HKCU 三处）
func webview2Installed() bool {
	paths := []string{
		`SOFTWARE\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}`,
		`SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}`,
	}
	roots := []registry.Key{registry.LOCAL_MACHINE, registry.LOCAL_MACHINE, registry.CURRENT_USER}
	for i, p := range paths {
		k, err := registry.OpenKey(roots[i], p, registry.QUERY_VALUE)
		if err == nil {
			k.Close()
			return true
		}
	}
	return false
}

// ensureWebview2 缺失时：中文提示 → 下载官方安装器 → 静默安装 → 重启自己
// 返回 true = 环境就绪可以继续启动
func ensureWebview2() bool {
	if webview2Installed() {
		return true
	}
	if !sysMsgBox("首次运行准备",
		"检测到系统缺少微软 WebView2 显示组件（正规系统组件，本程序界面所需）。\n\n"+
			"点击「确定」自动下载安装（约 1-3 分钟，取决于网速），\n"+
			"完成后程序将自动重新打开，无需其他操作。", true) {
		return false
	}
	boot := os.TempDir() + "\\bm_wv2setup.exe"
	var lastErr error
	for _, u := range []string{
		"https://go.microsoft.com/fwlink/p/?linkid=2124701",
		"https://ghfast.top/https://github.com/qinlong9518/binmei-assistant/releases/latest/download/WebView2Bootstrapper.exe",
		"https://github.com/qinlong9518/binmei-assistant/releases/latest/download/WebView2Bootstrapper.exe",
	} {
		done, err := downloadTo(u, boot, nil)
		if done {
			lastErr = nil
			break
		}
		lastErr = err
	}
	if lastErr != nil {
		sysMsgBoxInfo("下载失败", "自动下载未成功："+lastErr.Error()+"\n\n请检查网络后重新打开程序。")
		return false
	}
	cmd := exec.Command(boot, "/silent", "/install")
	cmd.SysProcAttr = hideWindow()
	_ = cmd.Start()
	for i := 0; i < 120; i++ {
		if webview2Installed() {
			break
		}
		time.Sleep(time.Second)
	}
	if !webview2Installed() {
		sysMsgBoxInfo("准备未完成", "组件安装未在预期时间内完成，请稍后重新打开程序。")
		return false
	}
	exe, err := os.Executable()
	if err == nil {
		startExe(exe)
	}
	os.Exit(0)
	return false
}