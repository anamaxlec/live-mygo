//go:build darwin || windows

package app

import (
	"runtime"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

var windowsShortcutLabels = strings.NewReplacer("⌃⌘F", "F11", "⌘", "Ctrl+", "⌃", "Ctrl+")

func shortcutLabel(label string) string {
	if runtime.GOOS == "windows" {
		return windowsShortcutLabels.Replace(label)
	}
	return label
}

func titlebarRight(c *ui.Context, inset float32) float32 {
	if right := c.TitleBar().Right; right > 0 {
		return max(inset, right+12)
	}
	return inset
}

func titlebarLeft(c *ui.Context) float32 {
	if runtime.GOOS == "windows" {
		return max(12, c.TitleBar().Left+14)
	}
	return max(100, c.TitleBar().Left+14)
}

func (a *app) normalMinimum() {
	if a.win == nil {
		return
	}
	w, h := 1000, 580
	if runtime.GOOS == "windows" {
		area := mygo.Screen.DisplayMatching(a.win.Bounds()).WorkArea
		if area.Width > 0 && area.Height > 0 {
			w = min(w, max(320, area.Width-16))
			h = min(h, max(210, area.Height-16))
		}
	}
	a.win.SetMinimumSize(w, h)
}

func (a *app) fitInitialWindow() {
	if runtime.GOOS != "windows" || a.win == nil {
		return
	}
	a.normalMinimum()
	area := mygo.Screen.DisplayMatching(a.win.Bounds()).WorkArea
	if area.Width <= 0 || area.Height <= 0 {
		return
	}
	r := a.win.ContentBounds()
	w, h := max(320, area.Width-16), max(210, area.Height-16)
	if r.Width > w || r.Height > h {
		r.Width, r.Height = min(r.Width, w), min(r.Height, h)
		r.X, r.Y = area.X+(area.Width-r.Width)/2, area.Y+(area.Height-r.Height)/2
		a.win.SetContentBounds(r)
	}
}

func captionFontOptions() ([]string, []string) {
	if runtime.GOOS == "windows" {
		return []string{"系统默认", "微软雅黑", "黑体", "宋体", "楷体", "等宽"}, []string{"", "Microsoft YaHei", "SimHei", "SimSun", "KaiTi", "Consolas"}
	}
	return []string{"系统默认", "苹方", "宋体", "等宽"}, []string{"", "PingFang SC", "Songti SC", "Menlo"}
}
