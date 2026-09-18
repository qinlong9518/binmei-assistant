# 彬煤答题助手 · 电脑端

与 Android 版同源的桌面客户端（Go 实现，纯协议直连站点，无需 WebView/模拟器）。

## 功能
- 原生登录（Esdt 协议，多账号管理，显示人名）
- 今日积分面板（两行三列，3 秒缓存轮询）
- 自动答题任务：手机考试/模拟考试分别判断，不足 24 分自动选「试卷二」补足
- 配置持久化 `bm_config.json`（与 exe 同目录），重启自动恢复登录

## 运行
双击 exe（Windows），自动打开浏览器界面 `http://127.0.0.1:8642`。

## 构建
```
# Windows exe（任意平台交叉编译）
GOOS=windows GOARCH=amd64 go build -ldflags="-s -w -H windowsgui" -o 彬煤答题助手-电脑端.exe ./cmd/bmdesktop

# Linux / macOS
GOOS=linux  GOARCH=amd64 go build -o bmdesk ./cmd/bmdesktop
GOOS=darwin GOARCH=arm64 go build -o bmdesk-mac ./cmd/bmdesktop
```

## 结构
```
bm/                 协议核心（Esdt/登录/积分/试卷/建卷/答题/交卷）
cmd/bmdesktop/      桌面端（本地 HTTP 服务 + 内嵌 Web UI + 调度循环）
```

## 协议要点（站点逆向成果）
- 登录 `POST /PersonWap/GetPersonInfo`：Esdt(idcard/pwd)，yzm=1 回退 auto
- 服务端会话：`POST /PersonWap/FirstIndexOne`（**原始 pid**，非 Esdt）
- 积分：`GET /AccumulateManger/S_Accumulate/GetPersonTodayAccumulateOne?pid=Esdt(pid)`
- 试卷列表：`GET /ExamManger/P_Paper/SelectCanRunListOne?examtypeid=…&pid=…`
- 建考卷：`POST /Home/CreateTempExamOne`，**type=2 手机考试 / type=1 模拟考试（决定积分归属）**
- 写答题会话：`POST /P_ExamDetail/SetOnlineTestOne`（全参数 Esdt）
- 答题页 `GET /P_ExamDetail/OnlineTestOne`：内联 `vData.AllQuestionArray`
  每题 17 字段：[4]=我的答案、[13]=正确答案（单选 A-D / 判断 Y-N）、[14]=题型（1 单选/3 判断）
- 提交答案：`POST /Home/DoWriteAnswerAllOne {answer: 前15字段×题数用|拼接, ksmxid}`
- 交卷：`POST /Home/EndTimeOne {ksmxid}` → `分数|...|合格！`

仅供学习交流，请合理合规使用。
