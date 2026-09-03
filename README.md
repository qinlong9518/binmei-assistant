# 彬煤答题助手 (BM365 WebView App)

彬煤安培365（http://61.185.41.209:8888）自动答题 Android 助手。

## 功能

| 模块 | 说明 |
|---|---|
| 📱 原生登录 | 自绘登录页（仅需账号，密码/验证码内置），支持下拉切换历史账号 |
| ✅ 自动答题 | 注入 JS 轮询答题引擎，参数可调（轮询/延时/锁定/总题数/自动交卷） |
| 📊 积分面板 | 隐藏 WebView 双通道轮询积分，底部卡片实时显示 |
| ⚙️ 设置页 | Material 滑块设置 6 项引擎参数，SharedPreferences 持久化，热更新下发 |
| 🔋 电源菜单 | 标题栏电源按钮：账号下拉切换（最多5个+折叠）与退出登录 |
| 🔄 在线更新 | GitHub Releases 托管，多源 cache-buster 检测 + 进度条下载 + 自动安装 |

## 技术栈

- Kotlin + AndroidX（ViewBinding / ViewModel+LiveData / Material）
- 双 WebView 架构（主交互 + 隐藏积分监控），全局 Cookie 共享登录态
- minSdk 35 (Android 15+) / targetSdk 34

## 目录结构

```
app/src/main/java/com/bm365/app/
  MainActivity.kt      主界面（双 WebView + 工具栏 + 电源菜单 + 更新检查）
  LoginActivity.kt     原生登录页
  SettingsActivity.kt  设置页（滑块+开关+检查更新）
  BmSettings.kt        配置模型与持久化（bm_settings）
  BmAccountManager.kt  账号列表管理（bm_accounts）
  BmHttpLogin.kt       站点登录协议复刻（Esdt 加密 + Cookie 写入）
  BmUpdate.kt          在线更新（GitHub 多源 + 下载安装）
  PointsBridge.kt      积分 JS 桥
  PointsViewModel.kt   积分解析
  PointsAdapter.kt / PointsItem.kt
app/src/main/assets/
  auto_answer.js       自动答题引擎（window.BM_CFG 配置化）
  points_monitor.js    积分轮询（BM_SET_POLL 热更新）
```

## 构建

标准 Gradle Wrapper，任何装有 JDK 17 的环境克隆后直接构建：

```bash
./gradlew assembleRelease          # Linux/macOS（首次自动下载 Gradle 8.5）
gradlew.bat assembleRelease        # Windows
# 产物: app/build/outputs/apk/release/app-release.apk（debug 签名可直接安装）
```

- 需要 **JDK 17** 与网络（首次拉取 Android SDK 组件与依赖）
- Android Studio：直接 Open 项目根目录，其余全自动
- ARM64 手机端（proot）构建需本机 aapt2 适配，见 `gradle.properties` 注释技巧——常规环境无需任何额外配置

## 发版流程

见 [RELEASE_GUIDE.md](RELEASE_GUIDE.md)：升版本号 → 构建 → GitHub Release 传 APK → 更新 `update.json`（App 端自动弹窗更新）。