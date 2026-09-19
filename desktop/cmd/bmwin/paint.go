//go:build windows

// 美化绘图组件：渐变头部 / 圆角积分卡（CustomWidget 自绘）
package main

import (
	"fmt"
	"syscall"
	"unsafe"

	"github.com/lxn/walk"
	"github.com/lxn/win"
)

// 微信绿系配色
var (
	colGreen      = walk.RGB(7, 193, 96)   // 主绿 #07C160
	colGreenDark  = walk.RGB(5, 150, 74)   // 深绿
	colGreenPale  = walk.RGB(232, 248, 238) // 浅绿背景 #E8F8EE
	colRed        = walk.RGB(230, 80, 60)
	colWhite      = walk.RGB(255, 255, 255)
)

// roundedRect 画圆角矩形（GDI RoundRect + NullPen）
// walk 画刷 handle() 是小写未导出，直接用 win.CreateBrushIndirect 自己创建
func roundedRect(c *walk.Canvas, bounds walk.Rectangle, color walk.Color, radius int) {
	lb := &win.LOGBRUSH{LbStyle: win.BS_SOLID, LbColor: win.COLORREF(color)}
	hbr := win.CreateBrushIndirect(lb)
	lbp := &win.LOGBRUSH{LbStyle: win.BS_SOLID, LbColor: 0}
	hpen := win.ExtCreatePen(win.PS_NULL, 1, lbp, 0, nil)
	oldBrush := win.SelectObject(c.HDC(), win.HGDIOBJ(hbr))
	oldPen := win.SelectObject(c.HDC(), win.HGDIOBJ(hpen))
	win.RoundRect(c.HDC(), int32(bounds.X), int32(bounds.Y),
		int32(bounds.X+bounds.Width), int32(bounds.Y+bounds.Height), int32(radius), int32(radius))
	win.SelectObject(c.HDC(), oldBrush)
	win.SelectObject(c.HDC(), oldPen)
	win.DeleteObject(win.HGDIOBJ(hbr))
	win.DeleteObject(win.HGDIOBJ(hpen))
}

var _ = unsafe.Pointer(nil)
var _ = syscall.Getpid

// paintHeader 绿色渐变顶栏（品牌名 + 副标题）
func paintHeader(c *walk.Canvas, bounds walk.Rectangle) error {
	// 手动垂直渐变：#07C160 → #06AD56
	step := 2
	for y := 0; y < bounds.Height; y += step {
		t := float64(y) / float64(bounds.Height)
		r := uint8(int(7) + int((5-7)*t))
		g := uint8(int(193) + int((150-193)*t))
		b := uint8(int(96) + int((74-96)*t))
		brush, _ := walk.NewSolidColorBrush(walk.RGB(r, g, b))
		c.FillRectangle(brush, walk.Rectangle{X: bounds.X, Y: bounds.Y + y, Width: bounds.Width, Height: step})
		brush.Dispose()
	}
	// 品牌名
	font, _ := walk.NewFont("Microsoft YaHei", 15, walk.FontBold)
	defer font.Dispose()
	c.DrawText("彬煤助手", font, colWhite,
		walk.Rectangle{X: bounds.X + 18, Y: bounds.Y + 10, Width: 200, Height: 30},
		walk.TextLeft|walk.TextVCenter|walk.TextSingleLine)
	// 副标题
	font2, _ := walk.NewFont("Microsoft YaHei", 9, 0)
	defer font2.Dispose()
	c.DrawText("自动答题 · 积分管理", font2, walk.RGB(220, 245, 230),
		walk.Rectangle{X: bounds.X + 18, Y: bounds.Y + 40, Width: 200, Height: 20},
		walk.TextLeft|walk.TextVCenter|walk.TextSingleLine)
	return nil
}

// paintPointCard 单张积分小卡片（圆角白底 + 名称 + 数值）
// cx/cy 卡片左上角；name/val 显示内容；full 是否满分
func paintPointCard(c *walk.Canvas, b walk.Rectangle, name string, val string, full bool) {
	// 白底圆角卡
	roundedRect(c, b, colWhite, 16)
	// 名称（灰）
	fontS, _ := walk.NewFont("Microsoft YaHei", 9, 0)
	defer fontS.Dispose()
	c.DrawText(name, fontS, walk.RGB(102, 102, 102),
		walk.Rectangle{X: b.X, Y: b.Y + 8, Width: b.Width, Height: 18},
		walk.TextCenter|walk.TextVCenter|walk.TextSingleLine)
	// 数值（满绿缺红，加粗）
	color := walk.Color(colRed)
	if full {
		color = colGreen
	}
	fontV, _ := walk.NewFont("Microsoft YaHei", 13, walk.FontBold)
	defer fontV.Dispose()
	c.DrawText(val, fontV, color,
		walk.Rectangle{X: b.X, Y: b.Y + 28, Width: b.Width, Height: 26},
		walk.TextCenter|walk.TextVCenter|walk.TextSingleLine)
}

// paintPointsPanel 整个积分面板（6 卡片 3×2 + 面板底色）
func paintPointsPanel(c *walk.Canvas, bounds walk.Rectangle, pts []pointView) {
	// 面板底色浅绿
	brush, _ := walk.NewSolidColorBrush(colGreenPale)
	defer brush.Dispose()
	c.FillRectangle(brush, bounds)
	// 面板标题
	fontT, _ := walk.NewFont("Microsoft YaHei", 10, walk.FontBold)
	defer fontT.Dispose()
	c.DrawText("今日积分", fontT, walk.RGB(80, 80, 80),
		walk.Rectangle{X: bounds.X + 16, Y: bounds.Y + 10, Width: 120, Height: 20},
		walk.TextLeft|walk.TextVCenter|walk.TextSingleLine)
	// 6 卡片 3×2
	cardW := (bounds.Width - 32 - 16) / 3 // 左右边距16+间隙
	cardH := 62
	gap := 8
	startY := bounds.Y + 36
	for i, pv := range pts {
		row, col := i/3, i%3
		x := bounds.X + 16 + col*(cardW+gap)
		y := startY + row*(cardH+gap)
		paintPointCard(c, walk.Rectangle{X: x, Y: y, Width: cardW, Height: cardH}, pv.name, pv.val, pv.full)
	}
}

// paintPointsCanvas CustomWidget 绘制回调（积分面板）
func paintPointsCanvas(c *walk.Canvas, bounds walk.Rectangle) error {
	paintPointsPanel(c, bounds, currentPointViews())
	return nil
}

// pointView 积分视图数据（UI 线程每帧读取）
type pointView struct {
	name string
	val  string
	full bool
}

func currentPointViews() []pointView {
	rt.mu.Lock()
	pts := rt.lastPts
	rt.mu.Unlock()
	byName := map[string]struct {
		cur, max float64
		ok       bool
	}{}
	for _, p := range pts {
		byName[p.Name] = struct {
			cur, max float64
			ok       bool
		}{p.Cur, p.Max, true}
	}
	names := []string{"签到", "知识学习", "手机考试", "模拟考试", "手机练习", "视频学习"}
	out := make([]pointView, 0, 6)
	for _, n := range names {
		if d, ok := byName[n]; ok && d.ok {
			full := d.cur >= d.max
			out = append(out, pointView{n, fmt.Sprintf("%g/%g", d.cur, d.max), full})
		} else {
			out = append(out, pointView{n, "-", false})
		}
	}
	return out
}