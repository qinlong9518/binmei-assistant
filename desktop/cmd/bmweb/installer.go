//go:build windows

package main

import (
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows/registry"
)

// 安装目录：%LOCALAPPDATA%\Programs\BinmeiAssistant（用户级安装，无需管理员）
const (
	appExeName    = "BinmeiAssistant.exe"
	installSubDir = `Programs\BinmeiAssistant`
	uninstallKey  = `Software\Microsoft\Windows\CurrentVersion\Uninstall\BinmeiAssistant`
	lnkName       = `彬煤答题助手.lnk`
)

func installDir() string {
	la := os.Getenv("LOCALAPPDATA")
	if la == "" {
		la = filepath.Join(os.Getenv("USERPROFILE"), "AppData", "Local")
	}
	return filepath.Join(la, installSubDir)
}

func installedExePath() string { return filepath.Join(installDir(), appExeName) }

// isInstalled 当前进程是否运行于安装目录
func isInstalled() bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	return filepath.Dir(exe) == installDir()
}

// sysMsgBox 系统 MessageBox（不依赖 GUI 框架，供安装/卸载流程用）
func sysMsgBox(caption, text string, yesNo bool) bool {
	const mbYesNo = 0x04
	const mbIconQuestion = 0x20
	const mbIconError = 0x10
	const mbIconInfo = 0x40
	flags := uintptr(mbYesNo | mbIconQuestion)
	if !yesNo {
		flags = uintptr(mbIconError)
	}
	cap16, _ := syscall.UTF16PtrFromString(caption)
	txt16, _ := syscall.UTF16PtrFromString(text)
	mb := msgBoxProc()
	ret, _, _ := syscall.SyscallN(mb, 0,
		uintptr(unsafe.Pointer(txt16)), uintptr(unsafe.Pointer(cap16)), flags)
	return ret != 0
}

func sysMsgBoxInfo(caption, text string) {
	cap16, _ := syscall.UTF16PtrFromString(caption)
	txt16, _ := syscall.UTF16PtrFromString(text)
	mb := msgBoxProc()
	syscall.SyscallN(mb, 0,
		uintptr(unsafe.Pointer(txt16)), uintptr(unsafe.Pointer(cap16)), uintptr(0x40))
}

var (
	user32Mod   *syscall.DLL
	msgBoxAddr  uintptr
	onceSyscall sync.Once
)

func winUser32() *syscall.DLL {
	onceSyscall.Do(func() {
		user32Mod = syscall.MustLoadDLL("user32.dll")
		p, err := user32Mod.FindProc("MessageBoxW")
		if err != nil {
			panic(err)
		}
		msgBoxAddr = p.Addr()
	})
	return user32Mod
}

func msgBoxProc() uintptr {
	winUser32()
	return msgBoxAddr
}

// ensureInstalledFlow 未安装时询问是否安装（安装后从安装目录重启）
func ensureInstalledFlow() bool {
	if isInstalled() {
		return true
	}
	// 安装目录已有旧版本 → 自动升级：覆盖安装并从安装目录重启
	if _, err := os.Stat(installedExePath()); err == nil {
		exe, err := os.Executable()
		if err == nil && filepath.Dir(exe) != installDir() {
			// 从下载目录/其他位置运行 → 覆盖安装目录的程序
			if err := installFrom(exe); err == nil {
				startExe(installedExePath())
				os.Exit(0)
			}
		}
		return true
	}
	if !sysMsgBox("安装",
		"是否将「彬煤答题助手」安装到电脑？\n\n"+
			"· 安装到本地程序目录（无需管理员权限）\n"+
			"· 创建桌面和开始菜单快捷方式\n"+
			"· 之后可删除本安装包，程序独立运行\n"+
			"· 可在系统「设置-应用」中卸载", true) {
		return true // 便携模式
	}
	exe, err := os.Executable()
	if err != nil {
		return true
	}
	if err := installFrom(exe); err != nil {
		sysMsgBoxInfo("安装失败", err.Error())
		return true
	}
	startExe(installedExePath())
	os.Exit(0)
	return false
}

