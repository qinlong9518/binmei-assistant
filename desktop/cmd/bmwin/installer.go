//go:build windows

package main

import (
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"golang.org/x/sys/windows/registry"
)

// 安装目录：%LOCALAPPDATA%\Programs\BinmeiAssistant（微信/QQ 同类用户级安装，无需管理员）
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

// ensureInstalledFlow 未安装时询问是否安装（安装后从安装目录重启）
// 返回 true=继续运行当前进程（便携模式或已在安装目录）
func ensureInstalledFlow() bool {
	if isInstalled() {
		return true
	}
	// 若安装目录已有旧版本，视为已安装的升级场景
	if _, err := os.Stat(installedExePath()); err == nil {
		return true
	}
	ans := walkMsgBoxYesNo("安装",
		"是否将「彬煤答题助手」安装到电脑？\n\n"+
			"· 安装到本地程序目录（无需管理员权限）\n"+
			"· 创建桌面和开始菜单快捷方式\n"+
			"· 之后可删除本安装包，程序独立运行\n"+
			"· 可在系统「设置-应用」中卸载")
	if !ans {
		return true // 便携模式
	}
	exe, err := os.Executable()
	if err != nil {
		return true
	}
	if err := installFrom(exe); err != nil {
		walkMsgBoxError("安装失败", err.Error())
		return true
	}
	// 从安装目录重启
	startExe(installedExePath())
	os.Exit(0)
	return false
}

// installFrom 把 srcExe 安装到系统目录 + 快捷方式 + 卸载注册表
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

// createShortcuts 用 PowerShell WScript.Shell 创建桌面+开始菜单快捷方式
// （EncodedCommand 传 UTF-16LE base64，规避中文参数编码问题）
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
	// EncodedCommand 需要 UTF-16LE
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

// removeShortcuts 删除桌面+开始菜单快捷方式
func removeShortcuts() {
	dt := filepath.Join(os.Getenv("USERPROFILE"), "Desktop", lnkName)
	os.Remove(dt)
	sm := filepath.Join(os.Getenv("APPDATA"), "Microsoft", "Windows", "Start Menu", "Programs", lnkName)
	os.Remove(sm)
	// OneDrive 桌面重定向兼容
	od := filepath.Join(os.Getenv("USERPROFILE"), "OneDrive", "Desktop", lnkName)
	os.Remove(od)
	os.Remove(filepath.Join(os.Getenv("PUBLIC"), "Desktop", lnkName))
}

// writeUninstallEntry 写注册表：出现在系统「设置-应用」列表
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

// runUninstaller 卸载流程（bmmain /uninstall）
func runUninstaller() {
	if !walkMsgBoxYesNo("卸载确认", "确定要卸载「彬煤答题助手」吗？\n\n账号配置与本地数据将一并删除。") {
		return
	}
	removeShortcuts()
	removeUninstallEntry()
	dir := installDir()
	// 延迟自删（运行中的 exe 无法直接删除自己）
	cmd := exec.Command("cmd", "/c", fmt.Sprintf(`timeout /t 2 /nobreak >nul & rd /s /q "%s"`, dir))
	cmd.SysProcAttr = hideWindow()
	cmd.Start()
	walkMsgBoxInfo("卸载", "已卸载完成，程序目录将在数秒后自动清除。")
	os.Exit(0)
}

// startExe 独立启动一个程序
func startExe(path string) {
	exec.Command(path).Start()
}

// openURL 用系统默认浏览器打开（ShellExecute，正规 API）
func openURL(url string) {
	exec.Command("explorer.exe", url).Start()
}