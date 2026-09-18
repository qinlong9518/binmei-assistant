//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"time"

	"bmclient/bm"
)

// main 崩溃保护入口：任何 panic 都会写入 exe 同目录 bm_crash.log
func main() {
	defer func() {
		if r := recover(); r != nil {
			writeLog("bm_crash.log", fmt.Sprintf("PANIC: %v\n\n%s", r, debug.Stack()))
			os.Exit(2)
		}
	}()
	debugLog("进程启动")
	run()
	debugLog("消息循环退出")
}

func run() {
	cleanupOld()

	// 恢复上次账号
	if cfg.Last != "" {
		for _, a := range cfg.Accounts {
			if a.ID == cfg.Last {
				go func(acc Account) {
					c := bm.NewClient()
					if err := c.Login(acc.ID, acc.Pwd); err == nil {
						rt.mu.Lock()
						rt.client = c
						rt.mu.Unlock()
						logf("✅ 已恢复登录: %s（%s）", c.Name, acc.ID)
					} else {
						logf("⚠️ 恢复登录失败: %v", err)
					}
				}(a)
				break
			}
		}
	}

	debugLog("开始构建窗口")
	buildUI()
	debugLog("窗口构建完成")

	// UI 刷新 tick
	go func() {
		for {
			if appQuiting {
				return
			}
			time.Sleep(500 * time.Millisecond)
			if mw != nil {
				mw.Synchronize(updateUI)
			}
		}
	}()

	// 积分轮询（3s）
	go func() {
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
	}()

	time.AfterFunc(2*time.Second, silentCheckUpdate)

	debugLog("进入消息循环")
	mw.Run()
}

// writeLog 向 exe 同目录写日志（诊断双击无反应）
func writeLog(name, msg string) {
	dir := "."
	if exe, err := os.Executable(); err == nil {
		dir = filepath.Dir(exe)
	}
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