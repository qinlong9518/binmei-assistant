//go:build windows

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"bmclient/bm"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
	"github.com/lxn/win"
)

// ---------- 配置（存安装目录，卸载时一并清理） ----------

type Account struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Pwd  string `json:"pwd"`
}

type Config struct {
	Accounts []Account `json:"accounts"`
	Last     string    `json:"last"`
}

func cfgPath() string {
	return filepath.Join(installDir(), "bm_config.json")
}

// loadConfig 兼容旧版（浏览器版/便携版）配置迁移
func loadConfig() *Config {
	c := &Config{}
	// 1) 新位置：安装目录
	if b, err := os.ReadFile(cfgPath()); err == nil {
		json.Unmarshal(b, c)
		return c
	}
	// 2) 旧位置：exe 同目录（便携版/浏览器版遗留）
	if exe, err := os.Executable(); err == nil {
		old := filepath.Join(filepath.Dir(exe), "bm_config.json")
		if b, err := os.ReadFile(old); err == nil {
			if json.Unmarshal(b, c) == nil && len(c.Accounts) > 0 {
				return c // 安装流程里会迁移到新位置
			}
		}
	}
	// 3) 兼容安装目录存在但文件在旧 exe 旁的情况
	if _, err := os.Stat(installedExePath()); err == nil {
		old := filepath.Join(filepath.Dir(installedExePath()), "bm_config.json")
		if b, err := os.ReadFile(old); err == nil {
			json.Unmarshal(b, c)
		}
	}
	return c
}

func saveConfig(c *Config) {
	os.MkdirAll(installDir(), 0755)
	b, _ := json.MarshalIndent(c, "", "  ")
	os.WriteFile(cfgPath(), b, 0600)
}

// ---------- 运行时 ----------

type Runtime struct {
	mu      sync.Mutex
	client  *bm.Client
	running bool
	stopCh  chan struct{}
	logs    []string
	taskMsg string
	lastPts []bm.PointsDetail
}

var rt = &Runtime{}
var cfg = loadConfig()

func logf(format string, a ...interface{}) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.logs = append(rt.logs, time.Now().Format("15:04:05")+"  "+fmt.Sprintf(format, a...))
	if len(rt.logs) > 300 {
		rt.logs = rt.logs[len(rt.logs)-300:]
	}
}

// ---------- 自动答题引擎 ----------

const passScore = 24.0

func autoLoop(stop chan struct{}) {
	for {
		select {
		case <-stop:
			return
		default:
		}
		rt.mu.Lock()
		c := rt.client
		rt.mu.Unlock()
		if c == nil {
			return
		}
		runOnce(c)
		select {
		case <-stop:
			return
		case <-time.After(3 * time.Second):
		}
	}
}

func runOnce(c *bm.Client) {
	pts, err := c.GetPoints()
	if err == nil {
		rt.mu.Lock()
		rt.lastPts = pts
		rt.mu.Unlock()
		for _, p := range pts {
			if p.Cur < passScore && (strings.Contains(p.Name, "手机考试") || strings.Contains(p.Name, "模拟考试")) {
				logf("📋 %s %g/%g 未满 → 自动开考", p.Name, p.Cur, p.Max)
				doExam(c, p.Name)
				return
			}
		}
		rt.mu.Lock()
		rt.taskMsg = "全部达标"
		rt.mu.Unlock()
		return
	}
	logf("⚠️ 积分获取失败: %v", err)
}

