//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"time"

	"bmclient/bm"

	"github.com/lxn/walk"
)

func main() {
	defer func() {
		if r := recover(); r != nil {
			writeLog("bm_crash.log", fmt.Sprintf("PANIC: %v\n\n%s", r, debug.Stack()))
			os.Exit(2)
		}
	}()

	// 卸载模式（无窗口环境，walk.MsgBox 用 nil 宿主）
	if len(os.Args) > 1 && os.Args[1] == "/uninstall" {
		runUninstaller()
		return
	}

	debugLog("进程启动 " + AppVersion)

	debugLog("构建主窗口")
	if err := buildMainWindow(); err != nil {
		writeLog("bm_crash.log", "主窗口创建失败: "+err.Error())
		os.Exit(2)
	}
	debugLog("主窗口就绪")

	// 安装引导：未安装 → 询问安装到系统（微信/QQ 式）→ 从安装目录重启
	// （必须在主窗口创建后：MsgBox 需要宿主窗口）
	if !ensureInstalledFlow() {
		return
	}

	// 后台线程
	go uiTick()
	go pointsPoller()
	time.AfterFunc(2*time.Second, silentCheckUpdate)

	// 登录小窗：无已登录账号时先弹小窗（微信式）；取消则退出
	loggedIn := false
	if cfg.Last != "" {
		// 尝试静默恢复上次账号
		for _, a := range cfg.Accounts {
			if a.ID == cfg.Last {
				c := bm.NewClient()
				if err := c.Login(a.ID, a.Pwd); err == nil {
					rt.mu.Lock()
					rt.client = c
					rt.mu.Unlock()
					logf("✅ 已恢复登录: %s（%s）", c.Name, a.ID)
					loggedIn = true
				}
				break
			}
		}
	}
	if !loggedIn {
		c, ok := runLoginDialog()
		if !ok || c == nil {
			return
		}
	}

	debugLog("进入消息循环")
	mw.Run()
	appQuiting = true
	rt.mu.Lock()
	if rt.running && rt.stopCh != nil {
		close(rt.stopCh)
	}
	rt.mu.Unlock()
}

func uiTick() {
	for {
		if appQuiting {
			return
		}
		time.Sleep(500 * time.Millisecond)
		if mw != nil {
			mw.Synchronize(updateUI)
		}
	}
}

func pointsPoller() {
	for {
		if appQuiting {
			return
		}
		time.Sleep(3 * time.Second)
		rt.mu.Lock()
		c := rt.client
		rt.mu.Unlock()
		if c != nil {
			if pts, err := c.GetPoints(); err == nil {
				rt.mu.Lock()
				rt.lastPts = pts
				rt.mu.Unlock()
			}
		}
	}
}

// writeLog 向安装目录写日志
func writeLog(name, msg string) {
	dir := installDir()
	os.MkdirAll(dir, 0755)
	f, err := os.OpenFile(filepath.Join(dir, name), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "[%s] %s\n", time.Now().Format("2006-01-02 15:04:05"), msg)
}

func debugLog(msg string) {
	writeLog("bm_debug.log", msg)
}

// walkMsgBoxYesNo 确认框
func walkMsgBoxYesNo(title, msg string) bool {
	return walk.MsgBox(mw, title, msg, walk.MsgBoxIconQuestion|walk.MsgBoxYesNo) == walk.DlgCmdYes
}

// walkMsgBoxError 错误框
func walkMsgBoxError(title, msg string) {
	if mw == nil {
		walk.MsgBox(nil, title, msg, walk.MsgBoxIconError)
		return
	}
	walk.MsgBox(mw, title, msg, walk.MsgBoxIconError)
}

// walkMsgBoxInfo 信息框
func walkMsgBoxInfo(title, msg string) {
	if mw == nil {
		walk.MsgBox(nil, title, msg, walk.MsgBoxIconInformation)
		return
	}
	walk.MsgBox(mw, title, msg, walk.MsgBoxIconInformation)
}