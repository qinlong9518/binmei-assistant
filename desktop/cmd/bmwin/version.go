//go:build windows

package main

// 应用版本信息
const (
	AppVersion     = "1.9.0"
	AppVersionCode = 2
	OfficialSite   = "https://qinlong9518.github.io/binmei-assistant/"
)

// 在线更新元数据源（镜像优先，与 App 更新同策略）
var metaSources = []string{
	"https://ghfast.top/https://raw.githubusercontent.com/qinlong9518/binmei-assistant/main/desktop_update.json",
	"https://ghproxy.net/https://raw.githubusercontent.com/qinlong9518/binmei-assistant/main/desktop_update.json",
	"https://raw.githubusercontent.com/qinlong9518/binmei-assistant/main/desktop_update.json",
}

// downloadMirrors 对 GitHub 直链的镜像加速前缀（顺序尝试）
var downloadMirrors = []string{
	"%s", // 直链兜底放最后
	"https://ghfast.top/%s",
	"https://ghproxy.net/%s",
}