package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"bmclient/bm"
)

// ---------- 配置持久化 ----------

type Config struct {
	Accounts []Account `json:"accounts"`
	Last     string    `json:"last"`
}

type Account struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Pwd  string `json:"pwd"`
}

var cfgPath = "bm_config.json"

func loadConfig() *Config {
	c := &Config{}
	if b, err := os.ReadFile(cfgPath); err == nil {
		json.Unmarshal(b, c)
	}
	return c
}

func saveConfig(c *Config) {
	b, _ := json.MarshalIndent(c, "", "  ")
	os.WriteFile(cfgPath, b, 0600)
}

// ---------- 运行时状态 ----------

type Runtime struct {
	mu         sync.Mutex
	client     *bm.Client
	running    bool
	stopCh     chan struct{}
	logs       []string
	taskMsg    string
	lastPts    []bm.PointsDetail
	ptsErr     error
	ptsAt      time.Time
}

var rt = &Runtime{}

func logf(format string, a ...interface{}) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	line := time.Now().Format("15:04:05") + "  " + fmt.Sprintf(format, a...)
	rt.logs = append(rt.logs, line)
	if len(rt.logs) > 300 {
		rt.logs = rt.logs[len(rt.logs)-300:]
	}
	log.Println(line)
}

// ---------- 自动答题任务 ----------

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
		// 3 秒状态机轮询（与 App auto_start v3 同节奏）
		select {
		case <-stop:
			return
		case <-time.After(3 * time.Second):
		}
	}
}

func runOnce(c *bm.Client) {
	pts, err := c.GetPoints()
	if err != nil {
		logf("⚠️ 积分获取失败: %v", err)
		return
	}
	// 分别判断：手机考试 / 模拟考试（与 App v1.7.1 相同语义）
	for _, p := range pts {
		name := p.Name
		if p.Cur < passScore && (strings.Contains(name, "手机考试") || strings.Contains(name, "模拟考试")) {
			logf("📋 %s %g/%g 未满 → 自动开考", name, p.Cur, p.Max)
			doExam(c, name)
			return // 一次只跑一张卷，下轮继续判断
		}
	}
	rt.mu.Lock()
	rt.taskMsg = "全部达标，等待下一轮检查"
	rt.mu.Unlock()
}

func doExam(c *bm.Client, examName string) {
	papers, err := c.SelectCanRunList(c.ExamTypeID)
	if err != nil {
		logf("❌ 试卷列表失败: %v", err)
		return
	}
	// 选试卷二（与 App 一致），无则第一张
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
	logf("📝 [%s] 开始: %s（40题自动作答中…）", examName, target.PaperName)
	t0 := time.Now()
	// 积分归属由建卷 type 决定：手机考试=2、模拟考试=1（站点 SelectPaper/MobileExam 两条链路的唯一差异）
	examType := "2"
	if strings.Contains(examName, "模拟") {
		examType = "1"
	}
	res, err := c.RunExamMode(target, examType)
	if err != nil {
		logf("❌ 考试失败: %v", err)
		return
	}
	// res 形如 "您的分数为：100,合格！|100|合格！"
	parts := strings.Split(res, "|")
	score := ""
	if len(parts) > 1 {
		score = parts[1]
	}
	logf("🎉 [%s] 交卷完成: %s 分（用时 %ds）", examName, score, time.Since(t0).Milliseconds()/1000)
	rt.mu.Lock()
	rt.taskMsg = fmt.Sprintf("最近一次: %s %s 分", examName, score)
	rt.mu.Unlock()
}

// ---------- HTTP API ----------

type apiResp struct {
	OK   bool   `json:"ok"`
	Msg  string `json:"msg,omitempty"`
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(v)
}

func statusHandler(w http.ResponseWriter, r *http.Request) {
	// 先无锁快照，网络调用绝不在锁内（否则 UI 轮询与自动任务互锁）
	rt.mu.Lock()
	running, ready := rt.running, rt.client != nil
	taskMsg := rt.taskMsg
	logs := strings.Join(rt.logs, "\n")
	c := rt.client
	name, account := "", ""
	if c != nil {
		name, account = c.Name, c.Account
	}
	ptsCache, ptsErr, ptsAt := rt.lastPts, rt.ptsErr, rt.ptsAt
	rt.mu.Unlock()

	cfg := loadConfig()
	type acct struct {
		ID, Name string
		Cur      bool
	}
	accts := []acct{}
	for _, a := range cfg.Accounts {
		accts = append(accts, acct{a.ID, a.Name, a.ID == cfg.Last})
	}
	s := map[string]interface{}{
		"running": running, "ready": ready, "taskmsg": taskMsg, "log": logs, "accounts": accts,
	}
	if c != nil {
		s["name"] = name
		s["account"] = account
		if time.Since(ptsAt) > 3*time.Second {
			pts, err := c.GetPoints()
			ptsCache, ptsErr, ptsAt = pts, err, time.Now()
			rt.mu.Lock()
			rt.lastPts, rt.ptsErr, rt.ptsAt = ptsCache, ptsErr, ptsAt
			rt.mu.Unlock()
		}
		if ptsErr == nil {
			s["points"] = ptsCache
		}
	}
	writeJSON(w, s)
}