func doExam(c *bm.Client, examName string) {
	papers, err := c.SelectCanRunList(c.ExamTypeID)
	if err != nil {
		logf("❌ 试卷列表失败: %v", err)
		return
	}
	var target *bm.Paper
	for i := range papers {
		if strings.Contains(papers[i].PaperName, "二") {
			target = &papers[i]
			break
		}
	}
	if target == nil && len(papers) > 0 {
		target = &papers[0]
	}
	if target == nil {
		logf("❌ 无可考试卷")
		return
	}
	logf("📝 [%s] 开始: %s", examName, target.PaperName)
	t0 := time.Now()
	examType := "2"
	if strings.Contains(examName, "模拟") {
		examType = "1"
	}
	res, err := c.RunExamMode(target, examType)
	if err != nil {
		logf("❌ 考试失败: %v", err)
		return
	}
	parts := strings.Split(res, "|")
	score := ""
	if len(parts) > 1 {
		score = parts[1]
	}
	logf("🎉 [%s] 交卷: %s 分（%ds）", examName, score, time.Since(t0).Milliseconds()/1000)
	rt.mu.Lock()
	rt.taskMsg = fmt.Sprintf("最近: %s %s 分", examName, score)
	rt.mu.Unlock()
}

// ---------- 登录小窗 ----------

// runLoginDialog 登录小窗（微信式）：确认/取消
// 返回 (登录成功的client, 是否继续)
func runLoginDialog() (*bm.Client, bool) {
	var dlg *walk.Dialog
	var le *walk.LineEdit
	var cb *walk.ComboBox
	var statusLb *walk.Label
	var btnLogin *walk.PushButton
	var acceptPB, cancelPB *walk.PushButton

	model := &comboBoxModel{items: accountItems(cfg)}

	dlgResult := make(chan int, 1)

	dialogErr := (Dialog{
		AssignTo:      &dlg,
		Title:         "彬煤答题助手 - 登录",
		MinSize:       Size{Width: 340, Height: 200},
		MaxSize:       Size{Width: 340, Height: 200},
		DefaultButton: &acceptPB,
		CancelButton:  &cancelPB,
		Layout:        VBox{Margins: Margins{Left: 20, Top: 16, Right: 20, Bottom: 14}, Spacing: 10},
		Children: []Widget{
			Label{Text: "彬煤答题助手", Font: Font{PointSize: 13, Bold: true}},
			Label{AssignTo: &statusLb, Text: "输入账号登录（密码自动填充）", TextColor: walk.RGB(130, 130, 130)},
			LineEdit{AssignTo: &le, CueBanner: "身份证账号"},
			Composite{
				Layout: HBox{MarginsZero: true, Spacing: 6},
				Children: []Widget{
					Label{Text: "历史:"},
					ComboBox{AssignTo: &cb, Model: model, OnCurrentIndexChanged: func() {
						if uiLock {
							return
						}
						if id := idFromItem(cb.Text()); id != "" {
							le.SetText(id)
						}
					}},
				},
			},
			Composite{
				Layout: HBox{MarginsZero: true, Spacing: 8},
				Children: []Widget{
					PushButton{AssignTo: &btnLogin, Text: "登 录", MinSize: Size{Width: 90, Height: 30},
						OnClicked: func() {
							acc := strings.TrimSpace(le.Text())
							if acc == "" {
								statusLb.SetText("请输入账号")
								return
							}
							btnLogin.SetEnabled(false)
							statusLb.SetText("登录中…")
							go func() {
								c := bm.NewClient()
								err := c.Login(acc, bm.DefaultPassword)
								dlg.Synchronize(func() {
									if err != nil {
										btnLogin.SetEnabled(true)
										statusLb.SetText("登录失败：" + err.Error())
										return
									}
									upsertAccount(acc, c.Name)
									dlgResult <- 1
									dlg.Accept()
								})
								if err == nil {
									rt.mu.Lock()
									rt.client = c
									rt.mu.Unlock()
									logf("✅ 登录成功: %s（%s）", c.Name, acc)
								}
							}()
						}},
					PushButton{AssignTo: &acceptPB, Text: "", Visible: false},
					PushButton{AssignTo: &cancelPB, Text: "取 消", MinSize: Size{Width: 90, Height: 30},
						OnClicked: func() { dlgResult <- 0; dlg.Cancel() }},
				},
			},
		},
	}).Create(mw)
	if dialogErr != nil {
		return nil, false
	}
	// 已有历史账号则预填
	if cfg.Last != "" {
		le.SetText(cfg.Last)
	}
	dlg.Run()
	r := <-dlgResult
	if r == 1 {
		return rtClient(), true
	}
	return nil, false
}

