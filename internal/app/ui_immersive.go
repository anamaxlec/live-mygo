//go:build darwin || windows

package app

import (
	"runtime"
	"time"

	"github.com/egoist/mygo/ui"
)

func (a *app) toggleImmersive() {
	if a.page != "player" || a.companion || a.snap || a.win != nil && a.win.IsFullScreen() {
		return
	}
	a.immersive = !a.immersive
	a.immersiveHeaderVisible = false
	a.featuredViewer().closePlayerMenus()
	a.featuredViewer().revealControls()
}

func (a *app) watchHeaderTarget() float32 {
	if a.immersive {
		return 0
	}
	return watchHeaderHeight
}

func (a *app) immersiveHeader(c *ui.Context) {
	width, height := c.Size()
	video := watchGeometry(width, height, 0, a.playerNavWidth, a.playerChatWidth, true).Video
	if a.hasSideViewer() {
		video.W, _ = splitVideoWidths(video.W)
	}
	panelWidth := watchControlsWidth(video.W)
	left := video.X + (video.W-panelWidth)/2
	top := video.Y + transportBottom
	const panelHeight = float32(52)
	x, y, inside := c.Root().PointerPosition()
	atTop := x >= video.X && x <= video.X+video.W && y < top+8
	overPanel := x >= left-12 && x <= left+panelWidth+12 && y <= top+panelHeight+12
	a.immersiveHeaderVisible = inside && (atTop || a.immersiveHeaderVisible && overPanel)

	target := float32(0)
	if a.immersiveHeaderVisible {
		target = 1
	}
	panel := floatingWatchControls(ui.Box(c.Key("immersive-header"))).Absolute().
		Left(left).Top(top).Size(panelWidth, panelHeight).Label("沉浸式浮动顶栏")
	opacity := target
	if !c.Preferences().ReduceMotion {
		opacity = panel.AnimateTo("reveal", target, ui.Animation{Duration: 150 * time.Millisecond, Ease: ui.EaseInOut})
	}
	panel.Opacity(opacity).Translate(0, -8*(1-opacity)).Hidden(opacity < .005).Children(func() {
		a.watchHeaderControls(c, 10, 10, 8, true)
	})
}

func (a *app) syncWatchWindowButtons(hide bool) {
	if a.surface != nil && !a.companion && hide != a.watchButtonsHidden {
		a.surface.SetWindowButtonsVisible(!hide)
		if !hide && a.win != nil && runtime.GOOS == "darwin" {
			// AppKit 会在重新显示按钮时重排标题栏，交给 MyGo 恢复窗口配置的坐标。
			a.win.SetTitle(a.win.Title())
		}
		a.watchButtonsHidden = hide
	}
}
