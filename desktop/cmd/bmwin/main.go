//go:build windows

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"bmclient/bm"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
)

// ---------- 配置 ----------

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
	if exe, err := os.Executable(); err == nil {
		return filepath.Join(filepath.Dir(exe), "bm_config.json")
	}
	return "bm_config.json"
}

func loadConfig() *Config {
	c := &Config{}
	if b, err := os.ReadFile(cfgPath()); err == nil {
		json.Unmarshal(b, c)
	}
	return c
}

func saveConfig(c *Config) {
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

// ---------- UI ----------

var (
	mw          *walk.MainWindow
	verLabel    *walk.Label
	accEdit     *walk.LineEdit
	accBox      *walk.ComboBox
	btnLogin    *walk.PushButton
	ptLabels    [6]*walk.Label
	taskLabel   *walk.Label
	btnToggle   *walk.PushButton
	logEdit     *walk.TextEdit
	btnUpdate   *walk.PushButton
	statusLabel *walk.Label
	pb          *walk.ProgressBar

	accModel   *comboBoxModel
	uiLock     bool
	appQuiting bool
)

func pointNames() []string {
	return []string{"签到", "知识学习", "手机考试", "模拟考试", "手机练习", "视频学习"}
}

// comboBoxModel 账号下拉数据源
type comboBoxModel struct {
	walk.ListModelBase
	items []string
}

func (m *comboBoxModel) ItemCount() int          { return len(m.items) }
func (m *comboBoxModel) Value(i int) interface{} { return m.items[i] }

func refreshAccModel() {
	items := []string{}
	cur := ""
	for _, a := range cfg.Accounts {
		items = append(items, a.Name+"（"+a.ID+"）")
		if a.ID == cfg.Last {
			cur = a.Name + "（" + a.ID + "）"
		}
	}
	uiLock = true
	accModel = &comboBoxModel{items: items}
	accBox.SetModel(accModel)
	accBox.SetText(cur)
	uiLock = false
}

func buildUI() {
	accModel = &comboBoxModel{}
	err := (MainWindow{
		AssignTo: &mw,
		Title:    "彬煤答题助手",
		MinSize:  Size{Width: 470, Height: 640},
		MaxSize:  Size{Width: 470, Height: 640},
		Layout:   VBox{Margins: Margins{Left: 12, Top: 10, Right: 12, Bottom: 10}, Spacing: 8},
		Children: []Widget{
			Composite{
				Layout: HBox{MarginsZero: true},
				Children: []Widget{
					Label{AssignTo: &verLabel, Text: "彬煤答题助手  v" + AppVersion + "  ·  电脑端",
						Font: Font{PointSize: 11, Bold: true}},
				},
			},
			GroupBox{
				Title:  "账号",
				Layout: VBox{MarginsZero: true, Spacing: 6},
				Children: []Widget{
					Composite{
						Layout: HBox{MarginsZero: true, Spacing: 6},
						Children: []Widget{
							LineEdit{AssignTo: &accEdit, CueBanner: "输入身份证账号"},
							PushButton{AssignTo: &btnLogin, Text: "登录", OnClicked: onLoginClicked},
						},
					},
					ComboBox{AssignTo: &accBox, Model: accModel,
					OnCurrentIndexChanged: onAccountSwitch},
				},
			},
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
					Label{AssignTo: &taskLabel, Text: "⚪ 未登录"},
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
			Composite{
				Layout: HBox{MarginsZero: true, Spacing: 8},
				Children: []Widget{
					PushButton{AssignTo: &btnUpdate, Text: "检查更新", OnClicked: onCheckUpdateClicked},
					PushButton{Text: "官网", OnClicked: func() {
						exec.Command("rundll32", "url.dll,FileProtocolHandler", OfficialSite).Start()
					}},
					ProgressBar{AssignTo: &pb, MinValue: 0, MaxValue: 100, Visible: false},
					Label{AssignTo: &statusLabel, Text: ""},
				},
			},
		},
	}).Create()
	if err != nil {
		panic(err)
	}

	// 初始账号下拉
	if len(cfg.Accounts) > 0 {
		refreshAccModel()
	}

	mw.Closing().Attach(func(canceled *bool, reason walk.CloseReason) {
		rt.mu.Lock()
		if rt.running && rt.stopCh != nil {
			close(rt.stopCh)
		}
		rt.mu.Unlock()
	})
}

// ---------- UI 事件 ----------

func setStatus(s string) {
	mw.Synchronize(func() { statusLabel.SetText(s) })
}

func onLoginClicked() {
	acc := strings.TrimSpace(accEdit.Text())
	if acc == "" {
		walk.MsgBox(mw, "提示", "请输入账号", walk.MsgBoxIconWarning)
		return
	}
	mw.Synchronize(func() { btnLogin.SetEnabled(false) })
	setStatus("登录中…")
	go func() {
		c := bm.NewClient()
		if err := c.Login(acc, bm.DefaultPassword); err != nil {
			mw.Synchronize(func() {
				btnLogin.SetEnabled(true)
				statusLabel.SetText("登录失败")
			})
			logf("❌ 登录失败: %v", err)
			return
		}
		found := false
		for i := range cfg.Accounts {
			if cfg.Accounts[i].ID == acc {
				cfg.Accounts[i].Name = c.Name
				found = true
			}
		}
		if !found {
			cfg.Accounts = append(cfg.Accounts, Account{ID: acc, Name: c.Name, Pwd: bm.DefaultPassword})
		}
		cfg.Last = acc
		saveConfig(cfg)
		rt.mu.Lock()
		rt.client = c
		rt.mu.Unlock()
		logf("✅ 登录成功: %s（%s）", c.Name, acc)
		mw.Synchronize(func() {
			btnLogin.SetEnabled(true)
			accEdit.SetText("")
			refreshAccModel()
			statusLabel.SetText("登录成功")
		})
	}()
}

func onAccountSwitch() {
	if uiLock {
		return
	}
	txt := accBox.Text()
	i := strings.LastIndex(txt, "（")
	j := strings.LastIndex(txt, "）")
	if i < 0 || j <= i {
		return
	}
	id := txt[i+len("（") : j]
	for _, a := range cfg.Accounts {
		if a.ID == id {
			go func(acc Account) {
				setStatus("切换账号中…")
				c := bm.NewClient()
				if err := c.Login(acc.ID, acc.Pwd); err != nil {
					logf("❌ 切换失败: %v", err)
					setStatus("切换失败")
					return
				}
				cfg.Last = acc.ID
				saveConfig(cfg)
				rt.mu.Lock()
				rt.client = c
				rt.mu.Unlock()
				logf("🔄 切换账号: %s（%s）", c.Name, acc.ID)
				setStatus("已切换")
			}(a)
			return
		}
	}
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

// ---------- 更新 ----------

func silentCheckUpdate() {
	meta, err := fetchUpdateMeta()
	if err != nil || meta == nil {
		return
	}
	if meta.VersionCode > AppVersionCode {
		setStatus("发现新版本 v" + meta.VersionName + "，可点「检查更新」安装")
	}
}

func onCheckUpdateClicked() {
	mw.Synchronize(func() { btnUpdate.SetEnabled(false) })
	setStatus("检查更新中…")
	go func() {
		meta, err := fetchUpdateMeta()
		if err != nil {
			mw.Synchronize(func() {
				btnUpdate.SetEnabled(true)
				statusLabel.SetText("检查失败：网络不可达")
			})
			return
		}
		if meta.VersionCode <= AppVersionCode {
			mw.Synchronize(func() {
				btnUpdate.SetEnabled(true)
				statusLabel.SetText("已是最新版本 v" + AppVersion)
			})
			return
		}
		mw.Synchronize(func() {
			pb.SetVisible(true)
			pb.SetValue(0)
			statusLabel.SetText("下载 v" + meta.VersionName + "…")
		})
		tmp, err := downloadExe(meta.ExeURL, func(p int) {
			mw.Synchronize(func() { pb.SetValue(p) })
		})
		if err != nil {
			mw.Synchronize(func() {
				btnUpdate.SetEnabled(true)
				pb.SetVisible(false)
				statusLabel.SetText("下载失败: " + err.Error())
			})
			return
		}
		mw.Synchronize(func() {
			pb.SetValue(100)
			statusLabel.SetText("安装更新…")
		})
		if err := selfReplace(tmp); err != nil {
			mw.Synchronize(func() {
				btnUpdate.SetEnabled(true)
				pb.SetVisible(false)
				statusLabel.SetText("更新失败: " + err.Error())
			})
		}
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