func rtClient() *bm.Client {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return rt.client
}

func accountItems(c *Config) []string {
	items := []string{}
	for _, a := range c.Accounts {
		items = append(items, a.Name+"（"+a.ID+"）")
	}
	return items
}

func idFromItem(txt string) string {
	i := strings.LastIndex(txt, "（")
	j := strings.LastIndex(txt, "）")
	if i < 0 || j <= i {
		return ""
	}
	return txt[i+len("（") : j]
}

func upsertAccount(id, name string) {
	found := false
	for i := range cfg.Accounts {
		if cfg.Accounts[i].ID == id {
			cfg.Accounts[i].Name = name
			found = true
		}
	}
	if !found {
		cfg.Accounts = append(cfg.Accounts, Account{ID: id, Name: name, Pwd: bm.DefaultPassword})
	}
	cfg.Last = id
	saveConfig(cfg)
}

// ---------- 主窗口 ----------
var (
	mw          *walk.MainWindow
	userLink    *walk.LinkLabel
	verLabel    *walk.Label
	siteLink    *walk.LinkLabel
	updLink     *walk.LinkLabel
	ptLabels    [6]*walk.Label
	taskLabel   *walk.Label
	btnToggle   *walk.PushButton
	logEdit     *walk.TextEdit
	pb          *walk.ProgressBar
	statusLabel *walk.Label

	uiLock     bool
	appQuiting bool
)

func pointNames() []string {
	return []string{"签到", "知识学习", "手机考试", "模拟考试", "手机练习", "视频学习"}
}

type comboBoxModel struct {
	walk.ListModelBase
	items []string
}

func (m *comboBoxModel) ItemCount() int          { return len(m.items) }
func (m *comboBoxModel) Value(i int) interface{} { return m.items[i] }

