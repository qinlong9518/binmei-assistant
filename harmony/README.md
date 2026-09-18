# 彬煤答题助手 · 鸿蒙原生版（HarmonyOS ArkTS）

与 Windows/Android 端同源协议的鸿蒙原生应用。

## 工程
```
AppScope/app.json5                        应用配置（bundleName: com.binmei.assistant）
entry/src/main/module.json5               模块配置（INTERNET 权限）
entry/src/main/ets/entryability/          Ability 入口
entry/src/main/ets/pages/Index.ets        主页面（ArkUI 声明式，微信绿主题）
entry/src/main/ets/common/BmClient.ts     站点协议（Esdt/登录/积分，与桌面端同源）
entry/src/main/resources/                 图标/字符串/颜色
```

## 功能
- 登录页（微信绿主题，账号输入 + 登录）
- 主页：绿色顶栏（品牌+用户名）、积分网格（3列，满绿缺红）、任务卡片（启动/停止）、运行日志
- 3 秒轮询积分刷新

## 构建
使用 DevEco Studio 打开工程构建 HAP；或命令行：
```
hvigorw assembleHap
```

## 说明
- 协议与桌面端完全同源（Esdt 混淆 + 服务端会话 + 积分接口）
- 自动答题引擎（建卷/答题/交卷）可按 desktop/bm/exam.go 的协议移植，接口链路已在桌面端实测
- 纯原生 ArkUI，无 WebView 依赖

仅供学习交流使用。