func loginHandler(w http.ResponseWriter, r *http.Request) {
	var req struct{ Account string `json:"account"` }
	json.NewDecoder(r.Body).Decode(&req)
	req.Account = strings.TrimSpace(req.Account)
	if req.Account == "" {
		writeJSON(w, apiResp{OK: false, Msg: "账号不能为空"})
		return
	}
	c := bm.NewClient()
	if err := c.Login(req.Account, bm.DefaultPassword); err != nil {
		logf("❌ 账号 %s 登录失败: %v", req.Account, err)
		writeJSON(w, apiResp{OK: false, Msg: err.Error()})
		return
	}
	cfg := loadConfig()
	found := false
	for i := range cfg.Accounts {
		if cfg.Accounts[i].ID == req.Account {
			cfg.Accounts[i].Name = c.Name
			found = true
		}
	}
	if !found {
		cfg.Accounts = append(cfg.Accounts, Account{ID: req.Account, Name: c.Name, Pwd: bm.DefaultPassword})
	}
	cfg.Last = req.Account
	saveConfig(cfg)
	rt.mu.Lock()
	rt.client = c
	rt.logs = nil
	rt.mu.Unlock()
	logf("✅ 登录成功: %s（%s）", c.Name, req.Account)
	writeJSON(w, apiResp{OK: true})
}

func switchHandler(w http.ResponseWriter, r *http.Request) {
	var req struct{ Account string `json:"account"` }
	json.NewDecoder(r.Body).Decode(&req)
	cfg := loadConfig()
	for _, a := range cfg.Accounts {
		if a.ID == req.Account {
			c := bm.NewClient()
			if err := c.Login(a.ID, a.Pwd); err != nil {
				writeJSON(w, apiResp{OK: false, Msg: err.Error()})
				return
			}
			cfg.Last = a.ID
			saveConfig(cfg)
			rt.mu.Lock()
			rt.client = c
			rt.logs = nil
			rt.mu.Unlock()
			logf("🔄 切换账号: %s（%s）", c.Name, a.ID)
			writeJSON(w, apiResp{OK: true})
			return
		}
	}
	writeJSON(w, apiResp{OK: false, Msg: "账号不存在"})
}

func toggleHandler(w http.ResponseWriter, r *http.Request) {
	rt.mu.Lock()
	c := rt.client
	running := rt.running
	rt.mu.Unlock()
	if c == nil {
		writeJSON(w, apiResp{OK: false, Msg: "请先登录账号"})
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
	// 日志必须在锁外（logf 内部要抢 rt.mu，锁内调用会自死锁）
	if started {
		logf("▶ 自动答题已启动（3秒轮询，分别判断手机考试/模拟考试）")
	} else {
		logf("⏹ 自动答题已停止")
	}
	writeJSON(w, apiResp{OK: true})
}

// ---------- 启动 ----------

func openBrowser(url string) {
	switch runtime.GOOS {
	case "windows":
		// start 的第一个引号参数是窗口标题，避免 URL 被当标题
		exec.Command("cmd", "/c", "start", "", url).Start()
	case "darwin":
		exec.Command("open", url).Start()
	default:
		exec.Command("xdg-open", url).Start()
	}
}

func main() {
	// 恢复上次账号（静默重登）
	cfg := loadConfig()
	if cfg.Last != "" {
		for _, a := range cfg.Accounts {
			if a.ID == cfg.Last {
				c := bm.NewClient()
				if err := c.Login(a.ID, a.Pwd); err == nil {
					rt.mu.Lock()
					rt.client = c
					rt.mu.Unlock()
					logf("✅ 已恢复登录: %s（%s）", c.Name, a.ID)
				}
				break
			}
		}
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(indexHTML))
	})
	http.HandleFunc("/api/status", statusHandler)
	http.HandleFunc("/api/login", loginHandler)
	http.HandleFunc("/api/switch", switchHandler)
	http.HandleFunc("/api/toggle", toggleHandler)

	addr := "127.0.0.1:8642"
	logf("🖥 彬煤助手（电脑端）已启动: http://%s", addr)
	go func() {
		time.Sleep(500 * time.Millisecond)
		openBrowser("http://" + addr)
	}()
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatal(err)
	}
}