func buildMainWindow() error {
	return (MainWindow{
		AssignTo: &mw,
		Title:    "彬煤答题助手",
		MinSize:  Size{Width: 400, Height: 590},
		Layout:   VBox{MarginsZero: true, Spacing: 0},
		Children: []Widget{
			// 绿色顶栏：品牌 + 账号（点击管理）
			Composite{
				Layout:     HBox{Margins: Margins{Left: 14, Top: 12, Right: 14, Bottom: 12}},
				Background: SolidColorBrush{Color: walk.RGB(7, 193, 96)},
				Children: []Widget{
					Label{Text: "彬煤答题助手", TextColor: walk.RGB(255, 255, 255),
						Font: Font{PointSize: 13, Bold: true}},
					HSpacer{},
					LinkLabel{AssignTo: &userLink, Text: "未登录 ▼",
						OnMouseDown: func(x, y int, btn walk.MouseButton) {
							if btn == walk.LeftButton {
								onAccountManager()
							}
						}},
				},
			},
			// 主体
			Composite{
				Layout: VBox{Margins: Margins{Left: 12, Top: 10, Right: 12, Bottom: 4}, Spacing: 8},
				Children: []Widget{
					GroupBox{
						Title:  "今日积分",
						Layout: Grid{Columns: 3, Spacing: 6},
						Children: []Widget{
							Label{AssignTo: &ptLabels[0], Text: "签到  -"},
							Label{AssignTo: &ptLabels[1], Text: "知识学习  -"},
							Label{AssignTo: &ptLabels[2], Text: "手机考试  -"},
							Label{AssignTo: &ptLabels[3], Text: "模拟考试  -"},
							Label{AssignTo: &ptLabels[4], Text: "手机练习  -"},
							Label{AssignTo: &ptLabels[5], Text: "视频学习  -"},
						},
					},
					GroupBox{
						Title:  "自动答题（不足24分自动补足·选试卷二）",
						Layout: HBox{MarginsZero: true, Spacing: 8},
						Children: []Widget{
							Label{AssignTo: &taskLabel, Text: "⚪ 待登录"},
							PushButton{AssignTo: &btnToggle, Text: "启动", OnClicked: onToggleClicked},
						},
					},
					GroupBox{
						Title:  "运行日志",
						Layout: VBox{MarginsZero: true},
						Children: []Widget{
							TextEdit{AssignTo: &logEdit, ReadOnly: true, VScroll: true, MinSize: Size{Height: 170}},
						},
					},
					ProgressBar{AssignTo: &pb, MinValue: 0, MaxValue: 100, Visible: false},
				},
			},
			// 底部状态栏：官网 | 检查更新 | 版本号（右下）
			Composite{
				Layout: HBox{Margins: Margins{Left: 12, Top: 2, Right: 12, Bottom: 8}, Spacing: 12},
				Children: []Widget{
					LinkLabel{AssignTo: &siteLink, Text: "官网",
						OnMouseUp: func(x, y int, btn walk.MouseButton) {
							if btn == walk.LeftButton {
								openURL(OfficialSite)
							}
						}},
					LinkLabel{AssignTo: &updLink, Text: "检查更新",
						OnMouseUp: func(x, y int, btn walk.MouseButton) {
							if btn == walk.LeftButton {
								onCheckUpdateClicked()
							}
						}},
					HSpacer{},
					Label{AssignTo: &verLabel, Text: "v" + AppVersion,
						TextColor: walk.RGB(150, 150, 150)},
				},
			},
		},
	}).Create()
}
// ---------- 账号菜单（右上角名字点击 → 下拉） ----------

func onAccountManager() {
	// 右上角名字点击 → walk 标准弹出菜单
	cur := "未登录"
	var selID string
	if c := rtClient(); c != nil {
		cur = c.Name + "（" + c.Account + "）"
		selID = c.Account
	}
	pm, err := walk.NewMenu()
	if err != nil {
		return
	}
	defer pm.Dispose()
	acts := pm.Actions()
	// 当前账号（置灰）
	actCur := walk.NewAction()
	actCur.SetText("当前：" + cur)
	actCur.SetEnabled(false)
	acts.Add(actCur)
	// 其他账号（可切换）
	for _, a := range cfg.Accounts {
		if a.ID == selID {
			continue
		}
		aa := a
		act := walk.NewAction()
		act.SetText("切换 " + a.Name)
		act.Triggered().Attach(func() {
			go func(acc Account) {
				setStatus("切换中…")
				c := bm.NewClient()
				if err := c.Login(acc.ID, acc.Pwd); err == nil {
					cfg.Last = acc.ID
					saveConfig(cfg)
					rt.mu.Lock()
					rt.client = c
					rt.mu.Unlock()
					logf("🔄 已切换: %s（%s）", c.Name, acc.ID)
					setStatus("")
				} else {
					logf("❌ 切换失败: %v", err)
					setStatus("切换失败")
				}
			}(aa)
		})
		acts.Add(act)
	}
	acts.Add(walk.NewSeparatorAction())
	actMgmt := walk.NewAction()
	actMgmt.SetText("账号管理（删除记录）…")
	actMgmt.Triggered().Attach(onAccountManagerDialog)
	acts.Add(actMgmt)
	// 在窗口右上（用户名下方）弹出：SetContextMenu + 模拟右键消息
	if p := userMenuPos(); p != nil {
		win.SetForegroundWindow(mw.Handle())
		win.SendMessage(mw.Handle(), win.WM_CONTEXTMENU,
			uintptr(mw.Handle()), uintptr(win.MAKELONG(uint16(p.X), uint16(p.Y))))
	}
	// 注册为窗口 context menu（WM_CONTEXTMENU 触发时 walk 会弹出它）
	mw.SetContextMenu(pm)
}

