//go:build windows

package main

import "syscall"

// CREATE_NO_WINDOW：隐藏子进程窗口（PowerShell/cmd 调用时不闪黑框）
const createNoWindow = 0x08000000

func hideWindow() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
}