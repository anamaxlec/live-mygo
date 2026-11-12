//go:build darwin || windows

package app

import (
	"runtime"
	"time"

	"github.com/egoist/mygo/ui"
)

// 沉浸时一并收起导航侧栏，退出时恢复进入前的状态。
func (a *app) toggleImmersive() {
	if a.page != "player" || a.companion || a.snap || a.win != nil && a.win.IsFullScreen() {
		return
	}
	a.immersive = !a.immersive
	if a.immersive {
		a.immersiveRestoreNav = !a.focusMode
		a.focusMode = true
	} else if a.immersiveRestoreNav {
		a.focusMode = false
		a.immersiveRestoreNav = false
	}
	a.immersiveHeaderVisible = false
	a.featuredViewer().closePlayerMenus()
	a.featuredViewer().revealControls()
	a.fitWindowToVideo(true)
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
	// 顶部这条带同时唤出系统窗口按钮，指针停在按钮上时不收起。
	atTop := inside && y < top+8 && (x >= video.X && x <= video.X+video.W || x < video.X)
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
	a.syncWatchWindowButtons(!a.immersiveHeaderVisible)
	panel.Opacity(opacity).Translate(0, -8*(1-opacity)).Hidden(opacity < .005).Children(func() {
		a.watchHeaderControls(c, 10, 10, 8, true)
	})
}

// 只有这里改写系统窗口按钮；focus、resize 后调用 reapplyWindowButtons 恢复。
func (a *app) syncWatchWindowButtons(hide bool) {
	if a.companion || hide == a.watchButtonsHidden {
		return
	}
	a.watchButtonsHidden = hide
	a.reapplyWindowButtons()
}

// MyGo 在聚焦和改变尺寸时会重排标题栏按钮，需要按当前模式再应用一次。
func (a *app) reapplyWindowButtons() {
	if a.surface == nil {
		return
	}
	hide := a.companion || a.watchButtonsHidden
	a.surface.SetWindowButtonsVisible(!hide)
	if !hide && a.win != nil && runtime.GOOS == "darwin" && !a.win.IsFullScreen() {
		// AppKit 会在重新显示按钮时重排标题栏，交给 MyGo 恢复窗口配置的坐标。
		a.win.SetTitle(a.win.Title())
	}
}