// userMenuPos 用户名下方的屏幕坐标
func userMenuPos() *walk.Point {
	if mw == nil {
		return nil
	}
	var rect win.RECT
	if !win.GetWindowRect(mw.Handle(), &rect) {
		return nil
	}
	return &walk.Point{X: int(rect.Right) - 170, Y: int(rect.Top) + 55}
}

// ---------- 账号管理小窗（查看/切换/删除） ----------

func onAccountManagerDialog() {
	var dlg *walk.Dialog
	var list *walk.ListBox
	var okPB, cancelPB *walk.PushButton

	type row struct {
		name string
		id   string
	}
	rows := []acctRow{}
	for _, a := range cfg.Accounts {
		rows = append(rows, acctRow{a.Name + "（" + a.ID + "）", a.ID})
	}
	lm := &listModel{rows: rows}

	err := (Dialog{
		AssignTo:     &dlg,
		Title:        "账号管理",
		MinSize:      Size{Width: 320, Height: 260},
		MaxSize:      Size{Width: 320, Height: 260},
		CancelButton: &cancelPB,
		DefaultButton: &okPB,
		Layout:       VBox{Margins: Margins{Left: 16, Top: 14, Right: 16, Bottom: 12}, Spacing: 10},
		Children: []Widget{
			Label{Text: "已保存的账号（本机）", TextColor: walk.RGB(130, 130, 130)},
			ListBox{AssignTo: &list, Model: lm},
			Composite{
				Layout: HBox{MarginsZero: true, Spacing: 8},
				Children: []Widget{
					PushButton{Text: "删除选中", OnClicked: func() {
						i := list.CurrentIndex()
						if i < 0 || i >= len(rows) {
							return
						}
						removed := rows[i].id
						newAccs := []Account{}
						for _, a := range cfg.Accounts {
							if a.ID != removed {
								newAccs = append(newAccs, a)
							}
						}
						cfg.Accounts = newAccs
						if cfg.Last == removed {
							cfg.Last = ""
							if len(cfg.Accounts) > 0 {
								cfg.Last = cfg.Accounts[0].ID
							}
						}
						saveConfig(cfg)
						rows = append(rows[:i], rows[i+1:]...)
					lm.rows = rows
					list.SetModel(&listModel{rows: rows})
						logf("🗑 已删除账号记录: %s", removed)
					}},
					HSpacer{},
					PushButton{AssignTo: &okPB, Text: "关 闭", OnClicked: func() { dlg.Accept() }},
					PushButton{AssignTo: &cancelPB, Text: "", Visible: false},
				},
			},
		},
	}).Create(mw)
	if err != nil {
		return
	}
	dlg.Run()
	// 切换到当前 Last 账号
	switchToLast()
}

type acctRow struct{ name, id string }

type listModel struct {
	walk.ListModelBase
	rows []acctRow
}

func (m *listModel) ItemCount() int          { return len(m.rows) }
func (m *listModel) Value(i int) interface{} { return m.rows[i].name }

func switchToLast() {
	if cfg.Last == "" {
		return
	}
	for _, a := range cfg.Accounts {
		if a.ID == cfg.Last {
			go func(acc Account) {
				c := bm.NewClient()
				if err := c.Login(acc.ID, acc.Pwd); err == nil {
					rt.mu.Lock()
					rt.client = c
					rt.mu.Unlock()
					logf("🔄 已切换: %s（%s）", c.Name, acc.ID)
				}
			}(a)
			return
		}
	}
}

// ---------- 主窗口事件 ----------

func setStatus(s string) {
	mw.Synchronize(func() { statusLabel.SetText(s) })
}

