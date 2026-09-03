# 在线更新发布指南（GitHub 托管，无需自有服务器）

## 一、首次准备（只做一次）

1. 注册/登录 GitHub，创建**公开仓库**（如 `binmei-assistant`）
2. 把本目录的 `update.json`、本文档、以及（可选）APK 源码发布进去
3. 设置页里 `BmUpdate.kt` 顶部三个常量改为你的仓库信息：

```kotlin
const val REPO_OWNER = "你的GitHub用户名"
const val REPO_NAME  = "binmei-assistant"
const val BRANCH     = "main"
```

4. 重新构建 APK，发给用户安装（这是最后一个"手动发 APK"的版本）

## 二、以后每次发新版（3 步）

### 1. 改版本号
`app/build.gradle.kts`：
```kotlin
versionCode = 7        // 必须比上一版大
versionName = "1.6"
```

### 2. 构建 + 上传 Release
```bash
# 本工作区构建命令（已配好 ARM64 环境）：
cd /storage/emulated/0/Download/android-webview-app
export ANDROID_HOME=/root/Android
sh gradlew assembleRelease --no-daemon
# 产物：app/build/outputs/apk/release/app-release.apk
```
GitHub 网页操作：仓库页 → Releases → Draft a new release
- Tag：`v1.6`
- 标题：`v1.6`
- 拖入 `app-release.apk` 作为二进制资产
- Publish

### 3. 更新 update.json（推到仓库根目录）
```json
{
  "versionCode": 7,
  "versionName": "1.6",
  "force": false,
  "changelog": "1. 修复xxx\n2. 优化yyy",
  "apkUrl": "https://github.com/你的用户名/binmei-assistant/releases/download/v1.6/app-release.apk",
  "apkFile": "app-release.apk"
}
```
> `force: true` = 强制更新（弹窗不可关闭）。APK 资产名必须与 apkUrl 一致。

## 三、App 端行为（已实现）

| 入口 | 行为 |
|---|---|
| 设置页「检查更新」 | 显式检查：失败/最新/有新版均有 Toast 或弹窗 |
| 主界面启动 | 静默检查：仅发现新版才弹窗 |
| 弹窗 | 显示版本号 + 更新内容；force=true 时强制 |
| 下载 | GitHub 直链 → gh-proxy.com → mirror.ghproxy.com 三源自动回退，带进度条 |
| 安装 | FileProvider 拉起系统安装器；未授权「安装未知应用」时自动跳授权页 |

## 四、可达性说明（国内网络）

- `update.json` 读取：raw.githubusercontent.com → cdn.jsdelivr.net → fastly.jsdelivr.net 三源回退
- APK 下载：github.com → gh-proxy.com → mirror.ghproxy.com 三源回退
- 若 GitHub 完全不可达，可把 update.json 和 APK 放 Gitee（同结构改 URL 即可，代码无需变）

## 五、可选：CI 自动发布

配置 `.github/workflows/release.yml` 后，push tag 即自动构建上传（后续需要再加）。