// installFrom 安装到系统目录 + 快捷方式 + 卸载注册表
func installFrom(srcExe string) error {
	dir := installDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("创建安装目录失败: %w", err)
	}
	dst := installedExePath()
	if err := copyFile(srcExe, dst); err != nil {
		return fmt.Errorf("写入程序失败: %w", err)
	}
	if err := createShortcuts(dir); err != nil {
		return fmt.Errorf("创建快捷方式失败: %w", err)
	}
	if err := writeUninstallEntry(dir); err != nil {
		return fmt.Errorf("注册卸载信息失败: %w", err)
	}
	return nil
}

// createShortcuts PowerShell WScript.Shell 创建桌面+开始菜单快捷方式
func createShortcuts(dir string) error {
	exe := filepath.Join(dir, appExeName)
	ps := fmt.Sprintf(`
$ws = New-Object -ComObject WScript.Shell
$desktop = [Environment]::GetFolderPath('Desktop')
$startmenu = Join-Path $env:APPDATA 'Microsoft\Windows\Start Menu\Programs'
foreach ($d in @($desktop, $startmenu)) {
  $l = $ws.CreateShortcut((Join-Path $d '%s'))
  $l.TargetPath = '%s'
  $l.WorkingDirectory = '%s'
  $l.IconLocation = '%s,0'
  $l.Save()
}`, lnkName, exe, dir, exe)
	u16 := make([]byte, 0, len(ps)*2+2)
	for _, r := range ps {
		u16 = append(u16, byte(r), byte(r>>8))
	}
	enc := base64.StdEncoding.EncodeToString(u16)
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-EncodedCommand", enc)
	cmd.SysProcAttr = hideWindow()
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v: %s", err, string(out))
	}
	return nil
}

// removeShortcuts 删除快捷方式
func removeShortcuts() {
	dt := filepath.Join(os.Getenv("USERPROFILE"), "Desktop", lnkName)
	os.Remove(dt)
	sm := filepath.Join(os.Getenv("APPDATA"), "Microsoft", "Windows", "Start Menu", "Programs", lnkName)
	os.Remove(sm)
	od := filepath.Join(os.Getenv("USERPROFILE"), "OneDrive", "Desktop", lnkName)
	os.Remove(od)
	os.Remove(filepath.Join(os.Getenv("PUBLIC"), "Desktop", lnkName))
}

// writeUninstallEntry 写系统卸载注册表
func writeUninstallEntry(dir string) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, uninstallKey, registry.SET_VALUE|registry.QUERY_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	exe := filepath.Join(dir, appExeName)
	k.SetStringValue("DisplayName", "彬煤答题助手")
	k.SetStringValue("DisplayVersion", AppVersion)
	k.SetStringValue("DisplayIcon", exe)
	k.SetStringValue("UninstallString", exe+` /uninstall`)
	k.SetStringValue("Publisher", "Binmei Assistant")
	k.SetDWordValue("NoModify", 1)
	k.SetDWordValue("NoRepair", 1)
	k.SetStringValue("InstallLocation", dir)
	return nil
}

// removeUninstallEntry 删除卸载注册表项
func removeUninstallEntry() {
	registry.DeleteKey(registry.CURRENT_USER, uninstallKey)
}

// runUninstaller 卸载流程
func runUninstaller() {
	if !sysMsgBox("卸载确认", "确定要卸载「彬煤答题助手」吗？\n\n账号配置与本地数据将一并删除。", true) {
		return
	}
	removeShortcuts()
	removeUninstallEntry()
	dir := installDir()
	cmd := exec.Command("cmd", "/c", fmt.Sprintf(`timeout /t 2 /nobreak >nul & rd /s /q "%s"`, dir))
	cmd.SysProcAttr = hideWindow()
	cmd.Start()
	sysMsgBoxInfo("卸载", "已卸载完成，程序目录将在数秒后自动清除。")
	os.Exit(0)
}

func startExe(path string) {
	exec.Command(path).Start()
}

// openURL 系统默认浏览器打开
func openURL(url string) {
	exec.Command("explorer.exe", url).Start()
}

// copyFile 文件复制
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

var _ = unsafe.Pointer(nil)