func onToggleClicked() {
	rt.mu.Lock()
	c := rt.client
	running := rt.running
	rt.mu.Unlock()
	if c == nil {
		walk.MsgBox(mw, "提示", "请先登录账号", walk.MsgBoxIconWarning)
		return
	}
	rt.mu.Lock()
	started := false
	if running {
		close(rt.stopCh)
		rt.running = false
		rt.stopCh = nil
	} else {
		rt.stopCh = make(chan struct{})
		rt.running = true
		started = true
		go autoLoop(rt.stopCh)
	}
	rt.mu.Unlock()
	if started {
		logf("▶ 自动答题已启动")
	} else {
		logf("⏹ 自动答题已停止")
	}
}

func onCheckUpdateClicked() {
	setStatus("检查更新中…")
	go func() {
		meta, err := fetchUpdateMeta()
		if err != nil {
			setStatus("检查失败：网络不可达")
			return
		}
		if meta.VersionCode <= AppVersionCode {
			setStatus("已是最新版本 v" + AppVersion)
			return
		}
		if !walkMsgBoxYesNo("发现更新",
			"发现新版本 v"+meta.VersionName+"，是否下载安装？\n\n更新内容：\n"+meta.Changelog) {
			setStatus("")
			return
		}
		mw.Synchronize(func() {
			pb.SetVisible(true)
			pb.SetValue(0)
		})
		setStatus("下载 v" + meta.VersionName + "…")
		tmp, err := downloadExe(meta.ExeURL, func(p int) {
			mw.Synchronize(func() { pb.SetValue(p) })
		})
		if err != nil {
			mw.Synchronize(func() { pb.SetVisible(false) })
			setStatus("下载失败: " + err.Error())
			return
		}
		// 已安装场景：静默自替换；未安装（便携）场景：提示重启生效
		if isInstalled() {
			setStatus("安装更新…")
			if err := selfReplace(tmp); err != nil {
				mw.Synchronize(func() { pb.SetVisible(false) })
				setStatus("更新失败: " + err.Error())
			}
			return
		}
		mw.Synchronize(func() { pb.SetVisible(false) })
		setStatus("已下载，重启后生效")
	}()
}

// ---------- UI 刷新 ----------

func updateUI() {
	if appQuiting {
		return
	}
	rt.mu.Lock()
	pts := rt.lastPts
	running := rt.running
	taskMsg := rt.taskMsg
	logs := strings.Join(rt.logs, "\n")
	client := rt.client
	rt.mu.Unlock()

	byName := map[string]bm.PointsDetail{}
	for _, p := range pts {
		byName[p.Name] = p
	}
	for i, name := range pointNames() {
		if p, ok := byName[name]; ok {
			full := p.Cur >= p.Max
			ptLabels[i].SetText(fmt.Sprintf("%s  %g/%g", name, p.Cur, p.Max))
			if full {
				ptLabels[i].SetTextColor(walk.RGB(7, 193, 96))
			} else {
				ptLabels[i].SetTextColor(walk.RGB(230, 80, 60))
			}
		} else {
			ptLabels[i].SetText(name + "  -")
			ptLabels[i].SetTextColor(walk.RGB(120, 120, 120))
		}
	}

	if c := client; c != nil {
		userLink.SetText("👤 " + c.Name + " ▼")
	} else {
		userLink.SetText("未登录 ▼")
	}

	if running {
		if taskMsg == "" {
			taskMsg = "监控中（3秒轮询）"
		}
		taskLabel.SetText("🟢 " + taskMsg)
		if btnToggle.Text() != "停止" {
			btnToggle.SetText("停止")
		}
	} else {
		if client != nil {
			taskLabel.SetText("⚪ 未启动 · 点「启动」开始自动补分")
		} else {
			taskLabel.SetText("⚪ 未登录")
		}
		if btnToggle.Text() != "启动" {
			btnToggle.SetText("启动")
		}
	}

	if old := logEdit.Text(); old != logs {
		logEdit.SetText(logs)
	}
}