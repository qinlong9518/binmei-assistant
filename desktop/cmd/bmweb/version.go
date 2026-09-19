//go:build windows

package main

// 应用版本信息
const (
	AppVersion     = "1.14.1"
	AppVersionCode = 14
	OfficialSite   = "https://qinlong9518.github.io/binmei-assistant/"
)

// 在线更新元数据源（镜像优先）
var metaSources = []string{
	"https://ghfast.top/https://raw.githubusercontent.com/qinlong9518/binmei-assistant/main/desktop_update.json",
	"https://ghproxy.net/https://raw.githubusercontent.com/qinlong9518/binmei-assistant/main/desktop_update.json",
	"https://raw.githubusercontent.com/qinlong9518/binmei-assistant/main/desktop_update.json",
}

// downloadMirrors GitHub 直链镜像前缀
var downloadMirrors = []string{
	"https://ghfast.top/%s",
	"https://ghproxy.net/%s",
	"%s",
}