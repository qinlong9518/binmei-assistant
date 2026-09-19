//go:build windows

package main

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"bmclient/bm"
)

// ---------- 运行时状态（线程安全） ----------

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

func logf(format string, a ...interface{}) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.logs = append(rt.logs, time.Now().Format("15:04:05")+"  "+fmt.Sprintf(format, a...))
	if len(rt.logs) > 300 {
		rt.logs = rt.logs[len(rt.logs)-300:]
	}
}

// ---------- 自动答题引擎（与 walk 版完全同源） ----------

const passScore = 24.0

func autoLoop(stop chan struct{}) {
	for {
		select {
		case <-stop:
			return
		default:
		}
		c := rtClient()
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