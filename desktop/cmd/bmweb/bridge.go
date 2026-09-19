//go:build windows

package main

import (
	"context"
	"strings"
	"sync"
	"time"

	"bmclient/bm"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// UIBridge 绑定给前端 JS 的方法集（window.go.main.App.*）
type UIBridge struct {
	ctx context.Context
	mu  sync.Mutex
}

func NewBridge() *UIBridge { return &UIBridge{} }

func (b *UIBridge) attach(ctx context.Context) {
	b.ctx = ctx
	bridgeRef = b
	// 启动后台轮询（积分+UI 状态推送）
	go uiTick()
	// 已有历史账号 → 静默恢复登录（微信式记住登录）
	if cfg.Last != "" {
		for _, a := range cfg.Accounts {
			if a.ID == cfg.Last {
				acc := a
				go func() {
					c := bm.NewClient()
					if err := c.Login(acc.ID, acc.Pwd); err == nil {
						rt.mu.Lock()
						rt.client = c
						rt.mu.Unlock()
						logf("✅ 已恢复登录: %s（%s）", c.Name, acc.ID)
					} else {
						logf("⚠️ 恢复登录失败，请重新登录")
					}
				}()
				break
			}
		}
	}
}

// emit 向前端推事件（runtime.Events.Emit）
func (b *UIBridge) emit(name string, data interface{}) {
	if b.ctx == nil {
		return
	}
	runtime.EventsEmit(b.ctx, name, data)
}

// ---- 状态结构 ----

type PtView struct {
	Name  string  `json:"name"`
	Cur   float64 `json:"cur"`
	Max   float64 `json:"max"`
	IsMax bool    `json:"full"`
}

type StateView struct {
	LoggedIn bool     `json:"loggedIn"`
	Name     string   `json:"name"`
	Account  string   `json:"account"`
	Running  bool     `json:"running"`
	Task     string   `json:"task"`
	Points   []PtView `json:"points"`
	Logs     string   `json:"logs"`
	Status   string   `json:"status"`
}

func buildState() StateView {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	sv := StateView{
		Logs:   strings.Join(rt.logs, "\n"),
		Task:   rt.taskMsg,
		Status: statusMsg,
	}
	if c := rt.client; c != nil {
		sv.LoggedIn = true
		sv.Name = c.Name
		sv.Account = c.Account
	}
	sv.Running = rt.running
	for _, p := range rt.lastPts {
		sv.Points = append(sv.Points, PtView{
			Name: p.Name, Cur: p.Cur, Max: p.Max, IsMax: p.Cur >= p.Max,
		})
	}
	return sv
}

// statusMsg 全局状态栏消息（检查更新/切换账号等写这里）
var statusMsg = ""

func setStatus(s string) {
	rt.mu.Lock()
	statusMsg = s
	rt.mu.Unlock()
}

// uiTick 500ms 推一次全量状态（前端按需 diff）
func uiTick() {
	b := bridgeRef
	for {
		time.Sleep(500 * time.Millisecond)
		if b == nil {
			continue
		}
		b.emit("state", buildState())
	}
}

var bridgeRef *UIBridge

// ---- 登录 ----

// Login 登录（密码自动填充服务端默认值）
func (b *UIBridge) Login(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return "请输入账号"
	}
	c := bm.NewClient()
	if err := c.Login(id, bm.DefaultPassword); err != nil {
		logf("❌ 登录失败: %v", err)
		return err.Error()
	}
	upsertAccount(id, c.Name)
	rt.mu.Lock()
	rt.client = c
	rt.mu.Unlock()
	logf("✅ 登录成功: %s（%s）", c.Name, id)
	return ""
}

// Logout 登出
func (b *UIBridge) Logout() {
	rt.mu.Lock()
	if rt.running && rt.stopCh != nil {
		close(rt.stopCh)
	}
	rt.running = false
	rt.stopCh = nil
	rt.client = nil
	rt.mu.Unlock()
	logf("⏹ 已退出登录")
}

// ---- 自动答题开关 ----

func (b *UIBridge) ToggleRun() {
	rt.mu.Lock()
	c := rt.client
	running := rt.running
	rt.mu.Unlock()
	if c == nil {
		b.emit("toast", "请先登录账号")
		return
	}
	rt.mu.Lock()
	if running {
		close(rt.stopCh)
		rt.running = false
		rt.stopCh = nil
	} else {
		rt.stopCh = make(chan struct{})
		rt.running = true
		go autoLoop(rt.stopCh)
	}
	started := rt.running
	rt.mu.Unlock()
	if started {
		logf("▶ 自动答题已启动")
	} else {
		logf("⏹ 自动答题已停止")
	}
}

// ---- 账号管理 ----

// ListAccounts 返回本机保存的账号列表
func (b *UIBridge) ListAccounts() []Account {
	return cfg.Accounts
}

// CurrentAccount 当前登录账号 ID
func (b *UIBridge) CurrentAccount() string {
	if c := rtClient(); c != nil {
		return c.Account
	}
	return ""
}

// SwitchAccount 切换账号
func (b *UIBridge) SwitchAccount(id string) {
	var acc *Account
	for i := range cfg.Accounts {
		if cfg.Accounts[i].ID == id {
			acc = &cfg.Accounts[i]
			break
		}
	}
	if acc == nil {
		b.emit("toast", "账号不存在")
		return
	}
	setStatus("切换中…")
	go func(a Account) {
		c := bm.NewClient()
		if err := c.Login(a.ID, a.Pwd); err == nil {
			cfg.Last = a.ID
			saveConfig(cfg)
			rt.mu.Lock()
			rt.client = c
			rt.mu.Unlock()
			logf("🔄 已切换: %s（%s）", c.Name, a.ID)
			setStatus("")
		} else {
			logf("❌ 切换失败: %v", err)
			setStatus("切换失败")
		}
	}(*acc)
}

// DeleteAccount 删除账号记录（若删除的是当前登录账号则自动登出）
func (b *UIBridge) DeleteAccount(id string) {
	logout := removeAccount(id)
	logf("🗑 已删除账号记录: %s", id)
	if logout {
		b.emit("toast", "已删除当前登录账号，请重新登录")
	}
}

// ---- 更新 ----

// CheckUpdate 检查并安装更新（有新版本时前端弹确认）
func (b *UIBridge) CheckUpdate() {
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
		b.emit("updateAvailable", meta.VersionName+"\n\n"+meta.Changelog)
		setStatus("")
	}()
}

// DoUpdate 确认后下载安装
func (b *UIBridge) DoUpdate() {
	go func() {
		meta, err := fetchUpdateMeta()
		if err != nil {
			setStatus("下载失败：网络不可达")
			return
		}
		setStatus("下载 v" + meta.VersionName + "…")
		tmp, err := downloadExe(meta.ExeURL, func(p int) {
			b.emit("dlProgress", p)
		})
		if err != nil {
			setStatus("下载失败: " + err.Error())
			return
		}
		if isInstalled() {
			setStatus("安装更新…")
			if err := selfReplace(tmp); err != nil {
				setStatus("更新失败: " + err.Error())
			}
			return
		}
		setStatus("已下载，重启后生效")
	}()
}

// OpenSite 打开官网
func (b *UIBridge) OpenSite() {
	openURL(OfficialSite)
}